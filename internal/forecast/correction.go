package forecast

import (
	"time"

	"github.com/lox/wandiweather/internal/store"
)

type BiasCorrector struct {
	store *store.Store
}

func NewBiasCorrector(s *store.Store) *BiasCorrector {
	return &BiasCorrector{store: s}
}

func (c *BiasCorrector) ComputeStats(windowDays int) error {
	rows, err := c.store.GetBiasStatsFromVerification(windowDays)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	for _, row := range rows {
		if row.CountMax > 0 {
			stats := store.CorrectionStats{
				Source:        row.Source,
				Target:        "tmax",
				DayOfForecast: row.DayOfForecast,
				Regime:        "all",
				WindowDays:    windowDays,
				SampleSize:    row.CountMax,
				MeanBias:      row.AvgBiasMax,
				MAE:           row.MAEMax,
				UpdatedAt:     now,
			}
			if err := c.store.UpsertCorrectionStats(stats); err != nil {
				return err
			}
		}

		if row.CountMin > 0 {
			stats := store.CorrectionStats{
				Source:        row.Source,
				Target:        "tmin",
				DayOfForecast: row.DayOfForecast,
				Regime:        "all",
				WindowDays:    windowDays,
				SampleSize:    row.CountMin,
				MeanBias:      row.AvgBiasMin,
				MAE:           row.MAEMin,
				UpdatedAt:     now,
			}
			if err := c.store.UpsertCorrectionStats(stats); err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *BiasCorrector) GetCorrection(source string, target string, dayOfForecast int) float64 {
	stats, err := c.store.GetCorrectionStats(source, target, dayOfForecast)
	if err != nil || stats == nil {
		return 0
	}
	return stats.MeanBias
}
