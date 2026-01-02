package ingest

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/lox/wandiweather/internal/forecast"
	"github.com/lox/wandiweather/internal/imagegen"
	"github.com/lox/wandiweather/internal/models"
	"github.com/lox/wandiweather/internal/store"
)

type Scheduler struct {
	store       *store.Store
	pws         *PWS
	forecast    *ForecastClient
	bom         *BOMClient
	daily       *DailyJobs
	stationIDs  []string
	loc         *time.Location
	obsInterval time.Duration
	fcInterval  time.Duration
	imageGen    *imagegen.Generator
	imageCache  *imagegen.Cache
	imageGenMu  *sync.Mutex // Shared with server to prevent duplicate API calls
}

func NewScheduler(store *store.Store, pws *PWS, forecast *ForecastClient, stationIDs []string, loc *time.Location) *Scheduler {
	return &Scheduler{
		store:       store,
		pws:         pws,
		forecast:    forecast,
		bom:         NewBOMClient(""),
		daily:       NewDailyJobs(store),
		stationIDs:  stationIDs,
		loc:         loc,
		obsInterval: 5 * time.Minute,
		fcInterval:  6 * time.Hour,
	}
}

// SetImageGenerator configures the scheduler to pre-generate weather images after forecast ingestion.
// The mutex should be shared with the HTTP server to coordinate generation and prevent duplicate API calls.
func (s *Scheduler) SetImageGenerator(gen *imagegen.Generator, cache *imagegen.Cache, mu *sync.Mutex) {
	s.imageGen = gen
	s.imageCache = cache
	s.imageGenMu = mu
}

func (s *Scheduler) Run(ctx context.Context) {
	s.ingestObservations()
	s.ingestForecasts()
	s.runDailyJobsIfNeeded()
	s.checkWeatherImage()

	obsTicker := time.NewTicker(s.obsInterval)
	fcTicker := time.NewTicker(s.fcInterval)
	dailyTicker := time.NewTicker(1 * time.Hour)
	imageTicker := time.NewTicker(1 * time.Hour) // Check hourly for time-of-day transitions
	defer obsTicker.Stop()
	defer fcTicker.Stop()
	defer dailyTicker.Stop()
	defer imageTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("scheduler: shutting down")
			return
		case <-obsTicker.C:
			s.ingestObservations()
		case <-fcTicker.C:
			s.ingestForecasts()
		case <-dailyTicker.C:
			s.runDailyJobsIfNeeded()
		case <-imageTicker.C:
			s.checkWeatherImage()
		}
	}
}

func (s *Scheduler) runDailyJobsIfNeeded() {
	now := time.Now()
	localNow := now.In(s.loc)

	if localNow.Hour() >= 6 && localNow.Hour() < 7 {
		yesterday := localNow.AddDate(0, 0, -1)
		s.daily.RunAll(yesterday)
	}
}

func (s *Scheduler) ingestForecasts() {
	if s.forecast == nil {
		return
	}
	log.Println("scheduler: ingesting WU forecasts")
	forecasts, _, err := s.forecast.Fetch7Day()
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
	}

	if s.bom != nil {
		log.Println("scheduler: ingesting BOM forecasts")
		bomForecasts, _, err := s.bom.FetchForecasts()
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
		}
	}

	// Pre-generate weather image for current condition
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

func (s *Scheduler) ingestObservations() {
	log.Println("scheduler: ingesting observations")
	for _, stationID := range s.stationIDs {
		obs, rawJSON, err := s.pws.FetchCurrent(stationID)
		if err != nil {
			log.Printf("scheduler: fetch %s: %v", stationID, err)
			continue
		}
		obs.RawJSON = rawJSON
		if err := s.store.InsertObservation(*obs); err != nil {
			log.Printf("scheduler: insert %s: %v", stationID, err)
			continue
		}
		if obs.Temp.Valid {
			log.Printf("scheduler: %s: %.1f°C", stationID, obs.Temp.Float64)
		}
	}
}

func (s *Scheduler) IngestOnce() error {
	s.ingestObservations()
	s.ingestForecasts()
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
