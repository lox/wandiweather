package api

import (
	"database/sql"
	"testing"
	"time"

	"github.com/lox/wandiweather/internal/ecowitt"
	"github.com/lox/wandiweather/internal/forecast"
	"github.com/lox/wandiweather/internal/store"

	_ "modernc.org/sqlite"
)

func setupImageTestServer(t *testing.T) *Server {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	st := store.New(db, time.UTC)
	if err := st.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return NewServer(st, "8080", time.UTC, nil)
}

func TestGetCurrentSmokeLevel_UsesFreshStoredAirQuality(t *testing.T) {
	t.Parallel()

	srv := setupImageTestServer(t)
	now := time.Now().UTC()

	if _, err := srv.store.UpsertAirQualityReadings([]ecowitt.AirQualityReading{{
		ObservedAt:     now.Add(-10 * time.Minute),
		PM25:           51,
		RealTimeAQI:    139,
		HasRealTimeAQI: true,
		Category:       "Poor",
		CategoryClass:  "poor",
		SourceFieldKey: "pm25_ch1",
	}}); err != nil {
		t.Fatalf("UpsertAirQualityReadings: %v", err)
	}

	if got := srv.getCurrentSmokeLevel(now); got != forecast.SmokeVisible {
		t.Fatalf("getCurrentSmokeLevel() = %v, want %v", got, forecast.SmokeVisible)
	}
}

func TestGetCurrentSmokeLevel_IgnoresStaleStoredAirQuality(t *testing.T) {
	t.Parallel()

	srv := setupImageTestServer(t)
	now := time.Now().UTC()

	if _, err := srv.store.UpsertAirQualityReadings([]ecowitt.AirQualityReading{{
		ObservedAt:     now.Add(-2 * time.Hour),
		PM25:           80,
		RealTimeAQI:    188,
		HasRealTimeAQI: true,
		Category:       "Unhealthy",
		CategoryClass:  "unhealthy",
		SourceFieldKey: "pm25_ch1",
	}}); err != nil {
		t.Fatalf("UpsertAirQualityReadings: %v", err)
	}

	if got := srv.getCurrentSmokeLevel(now); got != forecast.SmokeClear {
		t.Fatalf("getCurrentSmokeLevel() = %v, want %v", got, forecast.SmokeClear)
	}
}

func TestParseSmokeOverride(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		want   forecast.SmokeLevel
		wantOK bool
	}{
		{name: "clear", input: "clear", want: forecast.SmokeClear, wantOK: true},
		{name: "haze", input: "haze", want: forecast.SmokeHaze, wantOK: true},
		{name: "smoke", input: "smoke", want: forecast.SmokeVisible, wantOK: true},
		{name: "dense smoke", input: "dense_smoke", want: forecast.SmokeDense, wantOK: true},
		{name: "invalid", input: "wildfire", want: "", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseSmokeOverride(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("parseSmokeOverride() ok = %t, want %t", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Fatalf("parseSmokeOverride() = %v, want %v", got, tt.want)
			}
		})
	}
}
