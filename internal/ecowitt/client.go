package ecowitt

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lox/wandiweather/internal/httputil"
)

const defaultBaseURL = "https://api.ecowitt.net/api/v3/device"

// HistoryCycleAuto lets Ecowitt choose the history resolution.
const HistoryCycleAuto = "auto"

// HistoryCycle5Min requests 5-minute Ecowitt history samples.
const HistoryCycle5Min = "5min"

// AirQualityReading is the latest WH41 air quality reading from Ecowitt.
type AirQualityReading struct {
	ObservedAt     time.Time
	PM25           float64
	RealTimeAQI    int
	HasRealTimeAQI bool
	AQI24H         float64
	HasAQI24H      bool
	PM25Avg24H     float64
	HasPM25Avg24H  bool
	Category       string
	CategoryClass  string
	SourceFieldKey string
}

// FetchResult contains HTTP metadata for Ecowitt API calls.
type FetchResult struct {
	HTTPStatus   int
	ResponseSize int
	RecordCount  int
	Error        error
}

// Client fetches current air quality data from the Ecowitt cloud API.
type Client struct {
	appKey              string
	apiKey              string
	mac                 string
	baseURL             string
	client              *http.Client
	cacheTTL            time.Duration
	minRequestInterval  time.Duration
	rateLimitRetryDelay time.Duration
	rateLimitRetryCount int

	mu            sync.Mutex
	cached        *AirQualityReading
	cachedAt      time.Time
	requestMu     sync.Mutex
	lastRequestAt time.Time
}

// NewClient creates a new Ecowitt cloud API client.
func NewClient(appKey, apiKey, mac string) (*Client, error) {
	appKey = strings.TrimSpace(appKey)
	apiKey = strings.TrimSpace(apiKey)
	mac = strings.TrimSpace(mac)

	if appKey == "" {
		return nil, fmt.Errorf("missing Ecowitt application key")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("missing Ecowitt API key")
	}
	if mac == "" {
		return nil, fmt.Errorf("missing Ecowitt MAC address")
	}

	return &Client{
		appKey:              appKey,
		apiKey:              apiKey,
		mac:                 mac,
		baseURL:             defaultBaseURL,
		client:              httputil.NewClient(),
		cacheTTL:            time.Minute,
		minRequestInterval:  time.Second,
		rateLimitRetryDelay: 5 * time.Second,
		rateLimitRetryCount: 5,
	}, nil
}

// CurrentAirQuality returns the latest WH41 reading, using a short cache to avoid
// duplicate cloud requests during page renders and HTMX refreshes.
func (c *Client) CurrentAirQuality() (*AirQualityReading, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cached != nil && time.Since(c.cachedAt) < c.cacheTTL {
		reading := *c.cached
		return &reading, nil
	}

	reading, _, _, err := c.FetchCurrentAirQuality()
	if err != nil {
		if c.cached != nil {
			cached := *c.cached
			return &cached, fmt.Errorf("fetch current air quality: %w", err)
		}
		return nil, fmt.Errorf("fetch current air quality: %w", err)
	}

	c.cached = reading
	c.cachedAt = time.Now()

	result := *reading
	return &result, nil
}

type currentResponse struct {
	Code int                        `json:"code"`
	Msg  string                     `json:"msg"`
	Data map[string]airQualityBlock `json:"data"`
}

type apiEnvelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

type airQualityBlock struct {
	PM25        *measurement `json:"pm25"`
	RealTimeAQI *measurement `json:"real_time_aqi"`
	PM25Avg24H  *measurement `json:"pm25_avg_24h"`
	AQI24H      *measurement `json:"24_hours_aqi"`
}

type measurement struct {
	Time  string `json:"time"`
	Unit  string `json:"unit"`
	Value string `json:"value"`
}

type historyResponse struct {
	Code int                                      `json:"code"`
	Msg  string                                   `json:"msg"`
	Data map[string]map[string]historyMeasurement `json:"data"`
}

