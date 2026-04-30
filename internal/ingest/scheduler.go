package ingest

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/lox/wandiweather/internal/ecowitt"
	"github.com/lox/wandiweather/internal/emergency"
	"github.com/lox/wandiweather/internal/firedanger"
	"github.com/lox/wandiweather/internal/forecast"
	"github.com/lox/wandiweather/internal/imagegen"
	"github.com/lox/wandiweather/internal/models"
	"github.com/lox/wandiweather/internal/store"
	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	store            *store.Store
	pws              *PWS
	forecast         *ForecastClient
	bom              *BOMClient
	bomDailyAPI      *BOMDailyAPIClient
	daily            *DailyJobs
	stationIDs       []string
	loc              *time.Location
	obsInterval      time.Duration
	imageGen         *imagegen.Generator
	imageCache       *imagegen.Cache
	imageGenMu       *sync.Mutex // Shared with server to prevent duplicate API calls
	airQualityClient *ecowitt.Client
	emergencyClient  *emergency.Client
	fireDangerClient *firedanger.Client
	cron             *cron.Cron
}

type bomIngestTarget struct {
	client     bomForecastSource
	run        *store.IngestRun
	locationID string
}

type bomFetchOutcome struct {
	forecasts   []models.Forecast
	rawBody     string
	fetchResult *FetchResult
	err         error
}

func NewScheduler(store *store.Store, pws *PWS, forecast *ForecastClient, stationIDs []string, loc *time.Location) *Scheduler {
	return &Scheduler{
		store:           store,
		pws:             pws,
		forecast:        forecast,
		bom:             NewBOMClient(""),
		bomDailyAPI:     NewBOMDailyAPIClient(""),
		daily:           NewDailyJobs(store),
		stationIDs:      stationIDs,
		loc:             loc,
		obsInterval:     5 * time.Minute,
		emergencyClient: nil, // Set via SetEmergencyClient
	}
}

// SetEmergencyClient configures the scheduler to poll for emergency alerts.
func (s *Scheduler) SetEmergencyClient(client *emergency.Client) {
	s.emergencyClient = client
}

// SetFireDangerClient configures the scheduler to poll for fire danger ratings.
func (s *Scheduler) SetFireDangerClient(client *firedanger.Client) {
	s.fireDangerClient = client
}

// SetImageGenerator configures the scheduler to pre-generate weather images after forecast ingestion.
// The mutex should be shared with the HTTP server to coordinate generation and prevent duplicate API calls.
func (s *Scheduler) SetImageGenerator(gen *imagegen.Generator, cache *imagegen.Cache, mu *sync.Mutex) {
	s.imageGen = gen
	s.imageCache = cache
	s.imageGenMu = mu
}

// SetAirQualityClient configures the scheduler to persist Ecowitt WH41 readings.
func (s *Scheduler) SetAirQualityClient(client *ecowitt.Client) {
	s.airQualityClient = client
}

func (s *Scheduler) Run(ctx context.Context) {
	// Initial ingestion on startup
	s.ingestObservations()
	s.ingestAirQuality()
	s.ingestForecasts()
	s.ingestAlerts()
	s.ingestFireDanger()
	s.checkWeatherImage()

	// Set up cron scheduler for fixed-time forecast fetching
	// Times are in Melbourne timezone (AEDT/AEST)
	// 5am: Critical - captures full day-0 forecast with temp_min before sunrise
	// 11am, 5pm, 11pm: Regular updates throughout the day
	s.cron = cron.New(cron.WithLocation(s.loc))

	s.cron.AddFunc("0 5 * * *", func() {
		log.Println("scheduler: 5am forecast fetch (pre-dawn)")
		s.ingestForecasts()
	})
	s.cron.AddFunc("0 11 * * *", func() {
		log.Println("scheduler: 11am forecast fetch")
		s.ingestForecasts()
	})
	s.cron.AddFunc("0 17 * * *", func() {
		log.Println("scheduler: 5pm forecast fetch")
		s.ingestForecasts()
	})
	s.cron.AddFunc("0 23 * * *", func() {
		log.Println("scheduler: 11pm forecast fetch")
		s.ingestForecasts()
	})

	// Daily jobs at 6am
	s.cron.AddFunc("0 6 * * *", func() {
		log.Println("scheduler: 6am daily jobs")
		yesterday := time.Now().In(s.loc).AddDate(0, 0, -1)
		s.daily.RunAll(yesterday)
	})

	s.cron.Start()
	log.Println("scheduler: cron started (forecasts at 5am, 11am, 5pm, 11pm Melbourne time)")

	// Interval-based tickers for frequent polling
	obsTicker := time.NewTicker(s.obsInterval)
	alertTicker := time.NewTicker(5 * time.Minute)
	fdrTicker := time.NewTicker(30 * time.Minute)
	imageTicker := time.NewTicker(1 * time.Hour)
	defer obsTicker.Stop()
	defer alertTicker.Stop()
	defer fdrTicker.Stop()
	defer imageTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("scheduler: shutting down")
			s.cron.Stop()
			return
		case <-obsTicker.C:
			s.ingestObservations()
			s.ingestAirQuality()
		case <-alertTicker.C:
			s.ingestAlerts()
		case <-fdrTicker.C:
			s.ingestFireDanger()
		case <-imageTicker.C:
			s.checkWeatherImage()
		}
	}
}

