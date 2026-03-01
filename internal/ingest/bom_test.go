package ingest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBOMClientFetchForecasts_UsesWangarattaDailyAPI(t *testing.T) {
	requestedPath := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath <- r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": [
				{
					"date": "2026-03-01T13:00:00Z",
					"temp_max": 21,
					"temp_min": 16,
					"short_text": "Rain. Heavy falls possible.",
					"extended_text": "Cloudy with heavy rain.",
					"rain": {
						"amount": {
							"min": 40,
							"max": 60,
							"units": "mm"
						},
						"chance": 100
					}
				},
				{
					"date": "2026-03-02T13:00:00Z",
					"temp_max": 29,
					"temp_min": 17,
					"short_text": null,
					"extended_text": "Possible shower.",
					"rain": {
						"amount": {
							"min": 0,
							"max": null,
							"units": "mm"
						},
						"chance": 20
					}
				}
			]
		}`))
	}))
	defer server.Close()

	client := NewBOMClient("r3811m")
	client.baseURL = server.URL + "/v1"
	client.client = server.Client()

	forecasts, rawBody, result, err := client.FetchForecasts()
	if err != nil {
		t.Fatalf("FetchForecasts() error = %v", err)
	}

	if got := <-requestedPath; got != "/v1/locations/r3811m/forecasts/daily" {
		t.Fatalf("request path = %q, want /v1/locations/r3811m/forecasts/daily", got)
	}

	if result.HTTPStatus != http.StatusOK {
		t.Fatalf("HTTPStatus = %d, want 200", result.HTTPStatus)
	}
	if result.RecordCount != 2 {
		t.Fatalf("RecordCount = %d, want 2", result.RecordCount)
	}
	if result.ParseErrors != 0 {
		t.Fatalf("ParseErrors = %d, want 0", result.ParseErrors)
	}
	if !strings.Contains(rawBody, `"temp_max": 21`) {
		t.Fatalf("raw body did not contain expected payload: %q", rawBody)
	}

	if len(forecasts) != 2 {
		t.Fatalf("len(forecasts) = %d, want 2", len(forecasts))
	}

	first := forecasts[0]
	if first.DayOfForecast != 0 {
		t.Fatalf("first.DayOfForecast = %d, want 0", first.DayOfForecast)
	}
	if !first.TempMax.Valid || first.TempMax.Float64 != 21 {
		t.Fatalf("first.TempMax = %+v, want valid 21", first.TempMax)
	}
	if !first.TempMin.Valid || first.TempMin.Float64 != 16 {
		t.Fatalf("first.TempMin = %+v, want valid 16", first.TempMin)
	}
	if !first.Narrative.Valid || first.Narrative.String != "Rain. Heavy falls possible." {
		t.Fatalf("first.Narrative = %+v, want short_text narrative", first.Narrative)
	}
	if !first.PrecipChance.Valid || first.PrecipChance.Int64 != 100 {
		t.Fatalf("first.PrecipChance = %+v, want 100", first.PrecipChance)
	}
	if !first.PrecipRange.Valid || first.PrecipRange.String != "40 to 60 mm" {
		t.Fatalf("first.PrecipRange = %+v, want 40 to 60 mm", first.PrecipRange)
	}
	if !first.PrecipAmount.Valid || first.PrecipAmount.Float64 != 60 {
		t.Fatalf("first.PrecipAmount = %+v, want 60", first.PrecipAmount)
	}
	if !first.LocationID.Valid || first.LocationID.String != "r3811m" {
		t.Fatalf("first.LocationID = %+v, want r3811m", first.LocationID)
	}

	second := forecasts[1]
	if second.DayOfForecast != 1 {
		t.Fatalf("second.DayOfForecast = %d, want 1", second.DayOfForecast)
	}
	if !second.Narrative.Valid || second.Narrative.String != "Possible shower." {
		t.Fatalf("second.Narrative = %+v, want extended_text fallback", second.Narrative)
	}
	if !second.PrecipRange.Valid || second.PrecipRange.String != "0 mm" {
		t.Fatalf("second.PrecipRange = %+v, want 0 mm", second.PrecipRange)
	}
	if !second.PrecipAmount.Valid || second.PrecipAmount.Float64 != 0 {
		t.Fatalf("second.PrecipAmount = %+v, want 0", second.PrecipAmount)
	}

	if got, want := first.ValidDate, mustMelbourneValidDate(t, "2026-03-01T13:00:00Z"); !got.Equal(want) {
		t.Fatalf("first.ValidDate = %s, want %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
	if got, want := second.ValidDate, mustMelbourneValidDate(t, "2026-03-02T13:00:00Z"); !got.Equal(want) {
		t.Fatalf("second.ValidDate = %s, want %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func TestBOMClientFetchForecasts_TracksParseErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": [
				{"date": "not-a-date", "temp_max": 20, "temp_min": 10},
				{"date": "2026-03-01T13:00:00Z", "temp_max": 22, "temp_min": 12}
			]
		}`))
	}))
	defer server.Close()

	client := NewBOMClient("r3811m")
	client.baseURL = server.URL + "/v1"
	client.client = server.Client()

	forecasts, _, result, err := client.FetchForecasts()
	if err != nil {
		t.Fatalf("FetchForecasts() error = %v", err)
	}

	if len(forecasts) != 1 {
		t.Fatalf("len(forecasts) = %d, want 1", len(forecasts))
	}
	if result.RecordCount != 1 {
		t.Fatalf("RecordCount = %d, want 1", result.RecordCount)
	}
	if result.ParseErrors != 1 {
		t.Fatalf("ParseErrors = %d, want 1", result.ParseErrors)
	}
	if !strings.Contains(result.ParseError, `data[0].date="not-a-date"`) {
		t.Fatalf("ParseError = %q, want reference to invalid date", result.ParseError)
	}
}

func mustMelbourneValidDate(t *testing.T, s string) time.Time {
	t.Helper()

	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse test timestamp %q: %v", s, err)
	}

	mel, err := time.LoadLocation("Australia/Melbourne")
	if err != nil {
		t.Fatalf("load Melbourne timezone: %v", err)
	}

	localDate := ts.In(mel)
	return time.Date(localDate.Year(), localDate.Month(), localDate.Day(), 0, 0, 0, 0, time.UTC)
}
