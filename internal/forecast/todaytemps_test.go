package forecast

import (
	"database/sql"
	"testing"

	"github.com/lox/wandiweather/internal/models"
	"github.com/lox/wandiweather/internal/store"
)

func TestComputeTodayTemps(t *testing.T) {
	tests := []struct {
		name    string
		input   TodayTempInput
		wantMax float64
		wantMin float64
		haveMax bool
		haveMin bool
	}{
		{
			name: "prefers BOM over WU for max",
			input: TodayTempInput{
				BOMForecast: &models.Forecast{TempMax: sql.NullFloat64{Float64: 28, Valid: true}},
				WUForecast:  &models.Forecast{TempMax: sql.NullFloat64{Float64: 30, Valid: true}},
			},
			wantMax: 28,
			haveMax: true,
		},
		{
			name: "prefers WU over BOM for min",
			input: TodayTempInput{
				WUForecast:  &models.Forecast{TempMin: sql.NullFloat64{Float64: 10, Valid: true}},
				BOMForecast: &models.Forecast{TempMin: sql.NullFloat64{Float64: 8, Valid: true}},
			},
			wantMin: 10,
			haveMin: true,
		},
		{
			name: "falls back to WU when current exceeds BOM by >3",
			input: TodayTempInput{
				BOMForecast:    &models.Forecast{TempMax: sql.NullFloat64{Float64: 25, Valid: true}},
				WUForecast:     &models.Forecast{TempMax: sql.NullFloat64{Float64: 30, Valid: true}},
				CurrentTemp:    29,
				HasCurrentTemp: true,
			},
			wantMax: 30,
			haveMax: true,
		},
		{
			name: "falls back to WU when WU exceeds BOM by >10",
			input: TodayTempInput{
				BOMForecast: &models.Forecast{TempMax: sql.NullFloat64{Float64: 20, Valid: true}},
				WUForecast:  &models.Forecast{TempMax: sql.NullFloat64{Float64: 31, Valid: true}},
			},
			wantMax: 31,
			haveMax: true,
		},
		{
			name: "falls back to WU when BOM exceeds WU by >10",
			input: TodayTempInput{
				BOMForecast: &models.Forecast{TempMax: sql.NullFloat64{Float64: 35, Valid: true}},
				WUForecast:  &models.Forecast{TempMax: sql.NullFloat64{Float64: 24, Valid: true}},
			},
			wantMax: 24,
			haveMax: true,
		},
		{
			name: "uses observed max as floor",
			input: TodayTempInput{
				BOMForecast:      &models.Forecast{TempMax: sql.NullFloat64{Float64: 25, Valid: true}},
				ObservedMax:      27.3,
				ObservedMaxValid: true,
			},
			wantMax: 27,
			haveMax: true,
		},
		{
			name: "uses observed min as ceiling",
			input: TodayTempInput{
				WUForecast:       &models.Forecast{TempMin: sql.NullFloat64{Float64: 12, Valid: true}},
				ObservedMin:      10.2,
				ObservedMinValid: true,
			},
			wantMin: 10,
			haveMin: true,
		},
		{
			name: "after 3pm with falling temp uses observed max",
			input: TodayTempInput{
				BOMForecast:      &models.Forecast{TempMax: sql.NullFloat64{Float64: 30, Valid: true}},
				ObservedMax:      28.6,
				ObservedMaxValid: true,
				Hour:             16,
				TempFalling:      true,
			},
			wantMax: 29,
			haveMax: true,
		},
		{
			name: "before 3pm does not use observed max even if falling",
			input: TodayTempInput{
				BOMForecast:      &models.Forecast{TempMax: sql.NullFloat64{Float64: 30, Valid: true}},
				ObservedMax:      26,
				ObservedMaxValid: true,
				Hour:             14,
				TempFalling:      true,
			},
			wantMax: 30,
			haveMax: true,
		},
		{
			name: "applies bias correction to max",
			input: TodayTempInput{
				BOMForecast: &models.Forecast{
					TempMax:       sql.NullFloat64{Float64: 30, Valid: true},
					DayOfForecast: 0,
				},
				CorrectionStats: map[string]map[string]map[int]*store.CorrectionStats{
					"bom": {
						"tmax": {
							0: {MeanBias: 2.0, SampleSize: 10},
						},
					},
				},
			},
			wantMax: 28, // 30 - 2.0 bias
			haveMax: true,
		},
		{
			name: "applies bias correction to min",
			input: TodayTempInput{
				WUForecast: &models.Forecast{
					TempMin:       sql.NullFloat64{Float64: 10, Valid: true},
					DayOfForecast: 0,
				},
				CorrectionStats: map[string]map[string]map[int]*store.CorrectionStats{
					"wu": {
						"tmin": {
							0: {MeanBias: -1.5, SampleSize: 10},
						},
					},
				},
			},
			wantMin: 12, // 10 - (-1.5) = 11.5, rounded to 12
			haveMin: true,
		},
		{
			name: "falls back to nearby day for bias",
			input: TodayTempInput{
				BOMForecast: &models.Forecast{
					TempMax:       sql.NullFloat64{Float64: 30, Valid: true},
					DayOfForecast: 2,
				},
				CorrectionStats: map[string]map[string]map[int]*store.CorrectionStats{
					"bom": {
						"tmax": {
							1: {MeanBias: 1.5, SampleSize: 10}, // Day 1 has data, day 2 doesn't
						},
					},
				},
			},
			wantMax: 29, // 30 - 1.5 = 28.5, rounds to 29
			haveMax: true,
		},
		{
			name: "caps bias correction at max",
			input: TodayTempInput{
				BOMForecast: &models.Forecast{
					TempMax:       sql.NullFloat64{Float64: 30, Valid: true},
					DayOfForecast: 0,
				},
				CorrectionStats: map[string]map[string]map[int]*store.CorrectionStats{
					"bom": {
						"tmax": {
							0: {MeanBias: 10.0, SampleSize: 10}, // Exceeds MaxBiasCorrection
						},
					},
				},
			},
			wantMax: 24, // 30 - 6.0 (capped)
			haveMax: true,
		},
		{
			name: "no forecast data returns zero values",
			input: TodayTempInput{},
		},
		{
			name: "does not use observed min when invalid even if zero",
			input: TodayTempInput{
				WUForecast:       &models.Forecast{TempMin: sql.NullFloat64{Float64: 8, Valid: true}},
				ObservedMin:      0,
				ObservedMinValid: false,
			},
			wantMin: 8,
			haveMin: true,
		},
		{
			name: "does not use observed max when invalid even if zero",
			input: TodayTempInput{
				BOMForecast:      &models.Forecast{TempMax: sql.NullFloat64{Float64: 25, Valid: true}},
				ObservedMax:      0,
				ObservedMaxValid: false,
			},
			wantMax: 25,
			haveMax: true,
		},
		{
			name: "falls back to BOM for min when WU unavailable",
			input: TodayTempInput{
				BOMForecast: &models.Forecast{TempMin: sql.NullFloat64{Float64: 8, Valid: true}},
			},
			wantMin: 8,
			haveMin: true,
		},
		{
			name: "falls back to WU for max when BOM unavailable",
			input: TodayTempInput{
				WUForecast: &models.Forecast{TempMax: sql.NullFloat64{Float64: 25, Valid: true}},
			},
			wantMax: 25,
			haveMax: true,
		},
		{
			name: "rejects overcorrection that exceeds both raw and observed by >3",
			input: TodayTempInput{
				BOMForecast: &models.Forecast{
					TempMax:       sql.NullFloat64{Float64: 25, Valid: true},
					DayOfForecast: 0,
				},
				CorrectionStats: map[string]map[string]map[int]*store.CorrectionStats{
					"bom": {
						"tmax": {
							0: {MeanBias: -6.0, SampleSize: 10}, // Would push to 31
						},
					},
				},
				ObservedMax:      26,
				ObservedMaxValid: true,
			},
			wantMax: 26, // Falls back to observed (higher than raw 25)
			haveMax: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ComputeTodayTemps(tt.input)

			if result.HaveMax != tt.haveMax {
				t.Errorf("HaveMax = %v, want %v", result.HaveMax, tt.haveMax)
			}
			if result.HaveMin != tt.haveMin {
				t.Errorf("HaveMin = %v, want %v", result.HaveMin, tt.haveMin)
			}
			if tt.haveMax && result.TempMax != tt.wantMax {
				t.Errorf("TempMax = %v, want %v", result.TempMax, tt.wantMax)
			}
			if tt.haveMin && result.TempMin != tt.wantMin {
				t.Errorf("TempMin = %v, want %v", result.TempMin, tt.wantMin)
			}
		})
	}
}

