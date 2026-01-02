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
			name:     "forecast >= 32C triggers heatwave",
			forecast: &models.Forecast{TempMax: sql.NullFloat64{Float64: 35, Valid: true}},
			prevDays: nil,
			want:     true,
		},
		{
			name:     "forecast exactly 32C triggers heatwave",
			forecast: &models.Forecast{TempMax: sql.NullFloat64{Float64: 32, Valid: true}},
			prevDays: nil,
			want:     true,
		},
		{
			name:     "forecast 31C does not trigger heatwave alone",
			forecast: &models.Forecast{TempMax: sql.NullFloat64{Float64: 31, Valid: true}},
			prevDays: nil,
			want:     false,
		},
		{
			name:     "previous day >= 30C triggers heatwave",
			forecast: &models.Forecast{TempMax: sql.NullFloat64{Float64: 28, Valid: true}},
			prevDays: []models.DailySummary{
				{TempMax: sql.NullFloat64{Float64: 30, Valid: true}},
			},
			want: true,
		},
		{
			name:     "2-day avg >= 28C triggers heatwave",
			forecast: &models.Forecast{TempMax: sql.NullFloat64{Float64: 25, Valid: true}},
			prevDays: []models.DailySummary{
				{TempMax: sql.NullFloat64{Float64: 29, Valid: true}},
				{TempMax: sql.NullFloat64{Float64: 27, Valid: true}},
			},
			want: true,
		},
		{
			name:     "2-day avg < 28C does not trigger heatwave",
			forecast: &models.Forecast{TempMax: sql.NullFloat64{Float64: 25, Valid: true}},
			prevDays: []models.DailySummary{
				{TempMax: sql.NullFloat64{Float64: 27, Valid: true}},
				{TempMax: sql.NullFloat64{Float64: 27, Valid: true}},
			},
			want: false,
		},
		{
			name:     "nil forecast with hot previous day",
			forecast: nil,
			prevDays: []models.DailySummary{
				{TempMax: sql.NullFloat64{Float64: 32, Valid: true}},
			},
			want: true,
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
