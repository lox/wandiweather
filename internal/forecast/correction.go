package forecast

import (
	"time"

	"github.com/lox/wandiweather/internal/store"
)

const (
	// MaxBiasCorrection is the maximum bias correction to apply (exported for consistency across packages)
	MaxBiasCorrection   = 6.0
	maxTotalCorrection  = 10.0
	minRegimeSamples    = 15
	minBiasSamples      = 7
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
	return c.GetCorrectionForRegime(source, target, dayOfForecast, "all")
}

func (c *BiasCorrector) GetCorrectionForRegime(source string, target string, dayOfForecast int, regime string) float64 {
	if regime != "all" && regime != "" {
		stats, err := c.store.GetCorrectionStatsForRegime(source, target, dayOfForecast, regime)
		if err == nil && stats != nil && stats.SampleSize >= minRegimeSamples {
			return capCorrection(stats.MeanBias, MaxBiasCorrection)
		}
	}

	stats, err := c.store.GetCorrectionStats(source, target, dayOfForecast)
	if err != nil || stats == nil {
		return 0
	}
	if stats.SampleSize < minBiasSamples {
		return 0
	}
	return capCorrection(stats.MeanBias, MaxBiasCorrection)
}

func capCorrection(correction float64, limit float64) float64 {
	if correction > limit {
		return limit
	}
	if correction < -limit {
		return -limit
	}
	return correction
}



type CorrectedForecast struct {
	RawMax           float64
	RawMin           float64
	BiasMax          float64
	BiasMin          float64
	CorrectedMax     float64
	CorrectedMin     float64
	NowcastApplied   bool
	NowcastDelta     float64
	NowcastAdjustment float64
	Regime           string
}

func (c *BiasCorrector) ApplyCorrections(
	source string,
	dayOfForecast int,
	rawMax float64,
	rawMin float64,
	regime RegimeFlags,
	nowcast *NowcastCorrection,
) CorrectedForecast {
	regimeStr := RegimeToString(regime)

	biasMax := c.GetCorrectionForRegime(source, "tmax", dayOfForecast, regimeStr)
	biasMin := c.GetCorrectionForRegime(source, "tmin", dayOfForecast, regimeStr)

	correctedMax := rawMax - biasMax
	correctedMin := rawMin - biasMin

	result := CorrectedForecast{
		RawMax:       rawMax,
		RawMin:       rawMin,
		BiasMax:      biasMax,
		BiasMin:      biasMin,
		CorrectedMax: correctedMax,
		CorrectedMin: correctedMin,
		Regime:       regimeStr,
	}

	if nowcast != nil && dayOfForecast == 0 {
		adjustment := capCorrection(nowcast.Adjustment, maxAdjustment)
		correctedMax = rawMax - biasMax + adjustment

		totalCorrection := correctedMax - rawMax
		if totalCorrection > maxTotalCorrection {
			correctedMax = rawMax + maxTotalCorrection
		} else if totalCorrection < -maxTotalCorrection {
			correctedMax = rawMax - maxTotalCorrection
		}

		result.CorrectedMax = correctedMax
		result.NowcastApplied = true
		result.NowcastDelta = nowcast.Delta
		result.NowcastAdjustment = adjustment
	}

	return result
}