func TestComputeTodayTemps_Explanation(t *testing.T) {
	input := TodayTempInput{
		BOMForecast: &models.Forecast{
			TempMax:       sql.NullFloat64{Float64: 30, Valid: true},
			DayOfForecast: 0,
		},
		WUForecast: &models.Forecast{
			TempMin:       sql.NullFloat64{Float64: 12, Valid: true},
			DayOfForecast: 0,
		},
		CorrectionStats: map[string]map[string]map[int]*store.CorrectionStats{
			"bom": {
				"tmax": {0: {MeanBias: 2.0, SampleSize: 15}},
			},
			"wu": {
				"tmin": {0: {MeanBias: -1.0, SampleSize: 10}},
			},
		},
	}

	result := ComputeTodayTemps(input)

	if result.Explanation.MaxSource != "bom" {
		t.Errorf("MaxSource = %q, want %q", result.Explanation.MaxSource, "bom")
	}
	if result.Explanation.MaxRaw != 30 {
		t.Errorf("MaxRaw = %v, want %v", result.Explanation.MaxRaw, 30)
	}
	if result.Explanation.MaxBiasApplied != 2.0 {
		t.Errorf("MaxBiasApplied = %v, want %v", result.Explanation.MaxBiasApplied, 2.0)
	}
	if result.Explanation.MaxBiasDayUsed != 0 {
		t.Errorf("MaxBiasDayUsed = %v, want %v", result.Explanation.MaxBiasDayUsed, 0)
	}
	if result.Explanation.MaxBiasSamples != 15 {
		t.Errorf("MaxBiasSamples = %v, want %v", result.Explanation.MaxBiasSamples, 15)
	}
	if result.Explanation.MaxFinal != 28 {
		t.Errorf("MaxFinal = %v, want %v", result.Explanation.MaxFinal, 28)
	}

	if result.Explanation.MinSource != "wu" {
		t.Errorf("MinSource = %q, want %q", result.Explanation.MinSource, "wu")
	}
	if result.Explanation.MinRaw != 12 {
		t.Errorf("MinRaw = %v, want %v", result.Explanation.MinRaw, 12)
	}
	if result.Explanation.MinBiasApplied != -1.0 {
		t.Errorf("MinBiasApplied = %v, want %v", result.Explanation.MinBiasApplied, -1.0)
	}
	if result.Explanation.MinFinal != 13 {
		t.Errorf("MinFinal = %v, want %v", result.Explanation.MinFinal, 13)
	}
}

