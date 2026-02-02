package forecast

import (
	"database/sql"
	"testing"

	"github.com/lox/wandiweather/internal/models"
)

func TestClassifyRegime_Heatwave(t *testing.T) {
	tests := []struct {
		name     string
		forecast *models.Forecast
		prevDays []models.DailySummary
		want     bool
	}{
		{
			name:     "forecast >= 35C triggers heatwave",
			forecast: &models.Forecast{TempMax: sql.NullFloat64{Float64: 35, Valid: true}},
			prevDays: nil,
			want:     true,
		},
		{
			name:     "forecast 34C does not trigger heatwave alone",
			forecast: &models.Forecast{TempMax: sql.NullFloat64{Float64: 34, Valid: true}},
			prevDays: nil,
			want:     false,
		},
		{
			name:     "two consecutive days >= 32C triggers heatwave",
			forecast: &models.Forecast{TempMax: sql.NullFloat64{Float64: 28, Valid: true}},
			prevDays: []models.DailySummary{
				{TempMax: sql.NullFloat64{Float64: 33, Valid: true}},
				{TempMax: sql.NullFloat64{Float64: 32, Valid: true}},
			},
			want: true,
		},
		{
			name:     "only one day >= 32C does not trigger heatwave",
			forecast: &models.Forecast{TempMax: sql.NullFloat64{Float64: 25, Valid: true}},
			prevDays: []models.DailySummary{
				{TempMax: sql.NullFloat64{Float64: 33, Valid: true}},
				{TempMax: sql.NullFloat64{Float64: 30, Valid: true}},
			},
			want: false,
		},
		{
			name:     "nil forecast with two hot days",
			forecast: nil,
			prevDays: []models.DailySummary{
				{TempMax: sql.NullFloat64{Float64: 32, Valid: true}},
				{TempMax: sql.NullFloat64{Float64: 34, Valid: true}},
			},
			want: true,
		},
		{
			name:     "only one previous day does not trigger",
			forecast: nil,
			prevDays: []models.DailySummary{
				{TempMax: sql.NullFloat64{Float64: 35, Valid: true}},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyRegime(tt.forecast, nil, tt.prevDays)
			if result.Heatwave != tt.want {
				t.Errorf("Heatwave = %v, want %v", result.Heatwave, tt.want)
			}
		})
	}
}

func TestClassifyRegime_Inversion(t *testing.T) {
	tests := []struct {
		name    string
		summary *models.DailySummary
		want    bool
	}{
		{
			name:    "nil summary",
			summary: nil,
			want:    false,
		},
		{
			name: "inversion detected",
			summary: &models.DailySummary{
				InversionDetected: sql.NullBool{Bool: true, Valid: true},
			},
			want: true,
		},
		{
			name: "no inversion",
			summary: &models.DailySummary{
				InversionDetected: sql.NullBool{Bool: false, Valid: true},
			},
			want: false,
		},
		{
			name: "inversion field not valid",
			summary: &models.DailySummary{
				InversionDetected: sql.NullBool{Bool: true, Valid: false},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyRegime(nil, tt.summary, nil)
			if result.InversionNight != tt.want {
				t.Errorf("InversionNight = %v, want %v", result.InversionNight, tt.want)
			}
		})
	}
}

