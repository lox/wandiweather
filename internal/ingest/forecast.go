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

	var daypart *Daypart
	if len(data.Daypart) > 0 {
		daypart = &data.Daypart[0]
	}

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

		if daypart != nil {
			dayIdx := i * 2
			nightIdx := i*2 + 1

			var totalQPF float64
			var maxPrecipChance int
			var hasQPF, hasPrecip bool

			if dayIdx < len(daypart.QPF) && daypart.QPF[dayIdx] != nil {
				totalQPF += *daypart.QPF[dayIdx]
				hasQPF = true
			}
			if nightIdx < len(daypart.QPF) && daypart.QPF[nightIdx] != nil {
				totalQPF += *daypart.QPF[nightIdx]
				hasQPF = true
			}
			if dayIdx < len(daypart.PrecipChance) && daypart.PrecipChance[dayIdx] != nil {
				if *daypart.PrecipChance[dayIdx] > maxPrecipChance {
					maxPrecipChance = *daypart.PrecipChance[dayIdx]
				}
				hasPrecip = true
			}
			if nightIdx < len(daypart.PrecipChance) && daypart.PrecipChance[nightIdx] != nil {
				if *daypart.PrecipChance[nightIdx] > maxPrecipChance {
					maxPrecipChance = *daypart.PrecipChance[nightIdx]
				}
				hasPrecip = true
			}

			if hasQPF {
				fc.PrecipAmount = sql.NullFloat64{Float64: totalQPF, Valid: true}
			}
			if hasPrecip {
				fc.PrecipChance = sql.NullInt64{Int64: int64(maxPrecipChance), Valid: true}
			}

			// Get daytime wind (use day index, skip night)
			if dayIdx < len(daypart.WindSpeed) && daypart.WindSpeed[dayIdx] != nil {
				fc.WindSpeed = sql.NullFloat64{Float64: float64(*daypart.WindSpeed[dayIdx]), Valid: true}
			}
			if dayIdx < len(daypart.WindDirectionCard) && daypart.WindDirectionCard[dayIdx] != nil {
				fc.WindDir = sql.NullString{String: *daypart.WindDirectionCard[dayIdx], Valid: true}
			}
		}

		forecasts = append(forecasts, fc)
	}

	return forecasts, string(body), nil
}
