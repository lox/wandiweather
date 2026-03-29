package api_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
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

func insertObservation(t *testing.T, s *store.Store, stationID string, observedAt time.Time, temp float64) {
	t.Helper()
	if err := s.InsertObservation(models.Observation{
		StationID:  stationID,
		ObservedAt: observedAt,
		Temp:       sql.NullFloat64{Float64: temp, Valid: true},
	}); err != nil {
		t.Fatalf("insert observation %s: %v", stationID, err)
	}
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

func TestHealthEndpoint_IgnoresStaleAuxiliaryStation(t *testing.T) {
	t.Parallel()

	s, loc := setupTestStore(t)
	if err := s.UpsertStation(models.Station{StationID: "PRIMARY", Name: "Primary", ElevationTier: "valley_floor", IsPrimary: true, Active: true}); err != nil {
		t.Fatalf("upsert primary station: %v", err)
	}
	if err := s.UpsertStation(models.Station{StationID: "AUX", Name: "Aux", ElevationTier: "upper", Active: true}); err != nil {
		t.Fatalf("upsert aux station: %v", err)
	}

	now := time.Now().UTC()
	insertObservation(t, s, "PRIMARY", now.Add(-5*time.Minute), 12)
	insertObservation(t, s, "AUX", now.Add(-2*time.Hour), 22)

	srv := api.NewServer(s, "8080", loc)
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var health api.HealthStatus
	if err := json.NewDecoder(w.Body).Decode(&health); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if health.Status != "ok" {
		t.Fatalf("expected ok health status, got %q", health.Status)
	}

	var aux *api.StationHealth
	for i := range health.Stations {
		if health.Stations[i].StationID == "AUX" {
			aux = &health.Stations[i]
			break
		}
	}
	if aux == nil {
		t.Fatal("expected AUX station in health response")
	}
	if !aux.Stale {
		t.Fatal("expected AUX station to be marked stale")
	}
}

func TestHealthEndpoint_DegradesWhenPrimaryStationIsStale(t *testing.T) {
	t.Parallel()

	s, loc := setupTestStore(t)
	if err := s.UpsertStation(models.Station{StationID: "PRIMARY", Name: "Primary", ElevationTier: "valley_floor", IsPrimary: true, Active: true}); err != nil {
		t.Fatalf("upsert primary station: %v", err)
	}
	if err := s.UpsertStation(models.Station{StationID: "AUX", Name: "Aux", ElevationTier: "upper", Active: true}); err != nil {
		t.Fatalf("upsert aux station: %v", err)
	}

	now := time.Now().UTC()
	insertObservation(t, s, "PRIMARY", now.Add(-2*time.Hour), 12)
	insertObservation(t, s, "AUX", now.Add(-5*time.Minute), 18)

	srv := api.NewServer(s, "8080", loc)
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}

	var health api.HealthStatus
	if err := json.NewDecoder(w.Body).Decode(&health); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if health.Status != "degraded" {
		t.Fatalf("expected degraded health status, got %q", health.Status)
	}
}

func TestAPICurrent_IgnoresStaleAuxiliaryObservations(t *testing.T) {
	t.Parallel()

	s, loc := setupTestStore(t)
	if err := s.UpsertStation(models.Station{StationID: "PRIMARY", Name: "Primary", ElevationTier: "valley_floor", IsPrimary: true, Active: true}); err != nil {
		t.Fatalf("upsert primary station: %v", err)
	}
	if err := s.UpsertStation(models.Station{StationID: "VALLEY2", Name: "Valley 2", ElevationTier: "valley_floor", Active: true}); err != nil {
		t.Fatalf("upsert valley station: %v", err)
	}
	if err := s.UpsertStation(models.Station{StationID: "STALEUP", Name: "Stale Upper", ElevationTier: "upper", Active: true}); err != nil {
		t.Fatalf("upsert upper station: %v", err)
	}

	now := time.Now().UTC()
	insertObservation(t, s, "PRIMARY", now.Add(-5*time.Minute), 10)
	insertObservation(t, s, "VALLEY2", now.Add(-4*time.Minute), 12)
	insertObservation(t, s, "STALEUP", now.Add(-2*time.Hour), 30)

	srv := api.NewServer(s, "8080", loc)
	req := httptest.NewRequest("GET", "/api/current", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var response struct {
		Stations    map[string]json.RawMessage `json:"Stations"`
		AllStations []struct {
			Station models.Station `json:"Station"`
		} `json:"AllStations"`
		Inversion *api.InversionStatus `json:"Inversion"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode current response: %v", err)
	}

	if _, ok := response.Stations["STALEUP"]; ok {
		t.Fatal("expected stale upper station to be excluded from current stations")
	}
	for _, station := range response.AllStations {
		if station.Station.StationID == "STALEUP" {
			t.Fatal("expected stale upper station to be excluded from all stations")
		}
	}
	if response.Inversion != nil {
		t.Fatalf("expected no inversion from stale upper observation, got %+v", *response.Inversion)
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
