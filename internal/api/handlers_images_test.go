package api

import (
	"database/sql"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lox/wandiweather/internal/ecowitt"
	"github.com/lox/wandiweather/internal/forecast"
	"github.com/lox/wandiweather/internal/store"

	_ "modernc.org/sqlite"
)

type fakeImageAirQualityProvider struct {
	reading *ecowitt.AirQualityReading
	err     error
}

func (f fakeImageAirQualityProvider) CurrentAirQuality() (*ecowitt.AirQualityReading, error) {
	return f.reading, f.err
}

func setupImageTestServer(t *testing.T, airQuality airQualityProvider) *Server {
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

	return NewServer(st, "8080", time.UTC, airQuality)
}

func TestGetCurrentSmokeLevel_UsesFreshStoredAirQuality(t *testing.T) {
	t.Parallel()

	srv := setupImageTestServer(t, nil)
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

	srv := setupImageTestServer(t, nil)
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

func TestGetCurrentSmokeLevel_UsesSharedAirQualityResolution(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	tests := []struct {
		name   string
		stored ecowitt.AirQualityReading
		live   *ecowitt.AirQualityReading
		want   forecast.SmokeLevel
	}{
		{
			name: "fresh live reading wins",
			stored: ecowitt.AirQualityReading{
				ObservedAt:     now.Add(-10 * time.Minute),
				PM25:           18,
				RealTimeAQI:    72,
				HasRealTimeAQI: true,
				SourceFieldKey: "pm25_ch1",
			},
			live: &ecowitt.AirQualityReading{
				ObservedAt:     now.Add(-5 * time.Minute),
				PM25:           51,
				RealTimeAQI:    139,
				HasRealTimeAQI: true,
				SourceFieldKey: "pm25_ch1",
			},
			want: forecast.SmokeVisible,
		},
		{
			name: "stale live reading is ignored",
			stored: ecowitt.AirQualityReading{
				ObservedAt:     now.Add(-10 * time.Minute),
				PM25:           18,
				RealTimeAQI:    72,
				HasRealTimeAQI: true,
				SourceFieldKey: "pm25_ch1",
			},
			live: &ecowitt.AirQualityReading{
				ObservedAt:     now.Add(-2 * time.Hour),
				PM25:           80,
				RealTimeAQI:    188,
				HasRealTimeAQI: true,
				SourceFieldKey: "pm25_ch1",
			},
			want: forecast.SmokeHaze,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := setupImageTestServer(t, fakeImageAirQualityProvider{reading: tt.live})
			if _, err := srv.store.UpsertAirQualityReadings([]ecowitt.AirQualityReading{tt.stored}); err != nil {
				t.Fatalf("UpsertAirQualityReadings: %v", err)
			}

			if got := srv.getCurrentSmokeLevel(now); got != tt.want {
				t.Fatalf("getCurrentSmokeLevel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestServeBannerImage_DisablesBrowserCaching(t *testing.T) {
	t.Parallel()

	srv := setupImageTestServer(t, nil)
	w := httptest.NewRecorder()

	srv.serveBannerImage(w, []byte("png"))

	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if got := w.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", got)
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