func (s *Scheduler) ingestForecasts() {
	if s.forecast == nil {
		return
	}

	geocode := fmt.Sprintf("%.4f,%.4f", s.forecast.lat, s.forecast.lon)

	log.Println("scheduler: ingesting WU forecasts")
	run, _ := s.store.StartIngestRun("wu", "forecast/daily/5day", nil, &geocode)
	forecasts, rawBody, fetchResult, err := s.forecast.Fetch5Day()

	if run != nil {
		run.Success = err == nil
		if fetchResult != nil {
			run.HTTPStatus = sql.NullInt64{Int64: int64(fetchResult.HTTPStatus), Valid: fetchResult.HTTPStatus > 0}
			run.ResponseSizeBytes = sql.NullInt64{Int64: int64(fetchResult.ResponseSize), Valid: fetchResult.ResponseSize > 0}
			run.RecordsParsed = sql.NullInt64{Int64: int64(fetchResult.RecordCount), Valid: true}
			if fetchResult.ParseErrors > 0 {
				run.ParseErrors = sql.NullInt64{Int64: int64(fetchResult.ParseErrors), Valid: true}
				run.ErrorMessage = sql.NullString{String: fetchResult.ParseError, Valid: true}
				log.Printf("scheduler: WU forecast parse errors: %s", fetchResult.ParseError)
			}
		}
		if err != nil {
			run.ErrorMessage = sql.NullString{String: err.Error(), Valid: true}
		}
	}

	if len(rawBody) > 0 && run != nil {
		if _, err := s.store.StoreRawPayload(&run.ID, "wu", "forecast/daily/5day", nil, &geocode, []byte(rawBody)); err != nil {
			log.Printf("scheduler: store WU raw payload: %v", err)
		}
	}

	if err != nil {
		log.Printf("scheduler: fetch WU forecast: %v", err)
	} else {
		inserted := 0
		for _, fc := range forecasts {
			if err := s.store.InsertForecast(fc); err != nil {
				log.Printf("scheduler: insert WU forecast: %v", err)
				continue
			}
			inserted++
		}
		log.Printf("scheduler: inserted %d WU forecast days", inserted)
		if run != nil {
			run.RecordsStored = sql.NullInt64{Int64: int64(inserted), Valid: true}
		}
	}

	if run != nil {
		s.store.CompleteIngestRun(run)
	}

	s.ingestBOMForecastSources()

	s.ensureWeatherImage(forecasts)
}

