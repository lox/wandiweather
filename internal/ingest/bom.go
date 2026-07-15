package ingest

import (
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jlaffaye/ftp"
	"github.com/lox/wandiweather/internal/httputil"
	"github.com/lox/wandiweather/internal/models"
)

const (
	bomFTPHost         = "ftp.bom.gov.au:21"
	bomForecastFile    = "/anon/gen/fwo/IDV10753.xml"
	wangarattaAAC      = "VIC_PT075"
	bomAPIBaseURL      = "https://api.weather.bom.gov.au/v1"
	wangarattaGeohash6 = "r3811m"
	bomSource          = "bom"
	bomDailyAPISource  = "bom_daily_api"
)

type bomForecastSource interface {
	Source() string
	Endpoint() string
	LocationID() string
	FetchForecasts() ([]models.Forecast, string, *FetchResult, error)
}

type BOMClient struct {
	areaCode string
}

func NewBOMClient(areaCode string) *BOMClient {
	areaCode = strings.TrimSpace(areaCode)
	if areaCode == "" {
		areaCode = wangarattaAAC
	}
	return &BOMClient{areaCode: areaCode}
}

func (b *BOMClient) Source() string {
	return bomSource
}

func (b *BOMClient) Endpoint() string {
	return "forecast/fwo"
}

func (b *BOMClient) LocationID() string {
	return b.areaCode
}

type BOMDailyAPIClient struct {
	locationID string
	baseURL    string
	client     *http.Client
}

func NewBOMDailyAPIClient(locationID string) *BOMDailyAPIClient {
	locationID = strings.ToLower(strings.TrimSpace(locationID))
	if locationID == "" {
		locationID = wangarattaGeohash6
	}

	return &BOMDailyAPIClient{
		locationID: locationID,
		baseURL:    bomAPIBaseURL,
		client:     newBOMDailyAPIHTTPClient(nil),
	}
}

func newBOMDailyAPIHTTPClient(dialContext func(context.Context, string, string) (net.Conn, error)) *http.Client {
	if dialContext == nil {
		dialer := &net.Dialer{Timeout: httputil.DefaultTimeout}
		dialContext = dialer.DialContext
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, _ string, address string) (net.Conn, error) {
		return dialContext(ctx, "tcp4", address)
	}

	return &http.Client{
		Timeout:   httputil.DefaultTimeout,
		Transport: transport,
	}
}

func (b *BOMDailyAPIClient) Source() string {
	return bomDailyAPISource
}

func (b *BOMDailyAPIClient) Endpoint() string {
	return fmt.Sprintf("locations/%s/forecasts/daily", b.locationID)
}

func (b *BOMDailyAPIClient) LocationID() string {
	return b.locationID
}

type bomProduct struct {
	XMLName  xml.Name       `xml:"product"`
	Forecast bomForecastDoc `xml:"forecast"`
}

type bomForecastDoc struct {
	Areas []bomArea `xml:"area"`
}

type bomArea struct {
	AAC     string              `xml:"aac,attr"`
	Type    string              `xml:"type,attr"`
	Periods []bomForecastPeriod `xml:"forecast-period"`
}

type bomForecastPeriod struct {
	Index     int          `xml:"index,attr"`
	StartTime string       `xml:"start-time-utc,attr"`
	Elements  []bomElement `xml:"element"`
	TextItems []bomText    `xml:"text"`
}

type bomElement struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

type bomText struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
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

	conn, err := ftp.Dial(bomFTPHost, ftp.DialWithTimeout(30*time.Second))
	if err != nil {
		result.Error = fmt.Errorf("ftp dial: %w", err)
		return nil, "", result, result.Error
	}
	defer conn.Quit()

	if err := conn.Login("anonymous", "anonymous"); err != nil {
		result.Error = fmt.Errorf("ftp login: %w", err)
		return nil, "", result, result.Error
	}

	resp, err := conn.Retr(bomForecastFile)
	if err != nil {
		result.Error = fmt.Errorf("ftp retr: %w", err)
		return nil, "", result, result.Error
	}
	defer resp.Close()

	body, err := io.ReadAll(resp)
	if err != nil {
		result.Error = fmt.Errorf("read body: %w", err)
		return nil, "", result, result.Error
	}
	result.ResponseSize = len(body)
	result.HTTPStatus = 200

	forecasts, parseResult, err := parseLegacyBOMForecasts(body, b.areaCode, time.Now().UTC())
	mergeFetchResult(result, parseResult)
	if err != nil {
		result.Error = err
		return nil, string(body), result, err
	}

	return forecasts, string(body), result, nil
}

func (b *BOMDailyAPIClient) FetchForecasts() ([]models.Forecast, string, *FetchResult, error) {
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

	forecasts, parseResult, err := parseBOMDailyAPIForecasts(body, b.locationID, time.Now().UTC())
	mergeFetchResult(result, parseResult)
	if err != nil {
		result.Error = err
		return nil, string(body), result, err
	}

	return forecasts, string(body), result, nil
}