func TestRegimeToString_Priority(t *testing.T) {
	tests := []struct {
		name  string
		flags RegimeFlags
		want  string
	}{
		{
			name:  "heatwave takes priority",
			flags: RegimeFlags{Heatwave: true, InversionNight: true, ClearCalm: true},
			want:  "heatwave",
		},
		{
			name:  "inversion when no heatwave",
			flags: RegimeFlags{Heatwave: false, InversionNight: true, ClearCalm: true},
			want:  "inversion",
		},
		{
			name:  "clear_calm when no heatwave or inversion",
			flags: RegimeFlags{Heatwave: false, InversionNight: false, ClearCalm: true},
			want:  "clear_calm",
		},
		{
			name:  "all when no flags set",
			flags: RegimeFlags{},
			want:  "all",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RegimeToString(tt.flags)
			if got != tt.want {
				t.Errorf("RegimeToString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClassifyRegime_ClearCalm(t *testing.T) {
	tests := []struct {
		name    string
		summary *models.DailySummary
		want    bool
	}{
		{
			name:    "nil summary",
			summary: nil,
			want:    false,
		},
		{
			name: "all conditions met - clear calm",
			summary: &models.DailySummary{
				PrecipTotal:       sql.NullFloat64{Float64: 0.0, Valid: true},
				SolarIntegral:     sql.NullFloat64{Float64: 15.0, Valid: true},
				CalmFractionNight: sql.NullFloat64{Float64: 0.5, Valid: true},
			},
			want: true,
		},
		{
			name: "too much precip",
			summary: &models.DailySummary{
				PrecipTotal:       sql.NullFloat64{Float64: 2.0, Valid: true},
				SolarIntegral:     sql.NullFloat64{Float64: 15.0, Valid: true},
				CalmFractionNight: sql.NullFloat64{Float64: 0.5, Valid: true},
			},
			want: false,
		},
		{
			name: "low solar",
			summary: &models.DailySummary{
				PrecipTotal:       sql.NullFloat64{Float64: 0.0, Valid: true},
				SolarIntegral:     sql.NullFloat64{Float64: 5.0, Valid: true},
				CalmFractionNight: sql.NullFloat64{Float64: 0.5, Valid: true},
			},
			want: false,
		},
		{
			name: "not calm enough",
			summary: &models.DailySummary{
				PrecipTotal:       sql.NullFloat64{Float64: 0.0, Valid: true},
				SolarIntegral:     sql.NullFloat64{Float64: 15.0, Valid: true},
				CalmFractionNight: sql.NullFloat64{Float64: 0.2, Valid: true},
			},
			want: false,
		},
		{
			name: "boundary values - just dry enough",
			summary: &models.DailySummary{
				PrecipTotal:       sql.NullFloat64{Float64: 0.49, Valid: true},
				SolarIntegral:     sql.NullFloat64{Float64: 10.1, Valid: true},
				CalmFractionNight: sql.NullFloat64{Float64: 0.41, Valid: true},
			},
			want: true,
		},
		{
			name: "missing precip field",
			summary: &models.DailySummary{
				PrecipTotal:       sql.NullFloat64{Valid: false},
				SolarIntegral:     sql.NullFloat64{Float64: 15.0, Valid: true},
				CalmFractionNight: sql.NullFloat64{Float64: 0.5, Valid: true},
			},
			want: false,
		},
		{
			name: "missing solar field",
			summary: &models.DailySummary{
				PrecipTotal:       sql.NullFloat64{Float64: 0.0, Valid: true},
				SolarIntegral:     sql.NullFloat64{Valid: false},
				CalmFractionNight: sql.NullFloat64{Float64: 0.5, Valid: true},
			},
			want: false,
		},
		{
			name: "missing calm fraction field",
			summary: &models.DailySummary{
				PrecipTotal:       sql.NullFloat64{Float64: 0.0, Valid: true},
				SolarIntegral:     sql.NullFloat64{Float64: 15.0, Valid: true},
				CalmFractionNight: sql.NullFloat64{Valid: false},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyRegime(nil, tt.summary, nil)
			if result.ClearCalm != tt.want {
				t.Errorf("ClearCalm = %v, want %v", result.ClearCalm, tt.want)
			}
		})
	}
}

func TestClassifyRegime_Combined(t *testing.T) {
	forecast := &models.Forecast{TempMax: sql.NullFloat64{Float64: 36, Valid: true}}
	summary := &models.DailySummary{
		InversionDetected: sql.NullBool{Bool: true, Valid: true},
		PrecipTotal:       sql.NullFloat64{Float64: 0.0, Valid: true},
		SolarIntegral:     sql.NullFloat64{Float64: 15.0, Valid: true},
		CalmFractionNight: sql.NullFloat64{Float64: 0.5, Valid: true},
	}

	result := ClassifyRegime(forecast, summary, nil)

	if !result.Heatwave {
		t.Error("Expected Heatwave to be true (forecast >= 35)")
	}
	if !result.InversionNight {
		t.Error("Expected InversionNight to be true")
	}
	if !result.ClearCalm {
		t.Error("Expected ClearCalm to be true (all conditions met)")
	}

	regime := RegimeToString(result)
	if regime != "heatwave" {
		t.Errorf("RegimeToString() = %q, want 'heatwave' (highest priority)", regime)
	}
}