type historyMeasurement struct {
	Unit string            `json:"unit"`
	List map[string]string `json:"list"`
}

// FetchCurrentAirQuality fetches the latest WH41 reading without using the in-memory cache.
func (c *Client) FetchCurrentAirQuality() (*AirQualityReading, string, *FetchResult, error) {
	params := url.Values{}
	params.Set("application_key", c.appKey)
	params.Set("api_key", c.apiKey)
	params.Set("mac", c.mac)
	params.Set("call_back", "all")

	body, result, _, err := c.doAPIRequest("/real_time", params)
	if err != nil {
		return nil, string(body), result, err
	}

	var payload currentResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		result.Error = fmt.Errorf("decode response: %w", err)
		return nil, string(body), result, result.Error
	}

	reading, err := extractAirQualityReading(payload.Data)
	if err != nil {
		result.Error = err
		return nil, string(body), result, err
	}
	result.RecordCount = 1
	return reading, string(body), result, nil
}

// FetchAirQualityHistory fetches WH41 history samples for the requested time range.
func (c *Client) FetchAirQualityHistory(start, end time.Time, cycleType string) ([]AirQualityReading, string, *FetchResult, error) {
	if !start.Before(end) {
		return nil, "", nil, fmt.Errorf("invalid Ecowitt history range: start must be before end")
	}
	if cycleType == "" {
		cycleType = HistoryCycleAuto
	}

	params := url.Values{}
	params.Set("application_key", c.appKey)
	params.Set("api_key", c.apiKey)
	params.Set("mac", c.mac)
	params.Set("start_date", start.UTC().Format("2006-01-02 15:04:05"))
	params.Set("end_date", end.UTC().Format("2006-01-02 15:04:05"))
	params.Set("call_back", "pm25_ch1")
	params.Set("cycle_type", cycleType)

	body, result, _, err := c.doAPIRequest("/history", params)
	if err != nil {
		return nil, string(body), result, err
	}

	var payload historyResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		result.Error = fmt.Errorf("decode response: %w", err)
		return nil, string(body), result, result.Error
	}

	readings, err := extractAirQualityHistory(payload.Data)
	if err != nil {
		result.Error = err
		return nil, string(body), result, err
	}
	result.RecordCount = len(readings)
	return readings, string(body), result, nil
}

func (c *Client) doRequest(path string, params url.Values) ([]byte, *FetchResult, error) {
	result := &FetchResult{}
	c.waitForTurn()

	endpoint := c.baseURL + path + "?" + params.Encode()
	resp, err := c.client.Get(endpoint)
	if err != nil {
		result.Error = err
		return nil, result, err
	}
	defer resp.Body.Close()

	result.HTTPStatus = resp.StatusCode

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = fmt.Errorf("read body: %w", err)
		return nil, result, result.Error
	}
	result.ResponseSize = len(body)

	if resp.StatusCode != http.StatusOK {
		result.Error = fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		return body, result, result.Error
	}

	return body, result, nil
}

func (c *Client) doAPIRequest(path string, params url.Values) ([]byte, *FetchResult, *apiEnvelope, error) {
	attempts := c.rateLimitRetryCount
	if attempts < 1 {
		attempts = 1
	}

	for attempt := 0; attempt < attempts; attempt++ {
		body, result, err := c.doRequest(path, params)
		if err != nil {
			return body, result, nil, err
		}

		envelope, err := decodeAPIEnvelope(body)
		if err != nil {
			result.Error = fmt.Errorf("decode response: %w", err)
			return body, result, nil, result.Error
		}
		if envelope.Code == 0 {
			return body, result, envelope, nil
		}
		if isRateLimitedAPIError(envelope.Code, envelope.Msg) && attempt < attempts-1 {
			time.Sleep(c.rateLimitBackoff(attempt))
			continue
		}

		if envelope.Msg == "" {
			envelope.Msg = "unknown error"
		}
		result.Error = fmt.Errorf("ecowitt error %d: %s", envelope.Code, envelope.Msg)
		return body, result, envelope, result.Error
	}

	return nil, &FetchResult{}, nil, fmt.Errorf("exhausted Ecowitt API retries")
}