func (s *Scheduler) ingestBOMForecastSources() {
	var targets []bomIngestTarget
	if s.bom != nil {
		locationID := s.bom.LocationID()
		run, _ := s.store.StartIngestRun(s.bom.Source(), s.bom.Endpoint(), nil, &locationID)
		targets = append(targets, bomIngestTarget{client: s.bom, run: run, locationID: locationID})
	}
	if s.bomDailyAPI != nil {
		locationID := s.bomDailyAPI.LocationID()
		run, _ := s.store.StartIngestRun(s.bomDailyAPI.Source(), s.bomDailyAPI.Endpoint(), nil, &locationID)
		targets = append(targets, bomIngestTarget{client: s.bomDailyAPI, run: run, locationID: locationID})
	}
	if len(targets) == 0 {
		return
	}

	log.Println("scheduler: ingesting BOM forecast sources")

	outcomes := make([]bomFetchOutcome, len(targets))
	var wg sync.WaitGroup
	for i := range targets {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			forecasts, rawBody, fetchResult, err := targets[i].client.FetchForecasts()
			outcomes[i] = bomFetchOutcome{
				forecasts:   forecasts,
				rawBody:     rawBody,
				fetchResult: fetchResult,
				err:         err,
			}
		}(i)
	}
	wg.Wait()

	for i, target := range targets {
		outcome := outcomes[i]
		if target.run != nil {
			target.run.Success = outcome.err == nil
			if outcome.fetchResult != nil {
				target.run.HTTPStatus = sql.NullInt64{Int64: int64(outcome.fetchResult.HTTPStatus), Valid: outcome.fetchResult.HTTPStatus > 0}
				target.run.ResponseSizeBytes = sql.NullInt64{Int64: int64(outcome.fetchResult.ResponseSize), Valid: outcome.fetchResult.ResponseSize > 0}
				target.run.RecordsParsed = sql.NullInt64{Int64: int64(outcome.fetchResult.RecordCount), Valid: true}
				if outcome.fetchResult.ParseErrors > 0 {
					target.run.ParseErrors = sql.NullInt64{Int64: int64(outcome.fetchResult.ParseErrors), Valid: true}
					target.run.ErrorMessage = sql.NullString{String: outcome.fetchResult.ParseError, Valid: true}
					log.Printf("scheduler: %s forecast parse errors: %s", target.client.Source(), outcome.fetchResult.ParseError)
				}
			}
			if outcome.err != nil {
				target.run.ErrorMessage = sql.NullString{String: outcome.err.Error(), Valid: true}
			}
		}

		if len(outcome.rawBody) > 0 && target.run != nil {
			if _, err := s.store.StoreRawPayload(&target.run.ID, target.client.Source(), target.client.Endpoint(), nil, &target.locationID, []byte(outcome.rawBody)); err != nil {
				log.Printf("scheduler: store %s raw payload: %v", target.client.Source(), err)
			}
		}

		if outcome.err != nil {
			log.Printf("scheduler: fetch %s forecast: %v", target.client.Source(), outcome.err)
		} else {
			inserted := 0
			for _, fc := range outcome.forecasts {
				if err := s.store.InsertForecast(fc); err != nil {
					log.Printf("scheduler: insert %s forecast: %v", target.client.Source(), err)
					continue
				}
				inserted++
			}
			log.Printf("scheduler: inserted %d %s forecast days", inserted, target.client.Source())
			if target.run != nil {
				target.run.RecordsStored = sql.NullInt64{Int64: int64(inserted), Valid: true}
			}
		}

		if target.run != nil {
			s.store.CompleteIngestRun(target.run)
		}
	}
}

// checkWeatherImage checks if the current time-of-day image is cached and generates if needed.
// Called hourly to handle dawn/day/dusk/night transitions.
func (s *Scheduler) checkWeatherImage() {
	if s.imageGen == nil || s.imageCache == nil {
		return
	}

	// Fetch latest WU forecasts from database
	allForecasts, err := s.store.GetLatestForecasts()
	if err != nil {
		log.Printf("scheduler: failed to get forecasts for image check: %v", err)
		return
	}

	wuForecasts, ok := allForecasts["wu"]
	if !ok || len(wuForecasts) == 0 {
		return
	}

	s.ensureWeatherImage(wuForecasts)
}

