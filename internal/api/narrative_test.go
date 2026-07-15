package api

import (
	"database/sql"
	"testing"
	"time"

	"github.com/lox/wandiweather/internal/models"
)

func TestExtractCondition(t *testing.T) {
	tests := []struct {
		name      string
		narrative string
		want      string
	}{
		{
			name:      "WU with temps",
			narrative: "Partly cloudy. Highs 28 to 30°C and lows 12 to 14°C.",
			want:      "Partly cloudy",
		},
		{
			name:      "WU thunderstorms",
			narrative: "Thunderstorms developing in the afternoon. Highs 27 to 29°C and lows 14 to 16°C.",
			want:      "Thunderstorms developing in the afternoon",
		},
		{
			name:      "WU generally clear",
			narrative: "Generally clear. Highs 32 to 34°C and lows 17 to 19°C.",
			want:      "Generally clear",
		},
		{
			name:      "BOM simple",
			narrative: "Mostly sunny.",
			want:      "Mostly sunny",
		},
		{
			name:      "empty",
			narrative: "",
			want:      "",
		},
		{
			name:      "only temps",
			narrative: "Highs 28 to 30°C and lows 12 to 14°C.",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractCondition(tt.narrative)
			if got != tt.want {
				t.Errorf("extractCondition(%q) = %q, want %q", tt.narrative, got, tt.want)
			}
		})
	}
}

