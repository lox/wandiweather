package api_test

import (
	"database/sql"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lox/wandiweather/internal/api"
	"github.com/lox/wandiweather/internal/models"
	"github.com/lox/wandiweather/internal/store"

	_ "modernc.org/sqlite"
)

func setupTestStore(t *testing.T) (*store.Store, *time.Location) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	loc := time.UTC
	s := store.New(db, loc)
	if err := s.Migrate(); err != nil {
		t.Fatal(err)
	}
	return s, loc
}

func TestHealthEndpoint(t *testing.T) {
	t.Parallel()
	s, loc := setupTestStore(t)
	srv := api.NewServer(s, "8080", loc)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, `"status"`) {
		t.Error("expected status field in JSON response")
	}
}

func TestAccuracyPage_NoData(t *testing.T) {
	t.Parallel()
	s, loc := setupTestStore(t)
	srv := api.NewServer(s, "8080", loc)

	req := httptest.NewRequest("GET", "/accuracy", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "<h1>Forecast Accuracy</h1>") {
		t.Error("expected h1 'Forecast Accuracy'")
	}
	if !strings.Contains(body, "class=\"intro\"") {
		t.Error("expected intro section")
	}
	if strings.Contains(body, "id=\"biasChart\"") {
		t.Error("expected no chart when no data")
	}
}

func TestAccuracyPage_WithData(t *testing.T) {
	t.Parallel()
	s, loc := setupTestStore(t)

	s.UpsertStation(models.Station{
		StationID:     "TEST1",
		Name:          "Test Station",
		ElevationTier: "valley_floor",
		IsPrimary:     true,
		Active:        true,
	})

	now := time.Now().UTC()
	validDate := time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, time.UTC)
	s.InsertForecast(models.Forecast{
		Source:        "wu",
		FetchedAt:     validDate.Add(-24 * time.Hour),
		ValidDate:     validDate,
		DayOfForecast: 1,
		TempMax:       sql.NullFloat64{Float64: 30, Valid: true},
		TempMin:       sql.NullFloat64{Float64: 15, Valid: true},
	})

	srv := api.NewServer(s, "8080", loc)
	req := httptest.NewRequest("GET", "/accuracy", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "<title>Forecast Accuracy - WandiWeather</title>") {
		t.Error("expected correct title")
	}
	if !strings.Contains(body, "class=\"stats-card\"") {
		t.Error("expected stats card")
	}
}

func TestAccuracyPage_ChartPresent(t *testing.T) {
	t.Parallel()
	s, loc := setupTestStore(t)

	s.UpsertStation(models.Station{
		StationID:     "TEST1",
		Name:          "Test Station",
		ElevationTier: "valley_floor",
		IsPrimary:     true,
		Active:        true,
	})

	now := time.Now().UTC()
	for i := 1; i <= 3; i++ {
		validDate := time.Date(now.Year(), now.Month(), now.Day()-i, 0, 0, 0, 0, time.UTC)
		s.InsertForecast(models.Forecast{
			Source:        "wu",
			FetchedAt:     validDate.Add(-24 * time.Hour),
			ValidDate:     validDate,
			DayOfForecast: 1,
			TempMax:       sql.NullFloat64{Float64: float64(25 + i), Valid: true},
			TempMin:       sql.NullFloat64{Float64: float64(10 + i), Valid: true},
		})
		s.InsertForecastVerification(models.ForecastVerification{
			ForecastID:  int64(i),
			ValidDate:   validDate,
			BiasTempMax: sql.NullFloat64{Float64: float64(i), Valid: true},
			BiasTempMin: sql.NullFloat64{Float64: float64(-i), Valid: true},
		})
	}

	srv := api.NewServer(s, "8080", loc)
	req := httptest.NewRequest("GET", "/accuracy", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "id=\"biasChart\"") {
		t.Error("expected bias chart canvas")
	}
	if !strings.Contains(body, "<canvas") {
		t.Error("expected canvas element")
	}
	if !strings.Contains(body, "Bias Over Time") {
		t.Error("expected Bias Over Time section header")
	}
}
