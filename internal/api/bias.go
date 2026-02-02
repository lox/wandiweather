package api

import (
	"github.com/lox/wandiweather/internal/forecast"
	"github.com/lox/wandiweather/internal/store"
)

const minBiasSamples = 7

// BiasResult contains the bias correction and metadata about how it was determined.
type BiasResult struct {
	Bias       float64
	DayUsed    int  // which day's stats were used (-1 if none)
	Samples    int  // sample size the bias is based on
	IsFallback bool // true if a fallback day was used
}

// getCorrectionBiasWithFallback returns the bias correction for a source/target/day,
// falling back to nearby days if the exact day doesn't have enough samples.
func getCorrectionBiasWithFallback(stats map[string]map[string]map[int]*store.CorrectionStats, source, target string, dayOfForecast int) BiasResult {
	if stats == nil || stats[source] == nil || stats[source][target] == nil {
		return BiasResult{DayUsed: -1}
	}

	targetStats := stats[source][target]

	// First, try the exact day
	if s := targetStats[dayOfForecast]; s != nil && s.SampleSize >= minBiasSamples {
		bias := s.MeanBias
		if bias > forecast.MaxBiasCorrection {
			bias = forecast.MaxBiasCorrection
		} else if bias < -forecast.MaxBiasCorrection {
			bias = -forecast.MaxBiasCorrection
		}
		return BiasResult{
			Bias:       bias,
			DayUsed:    dayOfForecast,
			Samples:    s.SampleSize,
			IsFallback: false,
		}
	}

	// Fallback: find the nearest day with sufficient samples
	// Search nearby days (prefer closer days, then lower days on tie)
	searchOrder := []int{}
	for delta := 1; delta <= 14; delta++ {
		// Try lower day first (prefer earlier lead times on tie)
		if dayOfForecast-delta >= 0 {
			searchOrder = append(searchOrder, dayOfForecast-delta)
		}
		if dayOfForecast+delta <= 14 {
			searchOrder = append(searchOrder, dayOfForecast+delta)
		}
	}

	for _, day := range searchOrder {
		if s := targetStats[day]; s != nil && s.SampleSize >= minBiasSamples {
			bias := s.MeanBias
			if bias > forecast.MaxBiasCorrection {
				bias = forecast.MaxBiasCorrection
			} else if bias < -forecast.MaxBiasCorrection {
				bias = -forecast.MaxBiasCorrection
			}
			return BiasResult{
				Bias:       bias,
				DayUsed:    day,
				Samples:    s.SampleSize,
				IsFallback: true,
			}
		}
	}

	return BiasResult{DayUsed: -1}
}

// getCorrectionBias is kept for backward compatibility with other parts of the code.
func getCorrectionBias(stats map[string]map[string]map[int]*store.CorrectionStats, source, target string, dayOfForecast int) float64 {
	result := getCorrectionBiasWithFallback(stats, source, target, dayOfForecast)
	return result.Bias
}
