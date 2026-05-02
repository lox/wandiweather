package ecowitt

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCurrentAirQualityParsesWH41Response(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("application_key"); got != "app-key" {
			t.Fatalf("application_key = %q, want app-key", got)
		}
		if got := r.URL.Query().Get("api_key"); got != "api-key" {
			t.Fatalf("api_key = %q, want api-key", got)
		}
		if got := r.URL.Query().Get("mac"); got != "AA:BB:CC:DD:EE:FF" {
			t.Fatalf("mac = %q, want AA:BB:CC:DD:EE:FF", got)
		}
		if got := r.URL.Query().Get("call_back"); got != "all" {
			t.Fatalf("call_back = %q, want all", got)
		}

		fmt.Fprint(w, `{
			"code": 0,
			"msg": "ok",
			"data": {
				"indoor": {
					"temp": {"time": "1712476800", "unit": "F", "value": "72.1"}
				},
				"pm25_ch1": {
					"pm25": {"time": "1712476800", "unit": "ug/m3", "value": "2.8"},
					"real_time_aqi": {"time": "1712476800", "unit": "", "value": "12"},
					"24_hours_aqi": {"time": "1712476800", "unit": "", "value": "47.0"}
				}
			}
		}`)
	}))
	defer server.Close()

	client := &Client{
		appKey:   "app-key",
		apiKey:   "api-key",
		mac:      "AA:BB:CC:DD:EE:FF",
		baseURL:  server.URL,
		client:   server.Client(),
		cacheTTL: time.Minute,
	}

	reading, err := client.CurrentAirQuality()
	if err != nil {
		t.Fatalf("CurrentAirQuality: %v", err)
	}
	if reading == nil {
		t.Fatal("CurrentAirQuality returned nil")
	}
	if reading.PM25 != 2.8 {
		t.Fatalf("PM25 = %.1f, want 2.8", reading.PM25)
	}
	if reading.RealTimeAQI != 12 {
		t.Fatalf("RealTimeAQI = %d, want 12", reading.RealTimeAQI)
	}
	if !reading.HasRealTimeAQI {
		t.Fatal("expected HasRealTimeAQI to be true")
	}
	if !reading.HasAQI24H || reading.AQI24H != 47.0 {
		t.Fatalf("AQI24H = %.1f (valid=%t), want 47.0 true", reading.AQI24H, reading.HasAQI24H)
	}
	if reading.Category != "Good" {
		t.Fatalf("Category = %q, want Good", reading.Category)
	}
	if reading.CategoryClass != "good" {
		t.Fatalf("CategoryClass = %q, want good", reading.CategoryClass)
	}
	if got := reading.ObservedAt.UTC(); !got.Equal(time.Unix(1712476800, 0).UTC()) {
		t.Fatalf("ObservedAt = %v, want %v", got, time.Unix(1712476800, 0).UTC())
	}
	if reading.SourceFieldKey != "pm25_ch1" {
		t.Fatalf("SourceFieldKey = %q, want pm25_ch1", reading.SourceFieldKey)
	}
}

