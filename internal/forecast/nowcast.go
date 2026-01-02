package forecast

import (
	"database/sql"
	"time"

	"github.com/lox/wandiweather/internal/store"
)

const (
	nowcastAlpha     = 0.7
	maxAdjustment    = 4.0
	nowcastStartHour = 10
	nowcastEndHour   = 11
	minReadings      = 6
	nowcastEnabled   = false // Disabled until we have data to validate
)

type NowcastCorrection struct {
	ObservedMorning float64
	ForecastMorning float64
	Delta           float64
	Adjustment      float64
	CorrectedMax    float64
	AppliedAt       time.Time
}

type Nowcaster struct {
	store *store.Store
	loc   *time.Location
}

func NewNowcaster(s *store.Store, loc *time.Location) *Nowcaster {
	return &Nowcaster{store: s, loc: loc}
}

func (n *Nowcaster) ComputeNowcast(
	stationID string,
	forecastMax float64,
	biasCorrection float64,
) (*NowcastCorrection, error) {
	if !nowcastEnabled {
		return nil, nil
	}

	now := time.Now().In(n.loc)

	if now.Hour() < nowcastStartHour {
		return nil, nil
	}

	obs, err := n.store.GetMorningObservations(stationID, now)
	if err != nil {
		return nil, err
	}
	if len(obs) < minReadings {
		return nil, nil
	}

	var sum float64
	var count int
	for _, o := range obs {
		if o.Temp.Valid {
			sum += o.Temp.Float64
			count++
		}
	}
	if count == 0 {
		return nil, nil
	}
	observedMorning := sum / float64(count)

	forecastMorning := forecastMax * 0.7

	delta := observedMorning - forecastMorning
	adjustment := nowcastAlpha * delta

	adjustment = capCorrection(adjustment, maxAdjustment)

	correctedMax := forecastMax - biasCorrection + adjustment

	return &NowcastCorrection{
		ObservedMorning: observedMorning,
		ForecastMorning: forecastMorning,
		Delta:           delta,
		Adjustment:      adjustment,
		CorrectedMax:    correctedMax,
		AppliedAt:       now,
	}, nil
}

func (n *Nowcaster) LogNowcast(stationID string, forecastMaxRaw float64, correction *NowcastCorrection) error {
	if correction == nil {
		return nil
	}

	today := time.Now().In(n.loc)
	dateOnly := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)

	log := store.NowcastLog{
		Date:      dateOnly,
		StationID: stationID,
		ObservedMorning: sql.NullFloat64{
			Float64: correction.ObservedMorning,
			Valid:   true,
		},
		ForecastMorning: sql.NullFloat64{
			Float64: correction.ForecastMorning,
			Valid:   true,
		},
		Delta: sql.NullFloat64{
			Float64: correction.Delta,
			Valid:   true,
		},
		Adjustment: sql.NullFloat64{
			Float64: correction.Adjustment,
			Valid:   true,
		},
		ForecastMaxRaw: sql.NullFloat64{
			Float64: forecastMaxRaw,
			Valid:   true,
		},
		ForecastMaxCorrected: sql.NullFloat64{
			Float64: correction.CorrectedMax,
			Valid:   true,
		},
	}

	return n.store.UpsertNowcastLog(log)
}