// ensureWeatherImage pre-generates weather images for the current time of day.
func (s *Scheduler) ensureWeatherImage(forecasts []models.Forecast) {
	if s.imageGen == nil || s.imageCache == nil {
		return
	}

	now := time.Now().In(s.loc)
	todayDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	tod := forecast.GetTimeOfDay(now)
	smoke := s.currentSmokeLevel(now)

	// Find today's forecast
	var todayForecast *models.Forecast
	for i := range forecasts {
		if forecasts[i].ValidDate.Format("2006-01-02") == todayDate.Format("2006-01-02") {
			todayForecast = &forecasts[i]
			break
		}
	}

	if todayForecast == nil {
		return
	}

	// Extract condition
	narrative := ""
	if todayForecast.Narrative.Valid {
		narrative = todayForecast.Narrative.String
	}
	tempMax := 20.0
	tempMin := 10.0
	if todayForecast.TempMax.Valid {
		tempMax = todayForecast.TempMax.Float64
	}
	if todayForecast.TempMin.Valid {
		tempMin = todayForecast.TempMin.Float64
	}

	baseCondition := forecast.ExtractCondition(narrative, tempMax, tempMin)
	condition := forecast.ConditionWithTimeAndSmoke(baseCondition, tod, smoke)

	// Check cache (quick check before spawning goroutine)
	if _, ok := s.imageCache.Get(condition); ok {
		log.Printf("scheduler: weather image already cached for %s", condition)
		return
	}

	// Generate in background with shared mutex
	go func() {
		if s.imageGenMu != nil {
			s.imageGenMu.Lock()
			defer s.imageGenMu.Unlock()
		}

		// Double-check cache after acquiring lock
		if _, ok := s.imageCache.Get(condition); ok {
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		log.Printf("scheduler: pre-generating weather image for %s", condition)
		data, err := s.imageGen.Generate(ctx, baseCondition, tod, smoke, now)
		if err != nil {
			log.Printf("scheduler: image generation failed: %v", err)
			return
		}

		if err := s.imageCache.Set(condition, data); err != nil {
			log.Printf("scheduler: failed to cache image: %v", err)
			return
		}
		log.Printf("scheduler: cached weather image for %s", condition)
	}()
}

func (s *Scheduler) currentSmokeLevel(now time.Time) forecast.SmokeLevel {
	reading, err := s.store.GetLatestAirQualityReading()
	if err != nil || reading == nil {
		return forecast.SmokeClear
	}
	if now.Sub(reading.ObservedAt) > time.Hour {
		return forecast.SmokeClear
	}
	return forecast.SmokeLevelFromAirQuality(reading.RealTimeAQI, reading.HasRealTimeAQI, reading.PM25)
}

func (s *Scheduler) ingestFireDanger() {
	if s.fireDangerClient == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	forecasts, err := s.fireDangerClient.Fetch(ctx)
	if err != nil {
		log.Printf("scheduler: fetch fire danger: %v", err)
		return
	}

	now := time.Now()
	for _, f := range forecasts {
		if err := s.store.UpsertFireDanger(f, now); err != nil {
			log.Printf("scheduler: upsert fire danger %s: %v", f.Date.Format("2006-01-02"), err)
		}
	}

	if len(forecasts) > 0 {
		// Log today's rating
		today := forecasts[0]
		tfb := ""
		if today.TotalFireBan {
			tfb = " [TOTAL FIRE BAN]"
		}
		log.Printf("scheduler: fire danger %s: %s%s", today.Date.Format("Mon"), today.Rating, tfb)
	}
}

func (s *Scheduler) ingestAlerts() {
	if s.emergencyClient == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	alerts, err := s.emergencyClient.Fetch(ctx)
	if err != nil {
		log.Printf("scheduler: fetch alerts: %v", err)
		return
	}

	now := time.Now()
	inserted := 0
	for _, alert := range alerts {
		if err := s.store.UpsertAlert(alert, now); err != nil {
			log.Printf("scheduler: upsert alert %s: %v", alert.ID, err)
			continue
		}
		inserted++
	}

	if len(alerts) > 0 {
		log.Printf("scheduler: stored %d emergency alerts", inserted)
	}
}

func (s *Scheduler) ingestObservations() {
	log.Println("scheduler: ingesting observations")
	for _, stationID := range s.stationIDs {
		run, _ := s.store.StartIngestRun("wu", "pws/observations/current", &stationID, nil)

		obs, rawJSON, fetchResult, err := s.pws.FetchCurrent(stationID)

		if run != nil {
			run.Success = err == nil
			if fetchResult != nil {
				run.HTTPStatus = sql.NullInt64{Int64: int64(fetchResult.HTTPStatus), Valid: fetchResult.HTTPStatus > 0}
				run.ResponseSizeBytes = sql.NullInt64{Int64: int64(fetchResult.ResponseSize), Valid: fetchResult.ResponseSize > 0}
				run.RecordsParsed = sql.NullInt64{Int64: int64(fetchResult.RecordCount), Valid: true}
			}
			if err != nil {
				run.ErrorMessage = sql.NullString{String: err.Error(), Valid: true}
			}
		}

		if len(rawJSON) > 0 && run != nil {
			if _, err := s.store.StoreRawPayload(&run.ID, "wu", "pws/observations/current", &stationID, nil, []byte(rawJSON)); err != nil {
				log.Printf("scheduler: store PWS raw payload %s: %v", stationID, err)
			}
		}

		if err != nil {
			log.Printf("scheduler: fetch %s: %v", stationID, err)
			if run != nil {
				s.store.CompleteIngestRun(run)
			}
			continue
		}

		obs.RawJSON = rawJSON
		if err := s.store.InsertObservation(*obs); err != nil {
			log.Printf("scheduler: insert %s: %v", stationID, err)
			if run != nil {
				run.Success = false
				run.ErrorMessage = sql.NullString{String: fmt.Sprintf("insert: %v", err), Valid: true}
				s.store.CompleteIngestRun(run)
			}
			continue
		}

		if run != nil {
			run.RecordsStored = sql.NullInt64{Int64: 1, Valid: true}
			s.store.CompleteIngestRun(run)
		}

		if obs.Temp.Valid {
			log.Printf("scheduler: %s: %.1f°C", stationID, obs.Temp.Float64)
		}
	}
}

func (s *Scheduler) ingestAirQuality() {
	if s.airQualityClient == nil {
		return
	}

	run, _ := s.store.StartIngestRun("ecowitt", "device/real_time", nil, nil)

	reading, rawBody, fetchResult, err := s.airQualityClient.FetchCurrentAirQuality()
	if run != nil {
		run.Success = err == nil
		if fetchResult != nil {
			run.HTTPStatus = sql.NullInt64{Int64: int64(fetchResult.HTTPStatus), Valid: fetchResult.HTTPStatus > 0}
			run.ResponseSizeBytes = sql.NullInt64{Int64: int64(fetchResult.ResponseSize), Valid: fetchResult.ResponseSize > 0}
			run.RecordsParsed = sql.NullInt64{Int64: int64(fetchResult.RecordCount), Valid: true}
		}
		if err != nil {
			run.ErrorMessage = sql.NullString{String: err.Error(), Valid: true}
		}
	}

	if len(rawBody) > 0 && run != nil {
		if _, err := s.store.StoreRawPayload(&run.ID, "ecowitt", "device/real_time", nil, nil, []byte(rawBody)); err != nil {
			log.Printf("scheduler: store Ecowitt raw payload: %v", err)
		}
	}

	if err != nil {
		log.Printf("scheduler: fetch Ecowitt air quality: %v", err)
		if run != nil {
			s.store.CompleteIngestRun(run)
		}
		return
	}

	stored, err := s.store.UpsertAirQualityReadings([]ecowitt.AirQualityReading{*reading})
	if err != nil {
		log.Printf("scheduler: store Ecowitt air quality: %v", err)
		if run != nil {
			run.Success = false
			run.ErrorMessage = sql.NullString{String: fmt.Sprintf("store: %v", err), Valid: true}
			s.store.CompleteIngestRun(run)
		}
		return
	}

	if run != nil {
		run.RecordsStored = sql.NullInt64{Int64: int64(stored), Valid: true}
		s.store.CompleteIngestRun(run)
	}
}

func (s *Scheduler) IngestOnce() error {
	s.ingestObservations()
	s.ingestAirQuality()
	s.ingestForecasts()
	s.ingestAlerts()
	s.ingestFireDanger()
	return nil
}

func (s *Scheduler) BackfillHistory7Day() error {
	log.Println("scheduler: backfilling 7-day history (hourly)")
	for _, stationID := range s.stationIDs {
		observations, err := s.pws.FetchHistory7Day(stationID)
		if err != nil {
			log.Printf("scheduler: backfill7d %s: %v", stationID, err)
			continue
		}
		inserted := 0
		for _, obs := range observations {
			if err := s.store.InsertObservation(obs); err != nil {
				log.Printf("scheduler: insert %s: %v", stationID, err)
				continue
			}
			inserted++
		}
		log.Printf("scheduler: backfilled %s: %d hourly observations", stationID, inserted)
	}
	return nil
}

func (s *Scheduler) BackfillAirQualityHistory(days int) error {
	if s.airQualityClient == nil {
		return fmt.Errorf("ecowitt air quality client not configured")
	}
	if days <= 0 {
		return fmt.Errorf("air quality backfill days must be positive")
	}
	if days > 90 {
		return fmt.Errorf("ecowitt WH41 backfill currently supports up to 90 days of 5-minute history")
	}

	end := time.Now().UTC().Truncate(5 * time.Minute)
	start := end.Add(-time.Duration(days) * 24 * time.Hour)
	totalStored := 0

	for chunkStart := start; chunkStart.Before(end); {
		chunkEnd := chunkStart.Add(24 * time.Hour)
		if chunkEnd.After(end) {
			chunkEnd = end
		}

		run, _ := s.store.StartIngestRun("ecowitt", "device/history", nil, nil)
		readings, rawBody, fetchResult, err := s.airQualityClient.FetchAirQualityHistory(chunkStart, chunkEnd, ecowitt.HistoryCycle5Min)

		if run != nil {
			run.Success = err == nil
			if fetchResult != nil {
				run.HTTPStatus = sql.NullInt64{Int64: int64(fetchResult.HTTPStatus), Valid: fetchResult.HTTPStatus > 0}
				run.ResponseSizeBytes = sql.NullInt64{Int64: int64(fetchResult.ResponseSize), Valid: fetchResult.ResponseSize > 0}
				run.RecordsParsed = sql.NullInt64{Int64: int64(fetchResult.RecordCount), Valid: true}
			}
			if err != nil {
				run.ErrorMessage = sql.NullString{String: err.Error(), Valid: true}
			}
		}

		if len(rawBody) > 0 && run != nil {
			if _, err := s.store.StoreRawPayload(&run.ID, "ecowitt", "device/history", nil, nil, []byte(rawBody)); err != nil {
				log.Printf("scheduler: store Ecowitt history payload: %v", err)
			}
		}

		if err != nil {
			if run != nil {
				s.store.CompleteIngestRun(run)
			}
			return fmt.Errorf("fetch Ecowitt air quality history %s to %s: %w", chunkStart.Format(time.RFC3339), chunkEnd.Format(time.RFC3339), err)
		}

		stored, err := s.store.UpsertAirQualityReadings(readings)
		if err != nil {
			if run != nil {
				run.Success = false
				run.ErrorMessage = sql.NullString{String: fmt.Sprintf("store: %v", err), Valid: true}
				s.store.CompleteIngestRun(run)
			}
			return fmt.Errorf("store Ecowitt air quality history %s to %s: %w", chunkStart.Format(time.RFC3339), chunkEnd.Format(time.RFC3339), err)
		}

		totalStored += stored
		if run != nil {
			run.RecordsStored = sql.NullInt64{Int64: int64(stored), Valid: true}
			if err := s.store.CompleteIngestRun(run); err != nil {
				return fmt.Errorf("complete Ecowitt air quality ingest run: %w", err)
			}
		}

		chunkStart = chunkEnd
	}

	log.Printf("scheduler: backfilled %d Ecowitt air quality readings", totalStored)
	return nil
}

func (s *Scheduler) RunDailyJobs() error {
	yesterday := time.Now().AddDate(0, 0, -1)
	return s.daily.RunAll(yesterday)
}

func (s *Scheduler) BackfillDailySummaries() error {
	return s.daily.BackfillSummaries()
}

func (s *Scheduler) BackfillVerification() error {
	return s.daily.BackfillVerification()
}
