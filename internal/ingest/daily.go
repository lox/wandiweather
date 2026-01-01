package ingest

import (
	"database/sql"
	"log"
	"time"

	"github.com/lox/wandiweather/internal/forecast"
	"github.com/lox/wandiweather/internal/models"
	"github.com/lox/wandiweather/internal/store"
)

type DailyJobs struct {
	store *store.Store
}

func NewDailyJobs(store *store.Store) *DailyJobs {
	return &DailyJobs{store: store}
}

func (d *DailyJobs) RunAll(forDate time.Time) error {
	log.Printf("daily: running jobs for %s", forDate.Format("2006-01-02"))

	if err := d.ComputeDailySummaries(forDate); err != nil {
		log.Printf("daily: summaries error: %v", err)
	}

	if err := d.VerifyForecasts(forDate); err != nil {
		log.Printf("daily: verification error: %v", err)
	}

	corrector := forecast.NewBiasCorrector(d.store)
	if err := corrector.ComputeStats(30); err != nil {
		log.Printf("daily: correction stats error: %v", err)
	}

	return nil
}

func (d *DailyJobs) ComputeDailySummaries(forDate time.Time) error {
	stations, err := d.store.GetActiveStations()
	if err != nil {
		return err
	}

	overnightMins, err := d.store.GetOvernightMinByTier(forDate)
	if err != nil {
		log.Printf("daily: failed to get overnight mins: %v", err)
	}

	var inversionDetected bool
	var inversionStrength float64
	if valleyMin, ok := overnightMins["valley_floor"]; ok {
		if upperMin, ok := overnightMins["upper"]; ok {
			inversionStrength = upperMin - valleyMin
			inversionDetected = inversionStrength > 1.0
			if inversionDetected {
				log.Printf("daily: inversion detected on %s: valley=%.1f°C upper=%.1f°C strength=%.1f°C",
					forDate.Format("2006-01-02"), valleyMin, upperMin, inversionStrength)
			}
		}
	}

	computed := 0
	for _, station := range stations {
		summary, err := d.store.ComputeDailySummary(station.StationID, forDate)
		if err != nil {
			log.Printf("daily: compute summary %s: %v", station.StationID, err)
			continue
		}
		if summary == nil || !summary.TempMax.Valid {
			continue
		}

		if station.ElevationTier == "valley_floor" {
			summary.InversionDetected = sql.NullBool{Bool: inversionDetected, Valid: true}
			summary.InversionStrength = sql.NullFloat64{Float64: inversionStrength, Valid: inversionDetected}
		}

		if err := d.store.UpsertDailySummary(*summary); err != nil {
			log.Printf("daily: upsert summary %s: %v", station.StationID, err)
			continue
		}
		computed++
	}

	log.Printf("daily: computed %d summaries for %s", computed, forDate.Format("2006-01-02"))
	return nil
}

func (d *DailyJobs) VerifyForecasts(forDate time.Time) error {
	hasVerification, err := d.store.HasVerificationForDate(forDate)
	if err != nil {
		return err
	}
	if hasVerification {
		log.Printf("daily: verification already exists for %s", forDate.Format("2006-01-02"))
		return nil
	}

	primary, err := d.store.GetPrimaryStation()
	if err != nil {
		return err
	}
	if primary == nil {
		log.Println("daily: no primary station configured")
		return nil
	}

	actuals, err := d.store.GetActualsForDate(primary.StationID, forDate)
	if err != nil {
		return err
	}
	if !actuals.TempMax.Valid || !actuals.TempMin.Valid {
		log.Printf("daily: no actuals for %s on %s", primary.StationID, forDate.Format("2006-01-02"))
		return nil
	}

	forecasts, err := d.store.GetForecastsForDate(forDate)
	if err != nil {
		return err
	}

	seen := make(map[string]bool)
	verified := 0
	for _, fc := range forecasts {
		if seen[fc.Source] {
			continue
		}
		seen[fc.Source] = true

		v := models.ForecastVerification{
			ForecastID:     fc.ID,
			ValidDate:      forDate,
			ActualTempMax:  actuals.TempMax,
			ActualTempMin:  actuals.TempMin,
			ActualWindGust: actuals.WindGust,
			ActualPrecip:   actuals.PrecipSum,
		}

		if fc.TempMax.Valid {
			v.ForecastTempMax = fc.TempMax
			v.BiasTempMax = sql.NullFloat64{
				Float64: fc.TempMax.Float64 - actuals.TempMax.Float64,
				Valid:   true,
			}
		}
		if fc.TempMin.Valid {
			v.ForecastTempMin = fc.TempMin
			v.BiasTempMin = sql.NullFloat64{
				Float64: fc.TempMin.Float64 - actuals.TempMin.Float64,
				Valid:   true,
			}
		}
		if fc.WindSpeed.Valid && actuals.WindGust.Valid {
			v.ForecastWindSpeed = fc.WindSpeed
			v.BiasWind = sql.NullFloat64{
				Float64: fc.WindSpeed.Float64 - actuals.WindGust.Float64,
				Valid:   true,
			}
		}
		if fc.PrecipAmount.Valid && actuals.PrecipSum.Valid {
			v.ForecastPrecip = fc.PrecipAmount
			v.BiasPrecip = sql.NullFloat64{
				Float64: fc.PrecipAmount.Float64 - actuals.PrecipSum.Float64,
				Valid:   true,
			}
		}

		if err := d.store.InsertForecastVerification(v); err != nil {
			log.Printf("daily: insert verification: %v", err)
			continue
		}

		log.Printf("daily: verified %s forecast for %s: temp bias=%.1f/%.1f°C, wind bias=%.1f km/h, precip bias=%.1fmm",
			fc.Source, forDate.Format("2006-01-02"),
			v.BiasTempMax.Float64, v.BiasTempMin.Float64,
			v.BiasWind.Float64, v.BiasPrecip.Float64)
		verified++
	}

	log.Printf("daily: verified %d forecasts for %s", verified, forDate.Format("2006-01-02"))
	return nil
}

func (d *DailyJobs) BackfillSummaries() error {
	log.Println("daily: backfilling all daily summaries")

	stations, err := d.store.GetActiveStations()
	if err != nil {
		return err
	}

	if len(stations) == 0 {
		return nil
	}

	dates, err := d.store.GetObservationDates(stations[0].StationID)
	if err != nil {
		return err
	}

	for _, date := range dates {
		if err := d.ComputeDailySummaries(date); err != nil {
			log.Printf("daily: backfill %s: %v", date.Format("2006-01-02"), err)
		}
	}

	return nil
}

func (d *DailyJobs) BackfillVerification() error {
	log.Println("daily: backfilling forecast verification")

	primary, err := d.store.GetPrimaryStation()
	if err != nil {
		return err
	}
	if primary == nil {
		log.Println("daily: no primary station")
		return nil
	}

	dates, err := d.store.GetObservationDates(primary.StationID)
	if err != nil {
		return err
	}

	for _, date := range dates {
		if date.After(time.Now().Add(-24 * time.Hour)) {
			continue
		}
		if err := d.VerifyForecasts(date); err != nil {
			log.Printf("daily: verify %s: %v", date.Format("2006-01-02"), err)
		}
	}

	corrector := forecast.NewBiasCorrector(d.store)
	if err := corrector.ComputeStats(30); err != nil {
		log.Printf("daily: correction stats error: %v", err)
	}

	return nil
}
