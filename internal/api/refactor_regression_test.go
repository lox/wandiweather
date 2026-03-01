package api

import (
	"database/sql"
	"html/template"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/lox/wandiweather/internal/models"
	"github.com/lox/wandiweather/internal/store"

	_ "modernc.org/sqlite"
)

func setupRegressionServer(t *testing.T) (*Server, *store.Store) {
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

	srv := NewServer(st, "8080", time.UTC)
	return srv, st
}

func upsertActiveStation(t *testing.T, st *store.Store, stationID string, primary bool) {
	t.Helper()
	err := st.UpsertStation(models.Station{
		StationID:     stationID,
		Name:          stationID,
		ElevationTier: "valley_floor",
		IsPrimary:     primary,
		Active:        true,
	})
	if err != nil {
		t.Fatalf("upsert station %s: %v", stationID, err)
	}
}

func TestHandleCurrentPartial_RendersSubZeroValleyTemp(t *testing.T) {
	t.Parallel()

	srv, st := setupRegressionServer(t)
	upsertActiveStation(t, st, "CUSTOM_PRIMARY", true)

	now := time.Now().UTC().Truncate(time.Second)
	if err := st.InsertObservation(models.Observation{
		StationID:  "CUSTOM_PRIMARY",
		ObservedAt: now,
		Temp:       sql.NullFloat64{Float64: -3.2, Valid: true},
	}); err != nil {
		t.Fatalf("insert observation: %v", err)
	}

	req := httptest.NewRequest("GET", "/partials/current", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "<div class=\"temp-now\">-3<span class=\"unit\">°</span></div>") {
		t.Fatalf("expected rendered freezing temp, body: %s", body)
	}
	if strings.Contains(body, "<div class=\"temp-now\">—<span class=\"unit\">°</span></div>") {
		t.Fatalf("expected no missing-temp placeholder when freezing temp exists")
	}
}

func TestGetCurrentData_UsesConfiguredPrimaryStationForStats(t *testing.T) {
	t.Parallel()

	srv, st := setupRegressionServer(t)
	upsertActiveStation(t, st, "CUSTOM_PRIMARY", true)

	now := time.Now().UTC().Truncate(time.Second)
	if err := st.InsertObservation(models.Observation{
		StationID:  "CUSTOM_PRIMARY",
		ObservedAt: now.Add(-50 * time.Minute),
		Temp:       sql.NullFloat64{Float64: 6.0, Valid: true},
	}); err != nil {
		t.Fatalf("insert older observation: %v", err)
	}
	if err := st.InsertObservation(models.Observation{
		StationID:  "CUSTOM_PRIMARY",
		ObservedAt: now,
		Temp:       sql.NullFloat64{Float64: 8.0, Valid: true},
	}); err != nil {
		t.Fatalf("insert newer observation: %v", err)
	}

	data, err := srv.getCurrentData()
	if err != nil {
		t.Fatalf("getCurrentData: %v", err)
	}

	if data.TodayStats == nil || !data.TodayStats.MaxTempValid {
		t.Fatalf("expected today stats from configured primary station")
	}
	if data.TodayStats.MaxTemp < 8.0 {
		t.Fatalf("MaxTemp = %.1f, want >= 8.0", data.TodayStats.MaxTemp)
	}
	if data.TempChangeRate == nil {
		t.Fatalf("expected temp change rate from configured primary station")
	}
}

func TestHandleForecastPartial_RendersBOMTempsWhenWUUnavailable(t *testing.T) {
	t.Parallel()

	srv, st := setupRegressionServer(t)
	today := time.Now().UTC().Truncate(24 * time.Hour)

	if err := st.InsertForecast(models.Forecast{
		Source:        "bom",
		FetchedAt:     time.Now().UTC().Add(-2 * time.Hour),
		ValidDate:     today,
		DayOfForecast: 1,
		TempMax:       sql.NullFloat64{Float64: 31.0, Valid: true},
		TempMin:       sql.NullFloat64{Float64: 16.0, Valid: true},
		Narrative:     sql.NullString{String: "Mostly sunny.", Valid: true},
	}); err != nil {
		t.Fatalf("insert bom forecast: %v", err)
	}

	req := httptest.NewRequest("GET", "/partials/forecast", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "<div class=\"day-high\">31°</div>") {
		t.Fatalf("expected BOM high temp in output, body: %s", body)
	}
	if !strings.Contains(body, "<div class=\"day-low\">16°</div>") {
		t.Fatalf("expected BOM low temp in output, body: %s", body)
	}
}

