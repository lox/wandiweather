package api_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lox/wandiweather/internal/api"
	"github.com/lox/wandiweather/internal/ecowitt"
	"github.com/lox/wandiweather/internal/models"
	"github.com/lox/wandiweather/internal/store"

	_ "modernc.org/sqlite"
)

type fakeAirQualityProvider struct {
	reading *ecowitt.AirQualityReading
	err     error
}

func (f fakeAirQualityProvider) CurrentAirQuality() (*ecowitt.AirQualityReading, error) {
	return f.reading, f.err
}

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
	srv := api.NewServer(s, "8080", loc, nil)

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

	srv := api.NewServer(s, "8080", loc, nil)
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

	srv := api.NewServer(s, "8080", loc, nil)
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

	srv := api.NewServer(s, "8080", loc, nil)
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

func TestAPICurrent_UsesDailyAPIBOMForecastForLiveRead(t *testing.T) {
	t.Parallel()

	s, loc := setupTestStore(t)
	if err := s.UpsertStation(models.Station{StationID: "PRIMARY", Name: "Primary", ElevationTier: "valley_floor", IsPrimary: true, Active: true}); err != nil {
		t.Fatalf("upsert primary station: %v", err)
	}
	insertObservation(t, s, "PRIMARY", time.Now().UTC().Add(-5*time.Minute), 15)

	now := time.Now().UTC()
	validDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	fetchedAt := now.Add(-10 * time.Minute).Truncate(time.Second)
	insertForecastForLiveBOMTest(t, s, models.Forecast{
		Source:        "wu",
		FetchedAt:     fetchedAt,
		ValidDate:     validDate,
		DayOfForecast: 0,
		TempMax:       sql.NullFloat64{Float64: 27, Valid: true},
		TempMin:       sql.NullFloat64{Float64: 11, Valid: true},
		PrecipChance:  sql.NullInt64{Int64: 70, Valid: true},
		PrecipAmount:  sql.NullFloat64{Float64: 2, Valid: true},
		Narrative:     sql.NullString{String: "Partly cloudy.", Valid: true},
	})
	insertForecastForLiveBOMTest(t, s, models.Forecast{
		Source:        "bom",
		FetchedAt:     fetchedAt,
		ValidDate:     validDate,
		DayOfForecast: 0,
		TempMax:       sql.NullFloat64{Float64: 22, Valid: true},
		TempMin:       sql.NullFloat64{Float64: 8, Valid: true},
		PrecipRange:   sql.NullString{String: "0 to 1 mm", Valid: true},
		Narrative:     sql.NullString{String: "Legacy BOM narrative.", Valid: true},
	})
	insertForecastForLiveBOMTest(t, s, models.Forecast{
		Source:        "bom_daily_api",
		FetchedAt:     fetchedAt,
		ValidDate:     validDate,
		DayOfForecast: 0,
		TempMax:       sql.NullFloat64{Float64: 31, Valid: true},
		TempMin:       sql.NullFloat64{Float64: 9, Valid: true},
		PrecipRange:   sql.NullString{String: "3 to 8 mm", Valid: true},
		Narrative:     sql.NullString{String: "Daily API narrative.", Valid: true},
	})

	srv := api.NewServer(s, "8080", loc, nil)
	req := httptest.NewRequest("GET", "/api/current", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var response struct {
		TodayForecast *struct {
			TempMax       float64 `json:"TempMax"`
			PrecipDisplay string  `json:"PrecipDisplay"`
			Narrative     string  `json:"Narrative"`
			Explanation   struct {
				MaxRaw float64 `json:"MaxRaw"`
			} `json:"Explanation"`
		} `json:"TodayForecast"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode current response: %v", err)
	}
	if response.TodayForecast == nil {
		t.Fatal("expected today forecast")
	}
	if response.TodayForecast.Explanation.MaxRaw != 31 {
		t.Fatalf("expected daily API max raw 31, got %.1f", response.TodayForecast.Explanation.MaxRaw)
	}
	if response.TodayForecast.TempMax != 31 {
		t.Fatalf("expected daily API max display 31, got %.1f", response.TodayForecast.TempMax)
	}
	if response.TodayForecast.PrecipDisplay != "3–8mm" {
		t.Fatalf("expected daily API precip range, got %q", response.TodayForecast.PrecipDisplay)
	}
	if !strings.Contains(response.TodayForecast.Narrative, "Daily API narrative") {
		t.Fatalf("expected daily API narrative, got %q", response.TodayForecast.Narrative)
	}
}

func TestAPIForecast_UsesDailyAPIBOMForecastForLiveRead(t *testing.T) {
	t.Parallel()

	s, loc := setupTestStore(t)

	now := time.Now().UTC()
	validDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	fetchedAt := now.Add(-10 * time.Minute).Truncate(time.Second)
	insertForecastForLiveBOMTest(t, s, models.Forecast{
		Source:        "wu",
		FetchedAt:     fetchedAt,
		ValidDate:     validDate,
		DayOfForecast: 0,
		TempMax:       sql.NullFloat64{Float64: 27, Valid: true},
		TempMin:       sql.NullFloat64{Float64: 11, Valid: true},
		Narrative:     sql.NullString{String: "Partly cloudy.", Valid: true},
	})
	insertForecastForLiveBOMTest(t, s, models.Forecast{
		Source:        "bom",
		FetchedAt:     fetchedAt,
		ValidDate:     validDate,
		DayOfForecast: 0,
		TempMax:       sql.NullFloat64{Float64: 22, Valid: true},
		TempMin:       sql.NullFloat64{Float64: 8, Valid: true},
		PrecipRange:   sql.NullString{String: "0 to 1 mm", Valid: true},
		Narrative:     sql.NullString{String: "Legacy BOM narrative.", Valid: true},
	})
	insertForecastForLiveBOMTest(t, s, models.Forecast{
		Source:        "bom_daily_api",
		FetchedAt:     fetchedAt,
		ValidDate:     validDate,
		DayOfForecast: 0,
		TempMax:       sql.NullFloat64{Float64: 31, Valid: true},
		TempMin:       sql.NullFloat64{Float64: 9, Valid: true},
		PrecipRange:   sql.NullString{String: "3 to 8 mm", Valid: true},
		Narrative:     sql.NullString{String: "Daily API narrative.", Valid: true},
	})
	if err := s.UpsertCorrectionStats(store.CorrectionStats{
		Source:        "bom",
		Target:        "tmax",
		DayOfForecast: 0,
		Regime:        "all",
		WindowDays:    30,
		SampleSize:    10,
		MeanBias:      2,
	}); err != nil {
		t.Fatalf("upsert legacy BOM correction stats: %v", err)
	}
	if err := s.UpsertCorrectionStats(store.CorrectionStats{
		Source:        "bom_daily_api",
		Target:        "tmax",
		DayOfForecast: 0,
		Regime:        "all",
		WindowDays:    30,
		SampleSize:    10,
		MeanBias:      5,
	}); err != nil {
		t.Fatalf("upsert daily API correction stats: %v", err)
	}

	srv := api.NewServer(s, "8080", loc, nil)
	req := httptest.NewRequest("GET", "/api/forecast", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var response struct {
		Days []struct {
			BOM struct {
				Source  string `json:"Source"`
				TempMax struct {
					Float64 float64 `json:"Float64"`
					Valid   bool    `json:"Valid"`
				} `json:"TempMax"`
			} `json:"BOM"`
			BOMCorrectedMax    *float64 `json:"bom_corrected_max"`
			PrecipDisplay      string   `json:"precip_display"`
			GeneratedNarrative string   `json:"generated_narrative"`
		} `json:"Days"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode forecast response: %v", err)
	}
	if len(response.Days) == 0 {
		t.Fatal("expected forecast days")
	}
	if response.Days[0].BOM.Source != "bom_daily_api" {
		t.Fatalf("expected live BOM source bom_daily_api, got %q", response.Days[0].BOM.Source)
	}
	if !response.Days[0].BOM.TempMax.Valid || response.Days[0].BOM.TempMax.Float64 != 31 {
		t.Fatalf("expected daily API max 31, got %+v", response.Days[0].BOM.TempMax)
	}
	if response.Days[0].BOMCorrectedMax == nil {
		t.Fatal("expected daily API corrected max 26, got nil")
	}
	if *response.Days[0].BOMCorrectedMax != 26 {
		t.Fatalf("expected daily API corrected max 26, got %.1f", *response.Days[0].BOMCorrectedMax)
	}
	if response.Days[0].PrecipDisplay != "3–8mm" {
		t.Fatalf("expected daily API precip range, got %q", response.Days[0].PrecipDisplay)
	}
	if !strings.Contains(response.Days[0].GeneratedNarrative, "Daily API narrative") {
		t.Fatalf("expected daily API narrative, got %q", response.Days[0].GeneratedNarrative)
	}
}

func insertForecastForLiveBOMTest(t *testing.T, s *store.Store, forecast models.Forecast) {
	t.Helper()
	if err := s.InsertForecast(forecast); err != nil {
		t.Fatalf("insert %s forecast: %v", forecast.Source, err)
	}
}

func TestAccuracyPage_NoData(t *testing.T) {
	t.Parallel()
	s, loc := setupTestStore(t)
	srv := api.NewServer(s, "8080", loc, nil)

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

	srv := api.NewServer(s, "8080", loc, nil)
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

func TestCurrentPartial_IncludesAirQuality(t *testing.T) {
	t.Parallel()

	s, loc := setupTestStore(t)
	if err := s.UpsertStation(models.Station{StationID: "PRIMARY", Name: "Primary", ElevationTier: "valley_floor", IsPrimary: true, Active: true}); err != nil {
		t.Fatalf("upsert primary station: %v", err)
	}
	insertObservation(t, s, "PRIMARY", time.Now().UTC().Add(-5*time.Minute), 13)

	srv := api.NewServer(s, "8080", loc, fakeAirQualityProvider{
		reading: &ecowitt.AirQualityReading{
			ObservedAt:     time.Now().UTC().Add(-5 * time.Minute),
			PM25:           2.8,
			RealTimeAQI:    12,
			HasRealTimeAQI: true,
			AQI24H:         47,
			HasAQI24H:      true,
			Category:       "Good",
			CategoryClass:  "good",
		},
	})

	req := httptest.NewRequest("GET", "/partials/current", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "AQI 12") {
		t.Fatalf("expected AQI in current partial, body=%q", body)
	}
	if !strings.Contains(body, "PM2.5 2.8 ug/m3") {
		t.Fatalf("expected PM2.5 in current partial, body=%q", body)
	}
	if !strings.Contains(body, "24h AQI 47") {
		t.Fatalf("expected 24h AQI in current partial, body=%q", body)
	}
}

func TestCurrentPartial_FallsBackToStoredAirQuality(t *testing.T) {
	t.Parallel()

	s, loc := setupTestStore(t)
	if err := s.UpsertStation(models.Station{StationID: "PRIMARY", Name: "Primary", ElevationTier: "valley_floor", IsPrimary: true, Active: true}); err != nil {
		t.Fatalf("upsert primary station: %v", err)
	}
	insertObservation(t, s, "PRIMARY", time.Now().UTC().Add(-5*time.Minute), 13)

	if _, err := s.UpsertAirQualityReadings([]ecowitt.AirQualityReading{{
		ObservedAt:     time.Now().UTC().Add(-10 * time.Minute),
		PM25:           18.4,
		RealTimeAQI:    61,
		HasRealTimeAQI: true,
		AQI24H:         55,
		HasAQI24H:      true,
		Category:       "Moderate",
		CategoryClass:  "moderate",
		SourceFieldKey: "pm25_ch1",
	}}); err != nil {
		t.Fatalf("UpsertAirQualityReadings: %v", err)
	}

	srv := api.NewServer(s, "8080", loc, fakeAirQualityProvider{err: errors.New("boom")})

	req := httptest.NewRequest("GET", "/partials/current", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "AQI 61") {
		t.Fatalf("expected stored AQI fallback in current partial, body=%q", body)
	}
	if !strings.Contains(body, "PM2.5 18.4 ug/m3") {
		t.Fatalf("expected stored PM2.5 fallback in current partial, body=%q", body)
	}
}

func TestCurrentPartial_IgnoresStaleLiveAirQualityReading(t *testing.T) {
	t.Parallel()

	s, loc := setupTestStore(t)
	if err := s.UpsertStation(models.Station{StationID: "PRIMARY", Name: "Primary", ElevationTier: "valley_floor", IsPrimary: true, Active: true}); err != nil {
		t.Fatalf("upsert primary station: %v", err)
	}
	insertObservation(t, s, "PRIMARY", time.Now().UTC().Add(-5*time.Minute), 13)

	if _, err := s.UpsertAirQualityReadings([]ecowitt.AirQualityReading{{
		ObservedAt:     time.Now().UTC().Add(-10 * time.Minute),
		PM25:           18.4,
		RealTimeAQI:    61,
		HasRealTimeAQI: true,
		AQI24H:         55,
		HasAQI24H:      true,
		Category:       "Moderate",
		CategoryClass:  "moderate",
		SourceFieldKey: "pm25_ch1",
	}}); err != nil {
		t.Fatalf("UpsertAirQualityReadings: %v", err)
	}

	srv := api.NewServer(s, "8080", loc, fakeAirQualityProvider{
		reading: &ecowitt.AirQualityReading{
			ObservedAt:     time.Now().UTC().Add(-2 * time.Hour),
			PM25:           99.9,
			RealTimeAQI:    188,
			HasRealTimeAQI: true,
			Category:       "Unhealthy",
			CategoryClass:  "unhealthy",
		},
		err: errors.New("stale cached live value"),
	})

	req := httptest.NewRequest("GET", "/partials/current", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "AQI 61") {
		t.Fatalf("expected stored AQI to win over stale live reading, body=%q", body)
	}
	if strings.Contains(body, "AQI 188") {
		t.Fatalf("did not expect stale live AQI in current partial, body=%q", body)
	}
	if strings.Contains(body, "PM2.5 99.9 ug/m3") {
		t.Fatalf("did not expect stale live PM2.5 in current partial, body=%q", body)
	}
}

func TestChartPartial_IncludesAirQualityChart(t *testing.T) {
	t.Parallel()

	s, loc := setupTestStore(t)
	if err := s.UpsertStation(models.Station{StationID: "PRIMARY", Name: "Primary", ElevationTier: "valley_floor", IsPrimary: true, Active: true}); err != nil {
		t.Fatalf("upsert primary station: %v", err)
	}
	insertObservation(t, s, "PRIMARY", time.Now().UTC().Add(-10*time.Minute), 14)

	if _, err := s.UpsertAirQualityReadings([]ecowitt.AirQualityReading{{
		ObservedAt:     time.Now().UTC().Add(-5 * time.Minute),
		PM25:           27.4,
		RealTimeAQI:    82,
		HasRealTimeAQI: true,
		SourceFieldKey: "pm25_ch1",
	}}); err != nil {
		t.Fatalf("UpsertAirQualityReadings: %v", err)
	}

	srv := api.NewServer(s, "8080", loc, nil)
	req := httptest.NewRequest("GET", "/partials/chart", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Last 24 Hours — Air Quality") {
		t.Fatalf("expected air quality chart heading, body=%q", body)
	}
	if !strings.Contains(body, "airQualityChart") {
		t.Fatalf("expected air quality chart canvas, body=%q", body)
	}
	if !strings.Contains(body, "label: 'AQI'") {
		t.Fatalf("expected AQI dataset in chart script, body=%q", body)
	}
}

func TestChartPartial_RendersPM25OnlyAirQualityChart(t *testing.T) {
	t.Parallel()

	s, loc := setupTestStore(t)
	if err := s.UpsertStation(models.Station{StationID: "PRIMARY", Name: "Primary", ElevationTier: "valley_floor", IsPrimary: true, Active: true}); err != nil {
		t.Fatalf("upsert primary station: %v", err)
	}
	insertObservation(t, s, "PRIMARY", time.Now().UTC().Add(-10*time.Minute), 14)

	if _, err := s.UpsertAirQualityReadings([]ecowitt.AirQualityReading{{
		ObservedAt:     time.Now().UTC().Add(-10 * time.Minute),
		PM25:           27.4,
		SourceFieldKey: "pm25_ch1",
	}}); err != nil {
		t.Fatalf("UpsertAirQualityReadings: %v", err)
	}

	srv := api.NewServer(s, "8080", loc, nil)
	req := httptest.NewRequest("GET", "/partials/chart", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Last 24 Hours — Air Quality") {
		t.Fatalf("expected air quality chart heading, body=%q", body)
	}
	if !strings.Contains(body, "airQualityChart") {
		t.Fatalf("expected air quality chart canvas, body=%q", body)
	}
	if !strings.Contains(body, `"has_aqi":false`) {
		t.Fatalf("expected PM2.5-only chart data to mark has_aqi false, body=%q", body)
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

	srv := api.NewServer(s, "8080", loc, nil)
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