func TestCurrentAirQuality_AllowsPM25WithoutAQI(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{
			"code": 0,
			"msg": "ok",
			"data": {
				"pm25_ch1": {
					"pm25": {"time": "1712476800", "unit": "ug/m3", "value": "9.4"},
					"pm25_avg_24h": {"time": "1712476800", "unit": "ug/m3", "value": "11.2"}
				}
			}
		}`)
	}))
	defer server.Close()

	client := &Client{
		appKey:   "app-key",
		apiKey:   "api-key",
		mac:      "AA:BB:CC:DD:EE:FF",
		baseURL:  server.URL,
		client:   server.Client(),
		cacheTTL: time.Minute,
	}

	reading, err := client.CurrentAirQuality()
	if err != nil {
		t.Fatalf("CurrentAirQuality: %v", err)
	}
	if reading == nil {
		t.Fatal("CurrentAirQuality returned nil")
	}
	if reading.PM25 != 9.4 {
		t.Fatalf("PM25 = %.1f, want 9.4", reading.PM25)
	}
	if reading.HasRealTimeAQI {
		t.Fatal("expected HasRealTimeAQI to be false")
	}
	if reading.RealTimeAQI != 0 {
		t.Fatalf("RealTimeAQI = %d, want 0", reading.RealTimeAQI)
	}
	if !reading.HasPM25Avg24H || reading.PM25Avg24H != 11.2 {
		t.Fatalf("PM25Avg24H = %.1f (valid=%t), want 11.2 true", reading.PM25Avg24H, reading.HasPM25Avg24H)
	}
	if reading.Category != "" || reading.CategoryClass != "" {
		t.Fatalf("unexpected AQI classification %q/%q", reading.Category, reading.CategoryClass)
	}
}

func TestCurrentAirQualityUsesCache(t *testing.T) {
	t.Parallel()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		fmt.Fprint(w, `{
			"code": 0,
			"msg": "ok",
			"data": {
				"pm25_ch1": {
					"pm25": {"time": "1712476800", "unit": "ug/m3", "value": "3.1"},
					"real_time_aqi": {"time": "1712476800", "unit": "", "value": "14"},
					"24_hours_aqi": {"time": "1712476800", "unit": "", "value": "18.0"}
				}
			}
		}`)
	}))
	defer server.Close()

	client := &Client{
		appKey:   "app-key",
		apiKey:   "api-key",
		mac:      "AA:BB:CC:DD:EE:FF",
		baseURL:  server.URL,
		client:   server.Client(),
		cacheTTL: time.Minute,
	}

	first, err := client.CurrentAirQuality()
	if err != nil {
		t.Fatalf("first CurrentAirQuality: %v", err)
	}
	second, err := client.CurrentAirQuality()
	if err != nil {
		t.Fatalf("second CurrentAirQuality: %v", err)
	}
	if requestCount != 1 {
		t.Fatalf("requestCount = %d, want 1", requestCount)
	}
	if first.PM25 != second.PM25 || first.RealTimeAQI != second.RealTimeAQI {
		t.Fatalf("cached reading mismatch: first=%+v second=%+v", first, second)
	}
}

func TestFetchAirQualityHistoryParsesWH41Response(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("call_back"); got != historyAirQualityCallbacks {
			t.Fatalf("call_back = %q, want %q", got, historyAirQualityCallbacks)
		}
		if got := r.URL.Query().Get("cycle_type"); got != HistoryCycle5Min {
			t.Fatalf("cycle_type = %q, want %q", got, HistoryCycle5Min)
		}

		fmt.Fprint(w, `{
			"code": 0,
			"msg": "success",
			"data": {
				"pm25_ch2": {
					"pm25": {
						"unit": "ug/m3",
						"list": {
							"1712476800": "2.8",
							"1712477100": "3.1"
						}
					}
				}
			}
		}`)
	}))
	defer server.Close()

	client := &Client{
		appKey:   "app-key",
		apiKey:   "api-key",
		mac:      "AA:BB:CC:DD:EE:FF",
		baseURL:  server.URL,
		client:   server.Client(),
		cacheTTL: time.Minute,
	}

	start := time.Unix(1712476800, 0).UTC()
	end := start.Add(10 * time.Minute)
	readings, rawBody, result, err := client.FetchAirQualityHistory(start, end, HistoryCycle5Min)
	if err != nil {
		t.Fatalf("FetchAirQualityHistory: %v", err)
	}
	if result == nil {
		t.Fatal("FetchAirQualityHistory returned nil result")
	}
	if result.RecordCount != 2 {
		t.Fatalf("RecordCount = %d, want 2", result.RecordCount)
	}
	if rawBody == "" {
		t.Fatal("expected raw body to be returned")
	}
	if len(readings) != 2 {
		t.Fatalf("len(readings) = %d, want 2", len(readings))
	}
	if got := readings[0].ObservedAt.UTC(); !got.Equal(start) {
		t.Fatalf("first ObservedAt = %v, want %v", got, start)
	}
	if readings[0].PM25 != 2.8 {
		t.Fatalf("first PM25 = %.1f, want 2.8", readings[0].PM25)
	}
	if readings[0].HasRealTimeAQI {
		t.Fatal("expected historical reading to omit real-time AQI")
	}
	if readings[0].SourceFieldKey != "pm25_ch2" {
		t.Fatalf("SourceFieldKey = %q, want pm25_ch2", readings[0].SourceFieldKey)
	}
	if got := readings[1].ObservedAt.UTC(); !got.Equal(start.Add(5 * time.Minute)) {
		t.Fatalf("second ObservedAt = %v, want %v", got, start.Add(5*time.Minute))
	}
	if readings[1].PM25 != 3.1 {
		t.Fatalf("second PM25 = %.1f, want 3.1", readings[1].PM25)
	}
}

func TestFetchAirQualityHistoryReturnsEcowittAPIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{
			"code": -1,
			"msg": "The number of interface accesses reached the upper limit",
			"data": []
		}`)
	}))
	defer server.Close()

	client := &Client{
		appKey:              "app-key",
		apiKey:              "api-key",
		mac:                 "AA:BB:CC:DD:EE:FF",
		baseURL:             server.URL,
		client:              server.Client(),
		cacheTTL:            time.Minute,
		minRequestInterval:  time.Millisecond,
		rateLimitRetryCount: 1,
	}

	start := time.Unix(1712476800, 0).UTC()
	end := start.Add(10 * time.Minute)
	_, _, _, err := client.FetchAirQualityHistory(start, end, HistoryCycle5Min)
	if err == nil {
		t.Fatal("FetchAirQualityHistory returned nil error")
	}
	if got := err.Error(); got != "ecowitt error -1: The number of interface accesses reached the upper limit" {
		t.Fatalf("error = %q, want rate limit error", got)
	}
}