func TestHandleAccuracy_ChartUsesNullForMissingSourceValues(t *testing.T) {
	t.Parallel()

	srv, st := setupRegressionServer(t)
	upsertActiveStation(t, st, "PRIMARY", true)

	baseDate := time.Now().UTC().Truncate(24 * time.Hour)
	wuDate := baseDate.AddDate(0, 0, -1)
	bomDate := baseDate.AddDate(0, 0, -2)

	insertVerificationRow := func(source string, day int, date time.Time, bias float64) {
		t.Helper()

		err := st.InsertForecast(models.Forecast{
			Source:        source,
			FetchedAt:     date.Add(-24 * time.Hour),
			ValidDate:     date,
			DayOfForecast: day,
			TempMax:       sql.NullFloat64{Float64: 25.0, Valid: true},
			TempMin:       sql.NullFloat64{Float64: 11.0, Valid: true},
		})
		if err != nil {
			t.Fatalf("insert forecast (%s): %v", source, err)
		}

		forecasts, err := st.GetForecastsForDate(date)
		if err != nil {
			t.Fatalf("GetForecastsForDate(%s): %v", date, err)
		}

		var forecastID int64
		for _, fc := range forecasts {
			if fc.Source == source && fc.DayOfForecast == day {
				forecastID = fc.ID
				break
			}
		}
		if forecastID == 0 {
			t.Fatalf("missing forecast id for %s day %d", source, day)
		}

		err = st.InsertForecastVerification(models.ForecastVerification{
			ForecastID:  forecastID,
			ValidDate:   date,
			BiasTempMax: sql.NullFloat64{Float64: bias, Valid: true},
			BiasTempMin: sql.NullFloat64{Float64: bias / 2, Valid: true},
		})
		if err != nil {
			t.Fatalf("insert verification (%s): %v", source, err)
		}
	}

	insertVerificationRow("wu", 1, wuDate, 1.5)
	insertVerificationRow("bom", 2, bomDate, -2.0)

	req := httptest.NewRequest("GET", "/accuracy", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	wuWithGap := regexp.MustCompile(`const wuData = \[[^\]]*null[^\]]*\];`)
	bomWithGap := regexp.MustCompile(`const bomData = \[[^\]]*null[^\]]*\];`)
	if !wuWithGap.MatchString(body) {
		t.Fatalf("expected WU chart series to include null gap, body: %s", body)
	}
	if !bomWithGap.MatchString(body) {
		t.Fatalf("expected BOM chart series to include null gap, body: %s", body)
	}
}

func TestPartialHandlers_Return500OnTemplateExecutionErrors(t *testing.T) {
	t.Parallel()

	srv, _ := setupRegressionServer(t)
	srv.tmpl = template.Must(template.New("broken").Parse(`{{define "index.html"}}ok{{end}}`))

	forecastReq := httptest.NewRequest("GET", "/partials/forecast", nil)
	forecastRes := httptest.NewRecorder()
	srv.Handler().ServeHTTP(forecastRes, forecastReq)
	if forecastRes.Code != 500 {
		t.Fatalf("forecast partial status = %d, want 500", forecastRes.Code)
	}

	chartReq := httptest.NewRequest("GET", "/partials/chart", nil)
	chartRes := httptest.NewRecorder()
	srv.Handler().ServeHTTP(chartRes, chartReq)
	if chartRes.Code != 500 {
		t.Fatalf("chart partial status = %d, want 500", chartRes.Code)
	}
}
