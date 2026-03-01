package ingest

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lox/wandiweather/internal/httputil"
	"github.com/lox/wandiweather/internal/models"
)

const (
	bomAPIBaseURL      = "https://api.weather.bom.gov.au/v1"
	wangarattaGeohash6 = "r3811m"
)

type BOMClient struct {
	locationID string
	baseURL    string
	client     *http.Client
}

func NewBOMClient(locationID string) *BOMClient {
	locationID = strings.ToLower(strings.TrimSpace(locationID))
	if locationID == "" {
		locationID = wangarattaGeohash6
	}

	return &BOMClient{
		locationID: locationID,
		baseURL:    bomAPIBaseURL,
		client:     httputil.NewClient(),
	}
}

func (b *BOMClient) Endpoint() string {
	return fmt.Sprintf("locations/%s/forecasts/daily", b.locationID)
}

type bomDailyResponse struct {
	Data []bomDailyForecast `json:"data"`
}

type bomDailyForecast struct {
	Date         string   `json:"date"`
	TempMax      *float64 `json:"temp_max"`
	TempMin      *float64 `json:"temp_min"`
	ExtendedText *string  `json:"extended_text"`
	ShortText    *string  `json:"short_text"`
	Rain         *bomRain `json:"rain"`
}

type bomRain struct {
	Amount *bomRainAmount `json:"amount"`
	Chance *int           `json:"chance"`
}

type bomRainAmount struct {
	Min   *float64 `json:"min"`
	Max   *float64 `json:"max"`
	Units string   `json:"units"`
}

func (b *BOMClient) FetchForecasts() ([]models.Forecast, string, *FetchResult, error) {
	result := &FetchResult{}
	url := fmt.Sprintf("%s/%s", strings.TrimRight(b.baseURL, "/"), b.Endpoint())

	resp, err := b.client.Get(url)
	if err != nil {
		result.Error = fmt.Errorf("fetch daily forecast: %w", err)
		return nil, "", result, result.Error
	}
	defer resp.Body.Close()

	result.HTTPStatus = resp.StatusCode

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = fmt.Errorf("read body: %w", err)
		return nil, "", result, result.Error
	}
	result.ResponseSize = len(body)

	if resp.StatusCode != http.StatusOK {
		result.Error = fmt.Errorf("fetch daily forecast: status %d: %s", resp.StatusCode, truncateBody(body))
		return nil, string(body), result, result.Error
	}

	var data bomDailyResponse
	if err := json.Unmarshal(body, &data); err != nil {
		result.Error = fmt.Errorf("unmarshal: %w", err)
		return nil, string(body), result, result.Error
	}

	fetchedAt := time.Now().UTC()
	var forecasts []models.Forecast
	var parseErrors []string

	mel, _ := time.LoadLocation("Australia/Melbourne")

	for i, day := range data.Data {
		dayTime, err := time.Parse(time.RFC3339, day.Date)
		if err != nil {
			parseErrors = append(parseErrors, fmt.Sprintf("data[%d].date=%q: %v", i, day.Date, err))
			continue
		}
		localDate := dayTime.In(mel)
		validDate := time.Date(localDate.Year(), localDate.Month(), localDate.Day(), 0, 0, 0, 0, time.UTC)

		fc := models.Forecast{
			Source:        "bom",
			FetchedAt:     fetchedAt,
			ValidDate:     validDate,
			DayOfForecast: i,
			RawJSON:       "", // Don't store raw JSON to save memory
			LocationID:    sql.NullString{String: b.locationID, Valid: true},
		}

		if day.TempMax != nil {
			fc.TempMax = sql.NullFloat64{Float64: *day.TempMax, Valid: true}
		}
		if day.TempMin != nil {
			fc.TempMin = sql.NullFloat64{Float64: *day.TempMin, Valid: true}
		}
		if narrative := pickNarrative(day.ShortText, day.ExtendedText); narrative != "" {
			fc.Narrative = sql.NullString{String: narrative, Valid: true}
		}

		if day.Rain != nil {
			if day.Rain.Chance != nil {
				fc.PrecipChance = sql.NullInt64{Int64: int64(*day.Rain.Chance), Valid: true}
			}
			if day.Rain.Amount != nil {
				fc.PrecipRange = buildPrecipRange(day.Rain.Amount.Min, day.Rain.Amount.Max, day.Rain.Amount.Units)
				if fc.PrecipRange.Valid {
					fc.PrecipAmount = parsePrecipRange(fc.PrecipRange.String)
				}
			}
		}

		forecasts = append(forecasts, fc)
	}

	result.RecordCount = len(forecasts)
	if len(parseErrors) > 0 {
		result.ParseErrors = len(parseErrors)
		result.ParseError = fmt.Sprintf("%d parse errors: %v", len(parseErrors), parseErrors[0])
	}

	return forecasts, string(body), result, nil
}

func pickNarrative(shortText, extendedText *string) string {
	if shortText != nil {
		if s := strings.TrimSpace(*shortText); s != "" {
			return s
		}
	}
	if extendedText != nil {
		return strings.TrimSpace(*extendedText)
	}
	return ""
}

func buildPrecipRange(min, max *float64, units string) sql.NullString {
	if min == nil && max == nil {
		return sql.NullString{}
	}

	units = strings.TrimSpace(units)
	if units == "" {
		units = "mm"
	}

	format := func(v *float64) string {
		if v == nil {
			return ""
		}
		return strconv.FormatFloat(*v, 'f', -1, 64)
	}

	switch {
	case min != nil && max != nil:
		if *min == *max {
			return sql.NullString{String: fmt.Sprintf("%s %s", format(max), units), Valid: true}
		}
		return sql.NullString{String: fmt.Sprintf("%s to %s %s", format(min), format(max), units), Valid: true}
	case max != nil:
		return sql.NullString{String: fmt.Sprintf("%s %s", format(max), units), Valid: true}
	default:
		return sql.NullString{String: fmt.Sprintf("%s %s", format(min), units), Valid: true}
	}
}

// parsePrecipRange extracts the upper bound from a BOM precipitation range string
// like "0 to 5 mm" and returns it as a NullFloat64. This provides a comparable
// numeric value to WU's QPF for forecast verification.
func parsePrecipRange(s string) sql.NullFloat64 {
	if s == "" {
		return sql.NullFloat64{}
	}
	// Format: "X to Y mm" — extract Y (upper bound)
	var lo, hi float64
	if n, err := fmt.Sscanf(s, "%f to %f mm", &lo, &hi); err == nil && n == 2 {
		return sql.NullFloat64{Float64: hi, Valid: true}
	}
	// Single value like "5 mm"
	var v float64
	if n, err := fmt.Sscanf(s, "%f mm", &v); err == nil && n == 1 {
		return sql.NullFloat64{Float64: v, Valid: true}
	}
	return sql.NullFloat64{}
}
