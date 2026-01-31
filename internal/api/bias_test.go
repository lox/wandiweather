package api

import (
	"testing"

	"github.com/lox/wandiweather/internal/forecast"
	"github.com/lox/wandiweather/internal/store"
)

func TestGetCorrectionBiasWithFallback(t *testing.T) {
	tests := []struct {
		name           string
		stats          map[string]map[string]map[int]*store.CorrectionStats
		source         string
		target         string
		dayOfForecast  int
		wantBias       float64
		wantDayUsed    int
		wantSamples    int
		wantIsFallback bool
	}{
		{
			name:           "nil stats returns no correction",
			stats:          nil,
			source:         "bom",
			target:         "tmax",
			dayOfForecast:  1,
			wantBias:       0,
			wantDayUsed:    -1,
			wantSamples:    0,
			wantIsFallback: false,
		},
		{
			name: "exact day with sufficient samples",
			stats: map[string]map[string]map[int]*store.CorrectionStats{
				"bom": {
					"tmax": {
						2: {SampleSize: 28, MeanBias: 2.2},
					},
				},
			},
			source:         "bom",
			target:         "tmax",
			dayOfForecast:  2,
			wantBias:       2.2,
			wantDayUsed:    2,
			wantSamples:    28,
			wantIsFallback: false,
		},
		{
			name: "exact day with insufficient samples falls back",
			stats: map[string]map[string]map[int]*store.CorrectionStats{
				"bom": {
					"tmax": {
						1: {SampleSize: 3, MeanBias: 1.5},  // insufficient
						2: {SampleSize: 28, MeanBias: 2.2}, // sufficient
					},
				},
			},
			source:         "bom",
			target:         "tmax",
			dayOfForecast:  1,
			wantBias:       2.2,
			wantDayUsed:    2,
			wantSamples:    28,
			wantIsFallback: true,
		},
		{
			name: "fallback prefers lower day on tie (day 1 -> day 0 before day 2)",
			stats: map[string]map[string]map[int]*store.CorrectionStats{
				"wu": {
					"tmax": {
						0: {SampleSize: 10, MeanBias: -3.0},
						2: {SampleSize: 30, MeanBias: -4.0},
					},
				},
			},
			source:         "wu",
			target:         "tmax",
			dayOfForecast:  1,
			wantBias:       -3.0,
			wantDayUsed:    0,
			wantSamples:    10,
			wantIsFallback: true,
		},
		{
			name: "fallback to higher day when lower not available",
			stats: map[string]map[string]map[int]*store.CorrectionStats{
				"bom": {
					"tmax": {
						3: {SampleSize: 20, MeanBias: 2.5},
					},
				},
			},
			source:         "bom",
			target:         "tmax",
			dayOfForecast:  1,
			wantBias:       2.5,
			wantDayUsed:    3,
			wantSamples:    20,
			wantIsFallback: true,
		},
		{
			name: "no day has sufficient samples",
			stats: map[string]map[string]map[int]*store.CorrectionStats{
				"bom": {
					"tmax": {
						1: {SampleSize: 3, MeanBias: 1.5},
						2: {SampleSize: 5, MeanBias: 2.0},
					},
				},
			},
			source:         "bom",
			target:         "tmax",
			dayOfForecast:  1,
			wantBias:       0,
			wantDayUsed:    -1,
			wantSamples:    0,
			wantIsFallback: false,
		},
		{
			name: "bias is capped at max",
			stats: map[string]map[string]map[int]*store.CorrectionStats{
				"wu": {
					"tmax": {
						1: {SampleSize: 30, MeanBias: -10.0}, // exceeds cap
					},
				},
			},
			source:         "wu",
			target:         "tmax",
			dayOfForecast:  1,
			wantBias:       -forecast.MaxBiasCorrection,
			wantDayUsed:    1,
			wantSamples:    30,
			wantIsFallback: false,
		},
		{
			name: "positive bias is capped at max",
			stats: map[string]map[string]map[int]*store.CorrectionStats{
				"bom": {
					"tmax": {
						2: {SampleSize: 25, MeanBias: 8.5}, // exceeds cap
					},
				},
			},
			source:         "bom",
			target:         "tmax",
			dayOfForecast:  2,
			wantBias:       forecast.MaxBiasCorrection,
			wantDayUsed:    2,
			wantSamples:    25,
			wantIsFallback: false,
		},
		{
			name: "missing source returns no correction",
			stats: map[string]map[string]map[int]*store.CorrectionStats{
				"bom": {
					"tmax": {
						2: {SampleSize: 28, MeanBias: 2.2},
					},
				},
			},
			source:         "wu",
			target:         "tmax",
			dayOfForecast:  1,
			wantBias:       0,
			wantDayUsed:    -1,
			wantSamples:    0,
			wantIsFallback: false,
		},
		{
			name: "missing target returns no correction",
			stats: map[string]map[string]map[int]*store.CorrectionStats{
				"bom": {
					"tmax": {
						2: {SampleSize: 28, MeanBias: 2.2},
					},
				},
			},
			source:         "bom",
			target:         "tmin",
			dayOfForecast:  2,
			wantBias:       0,
			wantDayUsed:    -1,
			wantSamples:    0,
			wantIsFallback: false,
		},
		{
			name: "day 0 falls back to day 1",
			stats: map[string]map[string]map[int]*store.CorrectionStats{
				"wu": {
					"tmin": {
						0: {SampleSize: 2, MeanBias: 1.0},   // insufficient
						1: {SampleSize: 30, MeanBias: 1.23}, // sufficient
					},
				},
			},
			source:         "wu",
			target:         "tmin",
			dayOfForecast:  0,
			wantBias:       1.23,
			wantDayUsed:    1,
			wantSamples:    30,
			wantIsFallback: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getCorrectionBiasWithFallback(tt.stats, tt.source, tt.target, tt.dayOfForecast)

			if result.Bias != tt.wantBias {
				t.Errorf("Bias = %v, want %v", result.Bias, tt.wantBias)
			}
			if result.DayUsed != tt.wantDayUsed {
				t.Errorf("DayUsed = %v, want %v", result.DayUsed, tt.wantDayUsed)
			}
			if result.Samples != tt.wantSamples {
				t.Errorf("Samples = %v, want %v", result.Samples, tt.wantSamples)
			}
			if result.IsFallback != tt.wantIsFallback {
				t.Errorf("IsFallback = %v, want %v", result.IsFallback, tt.wantIsFallback)
			}
		})
	}
}

func TestGetCorrectionBias_BackwardCompatibility(t *testing.T) {
	stats := map[string]map[string]map[int]*store.CorrectionStats{
		"bom": {
			"tmax": {
				2: {SampleSize: 28, MeanBias: 2.2},
			},
		},
	}

	// Test that the old function still works
	bias := getCorrectionBias(stats, "bom", "tmax", 2)
	if bias != 2.2 {
		t.Errorf("getCorrectionBias = %v, want 2.2", bias)
	}

	// Test fallback works through old function too
	bias = getCorrectionBias(stats, "bom", "tmax", 1)
	if bias != 2.2 {
		t.Errorf("getCorrectionBias with fallback = %v, want 2.2", bias)
	}
}
