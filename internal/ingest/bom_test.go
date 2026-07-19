package ingest

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lox/wandiweather/internal/models"
)

func TestBOMDailyAPIHTTPClientForcesIPv4(t *testing.T) {
	var gotNetwork string
	client := newBOMDailyAPIHTTPClient(func(_ context.Context, network, _ string) (net.Conn, error) {
		gotNetwork = network
		return nil, errors.New("stop before dialing")
	})

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("client.Transport = %T, want *http.Transport", client.Transport)
	}

	_, _ = transport.DialContext(context.Background(), "tcp", "api.weather.bom.gov.au:443")

	if gotNetwork != "tcp4" {
		t.Fatalf("dial network = %q, want tcp4", gotNetwork)
	}
}

func TestBOMDailyAPIClientFetchForecasts_UsesWangarattaDailyAPI(t *testing.T) {
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

	client := NewBOMDailyAPIClient("r3811m")
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
	if first.Source != "bom_daily_api" {
		t.Fatalf("first.Source = %q, want bom_daily_api", first.Source)
	}
	if !first.Narrative.Valid || first.Narrative.String != "Rain. Heavy falls possible." {
		t.Fatalf("first.Narrative = %+v, want short_text narrative", first.Narrative)
	}
	if !first.NarrativeShort.Valid || first.NarrativeShort.String != "Rain. Heavy falls possible." {
		t.Fatalf("first.NarrativeShort = %+v, want short_text narrative", first.NarrativeShort)
	}
	if !first.NarrativeExtended.Valid || first.NarrativeExtended.String != "Cloudy with heavy rain." {
		t.Fatalf("first.NarrativeExtended = %+v, want extended_text narrative", first.NarrativeExtended)
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
	if !first.PrecipMin.Valid || first.PrecipMin.Float64 != 40 {
		t.Fatalf("first.PrecipMin = %+v, want 40", first.PrecipMin)
	}
	if !first.PrecipMax.Valid || first.PrecipMax.Float64 != 60 {
		t.Fatalf("first.PrecipMax = %+v, want 60", first.PrecipMax)
	}
	if !first.PrecipUnits.Valid || first.PrecipUnits.String != "mm" {
		t.Fatalf("first.PrecipUnits = %+v, want mm", first.PrecipUnits)
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
	if second.NarrativeShort.Valid {
		t.Fatalf("second.NarrativeShort = %+v, want invalid when short_text is null", second.NarrativeShort)
	}
	if !second.NarrativeExtended.Valid || second.NarrativeExtended.String != "Possible shower." {
		t.Fatalf("second.NarrativeExtended = %+v, want extended_text fallback", second.NarrativeExtended)
	}
	if !second.PrecipRange.Valid || second.PrecipRange.String != "0 mm" {
		t.Fatalf("second.PrecipRange = %+v, want 0 mm", second.PrecipRange)
	}
	if !second.PrecipAmount.Valid || second.PrecipAmount.Float64 != 0 {
		t.Fatalf("second.PrecipAmount = %+v, want 0", second.PrecipAmount)
	}
	if !second.PrecipMin.Valid || second.PrecipMin.Float64 != 0 {
		t.Fatalf("second.PrecipMin = %+v, want 0", second.PrecipMin)
	}
	if second.PrecipMax.Valid {
		t.Fatalf("second.PrecipMax = %+v, want invalid when max is null", second.PrecipMax)
	}

	if got, want := first.ValidDate, mustMelbourneValidDate(t, "2026-03-01T13:00:00Z"); !got.Equal(want) {
		t.Fatalf("first.ValidDate = %s, want %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
	if got, want := second.ValidDate, mustMelbourneValidDate(t, "2026-03-02T13:00:00Z"); !got.Equal(want) {
		t.Fatalf("second.ValidDate = %s, want %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func TestParseBOMHourlyAPIForecasts(t *testing.T) {
	fetchedAt := time.Date(2026, time.July, 15, 10, 30, 0, 0, time.UTC)
	body := []byte(`{
		"data": [
			{
				"time": "2026-07-15T11:00:00Z",
				"temp": 9,
				"temp_feels_like": 7,
				"dew_point": 6,
				"relative_humidity": 81,
				"is_night": true,
				"icon_descriptor": "shower",
				"rain": {
					"chance": 70,
					"amount": {"min": 0, "max": 2, "units": "mm"}
				},
				"wind": {
					"speed_kilometre": 12,
					"gust_speed_kilometre": 20,
					"direction": "SW"
				}
			},
			{
				"time": "not-a-time",
				"rain": {"chance": 10}
			}
		]
	}`)

	periods, result, err := parseBOMHourlyAPIForecasts(body, "r3811m", fetchedAt)
	if err != nil {
		t.Fatalf("parseBOMHourlyAPIForecasts: %v", err)
	}
	if len(periods) != 1 {
		t.Fatalf("len(periods) = %d, want 1", len(periods))
	}
	if result.ParseErrors != 1 || result.RecordCount != 1 {
		t.Fatalf("result = %+v, want one record and one parse error", result)
	}

	period := periods[0]
	if period.Source != "bom_hourly_api" || period.Period != "hourly" {
		t.Fatalf("period identity = %s/%s, want bom_hourly_api/hourly", period.Source, period.Period)
	}
	if period.PeriodStart != time.Date(2026, time.July, 15, 11, 0, 0, 0, time.UTC) {
		t.Fatalf("PeriodStart = %s", period.PeriodStart)
	}
	chance, ok := period.Component(models.ForecastMetricPrecipChance)
	if !ok || !chance.Value.Valid || chance.Value.Float64 != 70 {
		t.Fatalf("precipitation chance component = %+v, %v; want 70", chance, ok)
	}
	amount, ok := period.Component(models.ForecastMetricPrecipAmount)
	if !ok || !amount.ValueMax.Valid || amount.ValueMax.Float64 != 2 {
		t.Fatalf("precipitation amount component = %+v, %v; want max 2", amount, ok)
	}
	temperature, ok := period.Component(models.ForecastMetricTemperature)
	if !ok || !temperature.Value.Valid || temperature.Value.Float64 != 9 || !period.IsNight {
		t.Fatalf("hourly weather fields = %+v", period)
	}
}

func TestBOMDailyAPIClientFetchForecasts_TracksParseErrors(t *testing.T) {
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

	client := NewBOMDailyAPIClient("r3811m")
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

func TestParseLegacyBOMForecasts_UsesFTPLocationShape(t *testing.T) {
	fetchedAt := time.Date(2026, 3, 1, 1, 2, 3, 0, time.UTC)
	body := []byte(`
		<product>
		  <forecast>
		    <area aac="VIC_PT075" description="Wangaratta" type="location">
		      <forecast-period index="0" start-time-utc="2026-03-01T13:00:00Z" end-time-utc="2026-03-02T13:00:00Z">
		        <element type="air_temperature_maximum" units="C">21</element>
		        <element type="air_temperature_minimum" units="C">16</element>
		        <element type="precipitation_range" units="mm">40 to 60 mm</element>
		        <text type="precis">Rain. Heavy falls possible.</text>
		        <text type="probability_of_precipitation">100%</text>
		      </forecast-period>
		    </area>
		  </forecast>
		</product>
	`)

	forecasts, result, err := parseLegacyBOMForecasts(body, "VIC_PT075", fetchedAt)
	if err != nil {
		t.Fatalf("parseLegacyBOMForecasts() error = %v", err)
	}
	if result.RecordCount != 1 {
		t.Fatalf("RecordCount = %d, want 1", result.RecordCount)
	}
	if len(forecasts) != 1 {
		t.Fatalf("len(forecasts) = %d, want 1", len(forecasts))
	}

	forecast := forecasts[0]
	if forecast.Source != "bom" {
		t.Fatalf("forecast.Source = %q, want bom", forecast.Source)
	}
	if !forecast.LocationID.Valid || forecast.LocationID.String != "VIC_PT075" {
		t.Fatalf("forecast.LocationID = %+v, want VIC_PT075", forecast.LocationID)
	}
	if forecast.NarrativeShort.Valid {
		t.Fatalf("forecast.NarrativeShort = %+v, want invalid for FTP feed", forecast.NarrativeShort)
	}
	if forecast.NarrativeExtended.Valid {
		t.Fatalf("forecast.NarrativeExtended = %+v, want invalid for FTP feed", forecast.NarrativeExtended)
	}
	if forecast.PrecipMin.Valid {
		t.Fatalf("forecast.PrecipMin = %+v, want invalid for FTP feed", forecast.PrecipMin)
	}
	if forecast.PrecipMax.Valid {
		t.Fatalf("forecast.PrecipMax = %+v, want invalid for FTP feed", forecast.PrecipMax)
	}
	if forecast.PrecipUnits.Valid {
		t.Fatalf("forecast.PrecipUnits = %+v, want invalid for FTP feed", forecast.PrecipUnits)
	}
	if !forecast.PrecipRange.Valid || forecast.PrecipRange.String != "40 to 60 mm" {
		t.Fatalf("forecast.PrecipRange = %+v, want 40 to 60 mm", forecast.PrecipRange)
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
