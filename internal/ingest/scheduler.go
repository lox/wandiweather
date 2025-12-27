package ingest

import (
	"context"
	"log"
	"time"

	"github.com/lox/wandiweather/internal/store"
)

type Scheduler struct {
	store       *store.Store
	pws         *PWS
	forecast    *ForecastClient
	daily       *DailyJobs
	stationIDs  []string
	obsInterval time.Duration
	fcInterval  time.Duration
}

func NewScheduler(store *store.Store, pws *PWS, forecast *ForecastClient, stationIDs []string) *Scheduler {
	return &Scheduler{
		store:       store,
		pws:         pws,
		forecast:    forecast,
		daily:       NewDailyJobs(store),
		stationIDs:  stationIDs,
		obsInterval: 5 * time.Minute,
		fcInterval:  6 * time.Hour,
	}
}

func (s *Scheduler) Run(ctx context.Context) {
	s.ingestObservations()
	s.ingestForecasts()
	s.runDailyJobsIfNeeded()

	obsTicker := time.NewTicker(s.obsInterval)
	fcTicker := time.NewTicker(s.fcInterval)
	dailyTicker := time.NewTicker(1 * time.Hour)
	defer obsTicker.Stop()
	defer fcTicker.Stop()
	defer dailyTicker.Stop()

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
		}
	}
}

func (s *Scheduler) runDailyJobsIfNeeded() {
	now := time.Now()
	loc, _ := time.LoadLocation("Australia/Melbourne")
	localNow := now.In(loc)

	if localNow.Hour() >= 6 && localNow.Hour() < 7 {
		yesterday := localNow.AddDate(0, 0, -1)
		s.daily.RunAll(yesterday)
	}
}

func (s *Scheduler) ingestForecasts() {
	if s.forecast == nil {
		return
	}
	log.Println("scheduler: ingesting forecasts")
	forecasts, _, err := s.forecast.Fetch7Day()
	if err != nil {
		log.Printf("scheduler: fetch forecast: %v", err)
		return
	}
	for _, fc := range forecasts {
		if err := s.store.InsertForecast(fc); err != nil {
			log.Printf("scheduler: insert forecast: %v", err)
			continue
		}
	}
	log.Printf("scheduler: inserted %d forecast days", len(forecasts))
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
