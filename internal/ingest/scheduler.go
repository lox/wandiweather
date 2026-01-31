package ingest

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

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
	daily            *DailyJobs
	stationIDs       []string
	loc              *time.Location
	obsInterval      time.Duration
	imageGen         *imagegen.Generator
	imageCache       *imagegen.Cache
	imageGenMu       *sync.Mutex // Shared with server to prevent duplicate API calls
	emergencyClient  *emergency.Client
	fireDangerClient *firedanger.Client
	cron             *cron.Cron
}

func NewScheduler(store *store.Store, pws *PWS, forecast *ForecastClient, stationIDs []string, loc *time.Location) *Scheduler {
	return &Scheduler{
		store:           store,
		pws:             pws,
		forecast:        forecast,
		bom:             NewBOMClient(""),
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

func (s *Scheduler) Run(ctx context.Context) {
	// Initial ingestion on startup
	s.ingestObservations()
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

	if s.bom != nil {
		log.Println("scheduler: ingesting BOM forecasts")
		bomRun, _ := s.store.StartIngestRun("bom", "forecast/fwo", nil, &s.bom.areaCode)
		bomForecasts, bomRawBody, bomFetchResult, err := s.bom.FetchForecasts()

		if bomRun != nil {
			bomRun.Success = err == nil
			if bomFetchResult != nil {
				bomRun.HTTPStatus = sql.NullInt64{Int64: int64(bomFetchResult.HTTPStatus), Valid: bomFetchResult.HTTPStatus > 0}
				bomRun.ResponseSizeBytes = sql.NullInt64{Int64: int64(bomFetchResult.ResponseSize), Valid: bomFetchResult.ResponseSize > 0}
				bomRun.RecordsParsed = sql.NullInt64{Int64: int64(bomFetchResult.RecordCount), Valid: true}
				if bomFetchResult.ParseErrors > 0 {
					bomRun.ParseErrors = sql.NullInt64{Int64: int64(bomFetchResult.ParseErrors), Valid: true}
					bomRun.ErrorMessage = sql.NullString{String: bomFetchResult.ParseError, Valid: true}
					log.Printf("scheduler: BOM forecast parse errors: %s", bomFetchResult.ParseError)
				}
			}
			if err != nil {
				bomRun.ErrorMessage = sql.NullString{String: err.Error(), Valid: true}
			}
		}

		if len(bomRawBody) > 0 && bomRun != nil {
			if _, err := s.store.StoreRawPayload(&bomRun.ID, "bom", "forecast/fwo", nil, &s.bom.areaCode, []byte(bomRawBody)); err != nil {
				log.Printf("scheduler: store BOM raw payload: %v", err)
			}
		}

		if err != nil {
			log.Printf("scheduler: fetch BOM forecast: %v", err)
		} else {
			inserted := 0
			for _, fc := range bomForecasts {
				if err := s.store.InsertForecast(fc); err != nil {
					log.Printf("scheduler: insert BOM forecast: %v", err)
					continue
				}
				inserted++
			}
			log.Printf("scheduler: inserted %d BOM forecast days", inserted)
			if bomRun != nil {
				bomRun.RecordsStored = sql.NullInt64{Int64: int64(inserted), Valid: true}
			}
		}

		if bomRun != nil {
			s.store.CompleteIngestRun(bomRun)
		}
	}

	s.ensureWeatherImage(forecasts)
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
	condition := forecast.ConditionWithTime(baseCondition, tod)

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
		data, err := s.imageGen.Generate(ctx, baseCondition, tod, now)
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

func (s *Scheduler) IngestOnce() error {
	s.ingestObservations()
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