func TestFetchAirQualityHistoryRetriesRateLimit(t *testing.T) {
	t.Parallel()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount < 3 {
			fmt.Fprint(w, `{
				"code": -1,
				"msg": "The number of interface accesses reached the upper limit",
				"data": []
			}`)
			return
		}

		fmt.Fprint(w, `{
			"code": 0,
			"msg": "success",
			"data": {
				"pm25_ch1": {
					"pm25": {
						"unit": "ug/m3",
						"list": {
							"1712476800": "2.8"
						}
					}
				}
			}
		}`)
	}))
	defer server.Close()

	client := &Client{
		appKey:              "app-key",
		apiKey:              "api-key",
		mac:                 "AA:BB:CC:DD:EE:FF",
		baseURL:             server.URL,
		client:              server.Client(),
		cacheTTL:            time.Minute,
		minRequestInterval:  time.Millisecond,
		rateLimitRetryDelay: 5 * time.Millisecond,
		rateLimitRetryCount: 3,
	}

	start := time.Unix(1712476800, 0).UTC()
	end := start.Add(10 * time.Minute)
	readings, _, result, err := client.FetchAirQualityHistory(start, end, HistoryCycle5Min)
	if err != nil {
		t.Fatalf("FetchAirQualityHistory: %v", err)
	}
	if result == nil || result.RecordCount != 1 {
		t.Fatalf("RecordCount = %v, want 1", result)
	}
	if len(readings) != 1 || readings[0].PM25 != 2.8 {
		t.Fatalf("unexpected readings: %+v", readings)
	}
	if requestCount != 3 {
		t.Fatalf("requestCount = %d, want 3", requestCount)
	}
}

func TestClientThrottlesRequests(t *testing.T) {
	t.Parallel()

	requestTimes := make([]time.Time, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestTimes = append(requestTimes, time.Now())
		fmt.Fprint(w, `{
			"code": 0,
			"msg": "ok",
			"data": {
				"pm25_ch1": {
					"pm25": {"time": "1712476800", "unit": "ug/m3", "value": "3.1"},
					"real_time_aqi": {"time": "1712476800", "unit": "", "value": "14"}
				}
			}
		}`)
	}))
	defer server.Close()

	client := &Client{
		appKey:             "app-key",
		apiKey:             "api-key",
		mac:                "AA:BB:CC:DD:EE:FF",
		baseURL:            server.URL,
		client:             server.Client(),
		cacheTTL:           time.Minute,
		minRequestInterval: 50 * time.Millisecond,
	}

	if _, _, _, err := client.FetchCurrentAirQuality(); err != nil {
		t.Fatalf("first FetchCurrentAirQuality: %v", err)
	}
	if _, _, _, err := client.FetchCurrentAirQuality(); err != nil {
		t.Fatalf("second FetchCurrentAirQuality: %v", err)
	}
	if len(requestTimes) != 2 {
		t.Fatalf("len(requestTimes) = %d, want 2", len(requestTimes))
	}
	if got := requestTimes[1].Sub(requestTimes[0]); got < 45*time.Millisecond {
		t.Fatalf("request spacing = %v, want at least 45ms", got)
	}
}