func TestChooseCondition(t *testing.T) {
	tests := []struct {
		name string
		day  *ForecastDay
		want string
	}{
		{
			name: "prefer BOM when no storms",
			day: &ForecastDay{
				WU: &models.Forecast{
					Narrative: sql.NullString{String: "Partly cloudy. Highs 28 to 30°C.", Valid: true},
				},
				BOM: &models.Forecast{
					Narrative: sql.NullString{String: "Mostly sunny.", Valid: true},
				},
			},
			want: "Mostly sunny",
		},
		{
			name: "prefer WU when storms",
			day: &ForecastDay{
				WU: &models.Forecast{
					Narrative: sql.NullString{String: "Thunderstorms in the afternoon. Highs 28 to 30°C.", Valid: true},
				},
				BOM: &models.Forecast{
					Narrative: sql.NullString{String: "Possible storm.", Valid: true},
				},
			},
			want: "Thunderstorms in the afternoon",
		},
		{
			name: "WU only",
			day: &ForecastDay{
				WU: &models.Forecast{
					Narrative: sql.NullString{String: "Partly cloudy. Highs 28 to 30°C.", Valid: true},
				},
			},
			want: "Partly cloudy",
		},
		{
			name: "BOM only",
			day: &ForecastDay{
				BOM: &models.Forecast{
					Narrative: sql.NullString{String: "Sunny.", Valid: true},
				},
			},
			want: "Sunny",
		},
		{
			name: "neither",
			day:  &ForecastDay{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chooseCondition(tt.day)
			if got != tt.want {
				t.Errorf("chooseCondition() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChooseTemps(t *testing.T) {
	ptr := func(f float64) *float64 { return &f }

	tests := []struct {
		name     string
		day      *ForecastDay
		wantHi   float64
		wantLo   float64
		wantHave bool
	}{
		{
			name: "corrected WU temps",
			day: &ForecastDay{
				WUCorrectedMax: ptr(35),
				WUCorrectedMin: ptr(11),
				WU: &models.Forecast{
					TempMax: sql.NullFloat64{Float64: 29, Valid: true},
					TempMin: sql.NullFloat64{Float64: 10, Valid: true},
				},
			},
			wantHi:   35,
			wantLo:   11,
			wantHave: true,
		},
		{
			name: "raw WU temps when no correction",
			day: &ForecastDay{
				WU: &models.Forecast{
					TempMax: sql.NullFloat64{Float64: 29, Valid: true},
					TempMin: sql.NullFloat64{Float64: 10, Valid: true},
				},
			},
			wantHi:   29,
			wantLo:   10,
			wantHave: true,
		},
		{
			name: "fallback to BOM",
			day: &ForecastDay{
				BOM: &models.Forecast{
					TempMax: sql.NullFloat64{Float64: 34, Valid: true},
					TempMin: sql.NullFloat64{Float64: 13, Valid: true},
				},
			},
			wantHi:   34,
			wantLo:   13,
			wantHave: true,
		},
		{
			name:     "no temps",
			day:      &ForecastDay{},
			wantHave: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hi, lo, haveHi, haveLo := chooseTemps(tt.day)
			if tt.wantHave {
				if !haveHi || !haveLo {
					t.Errorf("chooseTemps() missing temps, haveHi=%v haveLo=%v", haveHi, haveLo)
				}
				if hi != tt.wantHi {
					t.Errorf("chooseTemps() hi = %v, want %v", hi, tt.wantHi)
				}
				if lo != tt.wantLo {
					t.Errorf("chooseTemps() lo = %v, want %v", lo, tt.wantLo)
				}
			} else {
				if haveHi || haveLo {
					t.Errorf("chooseTemps() should not have temps")
				}
			}
		})
	}
}

func TestBuildGeneratedNarrative(t *testing.T) {
	ptr := func(f float64) *float64 { return &f }

	tests := []struct {
		name string
		day  *ForecastDay
		want string
	}{
		{
			name: "full narrative with corrected temps",
			day: &ForecastDay{
				WUCorrectedMax: ptr(35),
				WUCorrectedMin: ptr(10.5),
				WU: &models.Forecast{
					Narrative: sql.NullString{String: "Partly cloudy. Highs 28 to 30°C.", Valid: true},
				},
				BOM: &models.Forecast{
					Narrative: sql.NullString{String: "Mostly sunny.", Valid: true},
				},
			},
			want: "Mostly sunny. High 35°C, low 11°C.",
		},
		{
			name: "storms from WU",
			day: &ForecastDay{
				WU: &models.Forecast{
					Narrative: sql.NullString{String: "Thunderstorms developing. Highs 28 to 30°C.", Valid: true},
					TempMax:   sql.NullFloat64{Float64: 28, Valid: true},
					TempMin:   sql.NullFloat64{Float64: 16, Valid: true},
				},
				BOM: &models.Forecast{
					Narrative: sql.NullString{String: "Possible storm.", Valid: true},
				},
			},
			want: "Thunderstorms developing. High 28°C, low 16°C.",
		},
		{
			name: "only max temp",
			day: &ForecastDay{
				BOM: &models.Forecast{
					Narrative: sql.NullString{String: "Sunny.", Valid: true},
					TempMax:   sql.NullFloat64{Float64: 35, Valid: true},
				},
			},
			want: "Sunny. High 35°C.",
		},
		{
			name: "no data",
			day:  &ForecastDay{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildGeneratedNarrative(tt.day)
			if got != tt.want {
				t.Errorf("buildGeneratedNarrative() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLiveBOMForecastsPrefersDailyAPIWithLegacyFallback(t *testing.T) {
	today := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	tomorrow := today.AddDate(0, 0, 1)
	legacy := models.Forecast{Source: "bom", ValidDate: today}
	dailyAPI := models.Forecast{Source: "bom_daily_api", ValidDate: today}

	got := liveBOMForecasts(map[string][]models.Forecast{
		"bom":           {legacy},
		"bom_daily_api": {dailyAPI},
	})
	if len(got) != 1 || got[0].Source != "bom_daily_api" {
		t.Fatalf("liveBOMForecasts() = %+v, want daily API", got)
	}

	got = liveBOMForecasts(map[string][]models.Forecast{
		"bom": {legacy},
	})
	if len(got) != 1 || got[0].Source != "bom" {
		t.Fatalf("liveBOMForecasts() = %+v, want legacy fallback", got)
	}

	got = liveBOMForecasts(map[string][]models.Forecast{
		"bom":           {legacy},
		"bom_daily_api": {{Source: "bom_daily_api", ValidDate: tomorrow}},
	})
	if len(got) != 2 {
		t.Fatalf("liveBOMForecasts() returned %d forecasts, want daily API plus legacy date fallback", len(got))
	}
	if got[0].Source != "bom" || !got[0].ValidDate.Equal(today) {
		t.Fatalf("liveBOMForecasts()[0] = %+v, want legacy fallback for missing today", got[0])
	}
	if got[1].Source != "bom_daily_api" || !got[1].ValidDate.Equal(tomorrow) {
		t.Fatalf("liveBOMForecasts()[1] = %+v, want daily API tomorrow", got[1])
	}
}

func TestBuildPrecipDisplayFallsBackToLiveBOMSourceBeforeLegacy(t *testing.T) {
	store := fakePrecipRangeLookup{
		ranges: map[string]string{
			"bom_daily_api": "3 to 8 mm",
			"bom":           "0 to 1 mm",
		},
	}

	got := buildPrecipDisplay(&ForecastDay{
		Date: time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC),
		BOM:  &models.Forecast{Source: "bom_daily_api"},
	}, store)
	if got != "3–8mm" {
		t.Fatalf("buildPrecipDisplay() = %q, want daily API range", got)
	}

	store.ranges["bom_daily_api"] = ""
	got = buildPrecipDisplay(&ForecastDay{
		Date: time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC),
		BOM:  &models.Forecast{Source: "bom_daily_api"},
	}, store)
	if got != "0–1mm" {
		t.Fatalf("buildPrecipDisplay() = %q, want legacy fallback range", got)
	}
}

type fakePrecipRangeLookup struct {
	ranges map[string]string
}

func (f fakePrecipRangeLookup) GetLastForecastPrecipRange(source string, _ time.Time) (string, error) {
	return f.ranges[source], nil
}