func decodeAPIEnvelope(body []byte) (*apiEnvelope, error) {
	var envelope apiEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}
	return &envelope, nil
}

func (c *Client) waitForTurn() {
	if c.minRequestInterval <= 0 {
		return
	}

	c.requestMu.Lock()
	defer c.requestMu.Unlock()

	if !c.lastRequestAt.IsZero() {
		if wait := time.Until(c.lastRequestAt.Add(c.minRequestInterval)); wait > 0 {
			time.Sleep(wait)
		}
	}

	c.lastRequestAt = time.Now()
}

func (c *Client) rateLimitBackoff(attempt int) time.Duration {
	if c.rateLimitRetryDelay <= 0 {
		return 5 * time.Second
	}
	return time.Duration(attempt+1) * c.rateLimitRetryDelay
}

func isRateLimitedAPIError(code int, msg string) bool {
	return code == -1 && strings.Contains(strings.ToLower(msg), "upper limit")
}

func extractAirQualityReading(data map[string]airQualityBlock) (*AirQualityReading, error) {
	keys := make([]string, 0, len(data))
	for key, block := range data {
		if (strings.HasPrefix(key, "ch_pm25_aqi") || strings.HasPrefix(key, "pm25_ch")) && (block.PM25 != nil || block.RealTimeAQI != nil || block.PM25Avg24H != nil || block.AQI24H != nil) {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("no Ecowitt PM2.5 data in response")
	}
	sort.Strings(keys)

	block := data[keys[0]]
	if block.PM25 == nil {
		return nil, fmt.Errorf("ecowitt PM2.5 data missing pm25 field")
	}
	if block.RealTimeAQI == nil {
		return nil, fmt.Errorf("ecowitt PM2.5 data missing real_time_aqi field")
	}

	pm25, err := parseFloat(block.PM25.Value)
	if err != nil {
		return nil, fmt.Errorf("parse pm25: %w", err)
	}
	aqiValue, err := parseFloat(block.RealTimeAQI.Value)
	if err != nil {
		return nil, fmt.Errorf("parse real_time_aqi: %w", err)
	}
	observedAt, err := parseUnixTime(block.PM25.Time)
	if err != nil {
		return nil, fmt.Errorf("parse pm25 time: %w", err)
	}

	reading := &AirQualityReading{
		ObservedAt:     observedAt,
		PM25:           pm25,
		RealTimeAQI:    int(aqiValue + 0.5),
		HasRealTimeAQI: true,
		SourceFieldKey: keys[0],
	}

	if block.AQI24H != nil {
		aqi24H, err := parseFloat(block.AQI24H.Value)
		if err != nil {
			return nil, fmt.Errorf("parse 24_hours_aqi: %w", err)
		}
		reading.AQI24H = aqi24H
		reading.HasAQI24H = true
	}

	if block.PM25Avg24H != nil {
		pm25Avg24H, err := parseFloat(block.PM25Avg24H.Value)
		if err != nil {
			return nil, fmt.Errorf("parse pm25_avg_24h: %w", err)
		}
		reading.PM25Avg24H = pm25Avg24H
		reading.HasPM25Avg24H = true
	}

	reading.Category, reading.CategoryClass = ClassifyAQI(reading.RealTimeAQI)
	return reading, nil
}

func extractAirQualityHistory(data map[string]map[string]historyMeasurement) ([]AirQualityReading, error) {
	keys := make([]string, 0, len(data))
	for key, block := range data {
		if (strings.HasPrefix(key, "ch_pm25_aqi") || strings.HasPrefix(key, "pm25_ch")) && len(block) > 0 {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("no Ecowitt PM2.5 history in response")
	}
	sort.Strings(keys)

	block := data[keys[0]]
	pm25Series, ok := block["pm25"]
	if !ok || len(pm25Series.List) == 0 {
		return nil, fmt.Errorf("ecowitt PM2.5 history missing pm25 series")
	}

	readingsByTime := make(map[time.Time]*AirQualityReading, len(pm25Series.List))
	for observedAtRaw, valueRaw := range pm25Series.List {
		observedAt, err := parseUnixTime(observedAtRaw)
		if err != nil {
			return nil, fmt.Errorf("parse pm25 history time: %w", err)
		}
		pm25, err := parseFloat(valueRaw)
		if err != nil {
			return nil, fmt.Errorf("parse pm25 history value: %w", err)
		}
		readingsByTime[observedAt] = &AirQualityReading{
			ObservedAt:     observedAt,
			PM25:           pm25,
			SourceFieldKey: keys[0],
		}
	}

	applySeries := func(name string, list map[string]string, apply func(*AirQualityReading, float64)) error {
		for observedAtRaw, valueRaw := range list {
			observedAt, err := parseUnixTime(observedAtRaw)
			if err != nil {
				return fmt.Errorf("parse %s history time: %w", name, err)
			}
			reading, ok := readingsByTime[observedAt]
			if !ok {
				continue
			}
			value, err := parseFloat(valueRaw)
			if err != nil {
				return fmt.Errorf("parse %s history value: %w", name, err)
			}
			apply(reading, value)
		}
		return nil
	}

	if series, ok := block["real_time_aqi"]; ok {
		if err := applySeries("real_time_aqi", series.List, func(reading *AirQualityReading, value float64) {
			reading.RealTimeAQI = int(value + 0.5)
			reading.HasRealTimeAQI = true
		}); err != nil {
			return nil, err
		}
	}
	if series, ok := block["24_hours_aqi"]; ok {
		if err := applySeries("24_hours_aqi", series.List, func(reading *AirQualityReading, value float64) {
			reading.AQI24H = value
			reading.HasAQI24H = true
		}); err != nil {
			return nil, err
		}
	}
	if series, ok := block["pm25_avg_24h"]; ok {
		if err := applySeries("pm25_avg_24h", series.List, func(reading *AirQualityReading, value float64) {
			reading.PM25Avg24H = value
			reading.HasPM25Avg24H = true
		}); err != nil {
			return nil, err
		}
	}
	if series, ok := block["avg_24h"]; ok {
		if err := applySeries("avg_24h", series.List, func(reading *AirQualityReading, value float64) {
			reading.PM25Avg24H = value
			reading.HasPM25Avg24H = true
		}); err != nil {
			return nil, err
		}
	}

	timestamps := make([]time.Time, 0, len(readingsByTime))
	for observedAt, reading := range readingsByTime {
		if reading.HasRealTimeAQI {
			reading.Category, reading.CategoryClass = ClassifyAQI(reading.RealTimeAQI)
		}
		timestamps = append(timestamps, observedAt)
	}
	sort.Slice(timestamps, func(i, j int) bool {
		return timestamps[i].Before(timestamps[j])
	})

	readings := make([]AirQualityReading, 0, len(timestamps))
	for _, observedAt := range timestamps {
		readings = append(readings, *readingsByTime[observedAt])
	}
	return readings, nil
}

func parseFloat(value string) (float64, error) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func parseUnixTime(value string) (time.Time, error) {
	seconds, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(seconds, 0).UTC(), nil
}

// ClassifyAQI returns the display label and CSS class for an AQI value.
func ClassifyAQI(aqi int) (string, string) {
	switch {
	case aqi <= 50:
		return "Good", "good"
	case aqi <= 100:
		return "Moderate", "moderate"
	case aqi <= 150:
		return "Poor", "poor"
	case aqi <= 200:
		return "Unhealthy", "unhealthy"
	case aqi <= 300:
		return "Severe", "severe"
	default:
		return "Hazardous", "hazardous"
	}
}