func parseLegacyBOMForecasts(body []byte, areaCode string, fetchedAt time.Time) ([]models.Forecast, *FetchResult, error) {
	result := &FetchResult{}

	var product bomProduct
	if err := xml.Unmarshal(body, &product); err != nil {
		return nil, result, fmt.Errorf("unmarshal xml: %w", err)
	}

	var targetArea *bomArea
	for i := range product.Forecast.Areas {
		if product.Forecast.Areas[i].AAC == areaCode && product.Forecast.Areas[i].Type == "location" {
			targetArea = &product.Forecast.Areas[i]
			break
		}
	}
	if targetArea == nil {
		return nil, result, fmt.Errorf("area %s not found in forecast", areaCode)
	}

	mel, _ := time.LoadLocation("Australia/Melbourne")
	var forecasts []models.Forecast
	var parseErrors []string

	for _, period := range targetArea.Periods {
		startTime, err := time.Parse(time.RFC3339, period.StartTime)
		if err != nil {
			parseErrors = append(parseErrors, fmt.Sprintf("period[%d].StartTime=%q: %v", period.Index, period.StartTime, err))
			continue
		}
		localStart := startTime.In(mel)
		validDate := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), 0, 0, 0, 0, time.UTC)

		fc := models.Forecast{
			Source:        bomSource,
			FetchedAt:     fetchedAt,
			ValidDate:     validDate,
			DayOfForecast: period.Index,
			LocationID:    sql.NullString{String: areaCode, Valid: true},
		}

		for _, elem := range period.Elements {
			switch elem.Type {
			case "air_temperature_maximum":
				if v, err := strconv.ParseFloat(strings.TrimSpace(elem.Value), 64); err == nil {
					fc.TempMax = sql.NullFloat64{Float64: v, Valid: true}
				}
			case "air_temperature_minimum":
				if v, err := strconv.ParseFloat(strings.TrimSpace(elem.Value), 64); err == nil {
					fc.TempMin = sql.NullFloat64{Float64: v, Valid: true}
				}
			case "precipitation_range":
				value := strings.TrimSpace(elem.Value)
				fc.PrecipRange = sql.NullString{String: value, Valid: value != ""}
				fc.PrecipAmount = parsePrecipRange(value)
			}
		}

		for _, text := range period.TextItems {
			switch text.Type {
			case "precis":
				value := strings.TrimSpace(text.Value)
				fc.Narrative = sql.NullString{String: value, Valid: value != ""}
			case "probability_of_precipitation":
				s := strings.TrimSpace(text.Value)
				if len(s) > 0 && s[len(s)-1] == '%' {
					if v, err := strconv.Atoi(s[:len(s)-1]); err == nil {
						fc.PrecipChance = sql.NullInt64{Int64: int64(v), Valid: true}
					}
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

	return forecasts, result, nil
}

func parseBOMDailyAPIForecasts(body []byte, locationID string, fetchedAt time.Time) ([]models.Forecast, *FetchResult, error) {
	result := &FetchResult{}

	var data bomDailyResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, result, fmt.Errorf("unmarshal: %w", err)
	}

	mel, _ := time.LoadLocation("Australia/Melbourne")
	var forecasts []models.Forecast
	var parseErrors []string

	for i, day := range data.Data {
		dayTime, err := time.Parse(time.RFC3339, day.Date)
		if err != nil {
			parseErrors = append(parseErrors, fmt.Sprintf("data[%d].date=%q: %v", i, day.Date, err))
			continue
		}
		localDate := dayTime.In(mel)
		validDate := time.Date(localDate.Year(), localDate.Month(), localDate.Day(), 0, 0, 0, 0, time.UTC)

		fc := models.Forecast{
			Source:        bomDailyAPISource,
			FetchedAt:     fetchedAt,
			ValidDate:     validDate,
			DayOfForecast: i,
			LocationID:    sql.NullString{String: locationID, Valid: true},
		}

		if day.TempMax != nil {
			fc.TempMax = sql.NullFloat64{Float64: *day.TempMax, Valid: true}
		}
		if day.TempMin != nil {
			fc.TempMin = sql.NullFloat64{Float64: *day.TempMin, Valid: true}
		}

		fc.NarrativeShort = trimmedNullString(day.ShortText)
		fc.NarrativeExtended = trimmedNullString(day.ExtendedText)
		if narrative := pickNarrative(day.ShortText, day.ExtendedText); narrative != "" {
			fc.Narrative = sql.NullString{String: narrative, Valid: true}
		}

		if day.Rain != nil {
			if day.Rain.Chance != nil {
				fc.PrecipChance = sql.NullInt64{Int64: int64(*day.Rain.Chance), Valid: true}
			}
			if day.Rain.Amount != nil {
				fc.PrecipMin = nullFloat64(day.Rain.Amount.Min)
				fc.PrecipMax = nullFloat64(day.Rain.Amount.Max)
				units := strings.TrimSpace(day.Rain.Amount.Units)
				if units == "" && (day.Rain.Amount.Min != nil || day.Rain.Amount.Max != nil) {
					units = "mm"
				}
				if units != "" {
					fc.PrecipUnits = sql.NullString{String: units, Valid: true}
				}
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

	return forecasts, result, nil
}

func mergeFetchResult(dst, src *FetchResult) {
	if dst == nil || src == nil {
		return
	}
	dst.RecordCount = src.RecordCount
	dst.ParseErrors = src.ParseErrors
	dst.ParseError = src.ParseError
}

func trimmedNullString(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: trimmed, Valid: true}
}

func nullFloat64(value *float64) sql.NullFloat64 {
	if value == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *value, Valid: true}
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
