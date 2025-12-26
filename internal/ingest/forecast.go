package ingest

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/lox/wandiweather/internal/models"
)

type ForecastClient struct {
	apiKey string
	client *http.Client
	lat    float64
	lon    float64
}

func NewForecastClient(apiKey string, lat, lon float64) *ForecastClient {
	return &ForecastClient{
		apiKey: apiKey,
		client: &http.Client{Timeout: 30 * time.Second},
		lat:    lat,
		lon:    lon,
	}
}

type ForecastResponse struct {
	DayOfWeek            []string   `json:"dayOfWeek"`
	ValidTimeLocal       []string   `json:"validTimeLocal"`
	ExpirationTimeUtc    []int64    `json:"expirationTimeUtc"`
	CalendarDayTempMax   []float64  `json:"calendarDayTemperatureMax"`
	CalendarDayTempMin   []float64  `json:"calendarDayTemperatureMin"`
	DaypartName          []string   `json:"daypartName"`
	Narrative            []string   `json:"narrative"`
	Daypart              []Daypart  `json:"daypart"`
}

type Daypart struct {
	DaypartName       []*string  `json:"daypartName"`
	Narrative         []*string  `json:"narrative"`
	PrecipChance      []*int     `json:"precipChance"`
	PrecipType        []*string  `json:"precipType"`
	QPF               []*float64 `json:"qpf"`
	RelativeHumidity  []*int     `json:"relativeHumidity"`
	Temperature       []*int     `json:"temperature"`
	WindDirection     []*int     `json:"windDirection"`
	WindDirectionCard []*string  `json:"windDirectionCardinal"`
	WindSpeed         []*int     `json:"windSpeed"`
}

func (f *ForecastClient) Fetch7Day() ([]models.Forecast, string, error) {
	url := fmt.Sprintf("https://api.weather.com/v3/wx/forecast/daily/5day?geocode=%.4f,%.4f&format=json&units=m&language=en-AU&apiKey=%s", f.lat, f.lon, f.apiKey)

	resp, err := f.client.Get(url)
	if err != nil {
		return nil, "", fmt.Errorf("fetch forecast: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("fetch forecast: status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read body: %w", err)
	}

	var data ForecastResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, "", fmt.Errorf("unmarshal: %w", err)
	}

	fetchedAt := time.Now().UTC()
	var forecasts []models.Forecast

	for i := range data.ValidTimeLocal {
		validTime, err := time.Parse("2006-01-02T15:04:05-0700", data.ValidTimeLocal[i])
		if err != nil {
			continue
		}
		validDate := time.Date(validTime.Year(), validTime.Month(), validTime.Day(), 0, 0, 0, 0, time.UTC)

		fc := models.Forecast{
			Source:        "wu",
			FetchedAt:     fetchedAt,
			ValidDate:     validDate,
			DayOfForecast: i,
			RawJSON:       string(body),
		}

		if i < len(data.CalendarDayTempMax) {
			fc.TempMax = sql.NullFloat64{Float64: data.CalendarDayTempMax[i], Valid: true}
		}
		if i < len(data.CalendarDayTempMin) {
			fc.TempMin = sql.NullFloat64{Float64: data.CalendarDayTempMin[i], Valid: true}
		}
		if i < len(data.Narrative) {
			fc.Narrative = sql.NullString{String: data.Narrative[i], Valid: true}
		}

		forecasts = append(forecasts, fc)
	}

	return forecasts, string(body), nil
}