func TestLookupBiasWithFallback(t *testing.T) {
	tests := []struct {
		name          string
		stats         map[string]map[string]map[int]*store.CorrectionStats
		source        string
		target        string
		dayOfForecast int
		wantBias      float64
		wantDayUsed   int
		wantFallback  bool
	}{
		{
			name:          "nil stats returns no bias",
			stats:         nil,
			source:        "bom",
			target:        "tmax",
			dayOfForecast: 0,
			wantDayUsed:   -1,
		},
		{
			name: "exact day match",
			stats: map[string]map[string]map[int]*store.CorrectionStats{
				"bom": {"tmax": {0: {MeanBias: 2.0, SampleSize: 10}}},
			},
			source:        "bom",
			target:        "tmax",
			dayOfForecast: 0,
			wantBias:      2.0,
			wantDayUsed:   0,
			wantFallback:  false,
		},
		{
			name: "falls back to lower day",
			stats: map[string]map[string]map[int]*store.CorrectionStats{
				"bom": {"tmax": {1: {MeanBias: 1.5, SampleSize: 10}}},
			},
			source:        "bom",
			target:        "tmax",
			dayOfForecast: 2,
			wantBias:      1.5,
			wantDayUsed:   1,
			wantFallback:  true,
		},
		{
			name: "falls back to higher day when lower unavailable",
			stats: map[string]map[string]map[int]*store.CorrectionStats{
				"bom": {"tmax": {3: {MeanBias: 1.0, SampleSize: 10}}},
			},
			source:        "bom",
			target:        "tmax",
			dayOfForecast: 0,
			wantBias:      1.0,
			wantDayUsed:   3,
			wantFallback:  true,
		},
		{
			name: "skips days with insufficient samples",
			stats: map[string]map[string]map[int]*store.CorrectionStats{
				"bom": {"tmax": {
					0: {MeanBias: 5.0, SampleSize: 3}, // Too few
					1: {MeanBias: 2.0, SampleSize: 10},
				}},
			},
			source:        "bom",
			target:        "tmax",
			dayOfForecast: 0,
			wantBias:      2.0,
			wantDayUsed:   1,
			wantFallback:  true,
		},
		{
			name: "caps positive bias at max",
			stats: map[string]map[string]map[int]*store.CorrectionStats{
				"wu": {"tmin": {0: {MeanBias: 10.0, SampleSize: 10}}},
			},
			source:        "wu",
			target:        "tmin",
			dayOfForecast: 0,
			wantBias:      MaxBiasCorrection,
			wantDayUsed:   0,
			wantFallback:  false,
		},
		{
			name: "caps negative bias at max",
			stats: map[string]map[string]map[int]*store.CorrectionStats{
				"wu": {"tmax": {0: {MeanBias: -10.0, SampleSize: 10}}},
			},
			source:        "wu",
			target:        "tmax",
			dayOfForecast: 0,
			wantBias:      -MaxBiasCorrection,
			wantDayUsed:   0,
			wantFallback:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := LookupBiasWithFallback(tt.stats, tt.source, tt.target, tt.dayOfForecast)

			if result.DayUsed != tt.wantDayUsed {
				t.Errorf("DayUsed = %v, want %v", result.DayUsed, tt.wantDayUsed)
			}
			if result.Bias != tt.wantBias {
				t.Errorf("Bias = %v, want %v", result.Bias, tt.wantBias)
			}
			if result.IsFallback != tt.wantFallback {
				t.Errorf("IsFallback = %v, want %v", result.IsFallback, tt.wantFallback)
			}
		})
	}
}
