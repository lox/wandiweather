package ingest

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/lox/wandiweather/internal/httputil"
	"github.com/lox/wandiweather/internal/metrics"
	"github.com/lox/wandiweather/internal/models"
)

type PWS struct {
	apiKey string
	client *http.Client
}

func NewPWS(apiKey string) *PWS {
	return &PWS{
		apiKey: apiKey,
		client: httputil.NewClient(),
	}
}

func truncateBody(b []byte) string {
	s := string(b)
	if len(s) > 512 {
		return s[:512] + "...(truncated)"
	}
	return s
}

type CurrentResponse struct {
	Observations []CurrentObservation `json:"observations"`
}

type CurrentObservation struct {
	StationID      string  `json:"stationID"`
	ObsTimeUtc     string  `json:"obsTimeUtc"`
	ObsTimeLocal   string  `json:"obsTimeLocal"`
	Neighborhood   string  `json:"neighborhood"`
	Lat            float64 `json:"lat"`
	Lon            float64 `json:"lon"`
	Humidity       *int    `json:"humidity"`
	UV             *float64 `json:"uv"`
	WindDir        *int    `json:"winddir"`
	SolarRadiation *float64 `json:"solarRadiation"`
	QCStatus       int     `json:"qcStatus"`
	Metric         *struct {
		Temp        *float64 `json:"temp"`
		HeatIndex   *float64 `json:"heatIndex"`
		Dewpt       *float64 `json:"dewpt"`
		WindChill   *float64 `json:"windChill"`
		WindSpeed   *float64 `json:"windSpeed"`
		WindGust    *float64 `json:"windGust"`
		Pressure    *float64 `json:"pressure"`
		PrecipRate  *float64 `json:"precipRate"`
		PrecipTotal *float64 `json:"precipTotal"`
		Elev        *float64 `json:"elev"`
	} `json:"metric"`
}

// FetchResult contains the result of a fetch operation with metadata for auditing.
type FetchResult struct {
	HTTPStatus   int
	ResponseSize int
	RecordCount  int
	ParseErrors  int    // Number of records that failed to parse
	Error        error  // Fatal error (if any)
	ParseError   string // Description of parse errors (if any)
}

func (p *PWS) FetchCurrent(stationID string) (*models.Observation, string, *FetchResult, error) {
	url := fmt.Sprintf("https://api.weather.com/v2/pws/observations/current?stationId=%s&format=json&units=m&apiKey=%s", stationID, p.apiKey)
	start := time.Now()
	result := &FetchResult{}

	var body []byte
	var lastStatus int
	operation := func() error {
		resp, err := p.client.Get(url)
		if err != nil {
			return fmt.Errorf("fetch current: %w", err)
		}
		defer resp.Body.Close()
		lastStatus = resp.StatusCode

		if resp.StatusCode == http.StatusTooManyRequests {
			metrics.PWSAPICallsTotal.WithLabelValues(stationID, "current", "rate_limited").Inc()
			return fmt.Errorf("rate limited: status %d", resp.StatusCode)
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			metrics.PWSAPICallsTotal.WithLabelValues(stationID, "current", "auth_error").Inc()
			b, _ := io.ReadAll(resp.Body)
			return backoff.Permanent(fmt.Errorf("auth error: status %d: %s", resp.StatusCode, truncateBody(b)))
		}
		if resp.StatusCode >= 500 {
			metrics.PWSAPICallsTotal.WithLabelValues(stationID, "current", "server_error").Inc()
			b, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("server error: status %d: %s", resp.StatusCode, truncateBody(b))
		}
		if resp.StatusCode != http.StatusOK {
			metrics.PWSAPICallsTotal.WithLabelValues(stationID, "current", "client_error").Inc()
			b, _ := io.ReadAll(resp.Body)
			return backoff.Permanent(fmt.Errorf("client error: status %d: %s", resp.StatusCode, truncateBody(b)))
		}

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read body: %w", err)
		}
		return nil
	}

	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = 2 * time.Minute
	if err := backoff.Retry(operation, bo); err != nil {
		result.HTTPStatus = lastStatus
		result.Error = err
		return nil, "", result, err
	}

	result.HTTPStatus = lastStatus
	result.ResponseSize = len(body)

	metrics.PWSAPICallsTotal.WithLabelValues(stationID, "current", "success").Inc()
	metrics.PWSAPILatency.WithLabelValues(stationID, "current").Observe(time.Since(start).Seconds())

	var data CurrentResponse
	if err := json.Unmarshal(body, &data); err != nil {
		result.Error = fmt.Errorf("unmarshal: %w", err)
		return nil, string(body), result, result.Error
	}

	if len(data.Observations) == 0 {
		result.Error = fmt.Errorf("no observations returned for %s", stationID)
		return nil, string(body), result, result.Error
	}

	result.RecordCount = len(data.Observations)

	obs := data.Observations[0]
	observedAt, err := time.Parse(time.RFC3339, obs.ObsTimeUtc)
	if err != nil {
		result.Error = fmt.Errorf("parse time: %w", err)
		return nil, string(body), result, result.Error
	}

	observation := &models.Observation{
		StationID:  obs.StationID,
		ObservedAt: observedAt,
		QCStatus:   obs.QCStatus,
		ObsType:    models.ObsTypeInstant,
	}

	// First populate all fields, then validate
	if obs.Humidity != nil {
		observation.Humidity = sql.NullInt64{Int64: int64(*obs.Humidity), Valid: true}
	}
	if obs.UV != nil {
		observation.UV = sql.NullFloat64{Float64: *obs.UV, Valid: true}
	}
	if obs.WindDir != nil {
		observation.WindDir = sql.NullInt64{Int64: int64(*obs.WindDir), Valid: true}
	}
	if obs.SolarRadiation != nil {
		observation.SolarRadiation = sql.NullFloat64{Float64: *obs.SolarRadiation, Valid: true}
	}

	if obs.Metric != nil {
		if obs.Metric.Temp != nil {
			observation.Temp = sql.NullFloat64{Float64: *obs.Metric.Temp, Valid: true}
		}
		if obs.Metric.Dewpt != nil {
			observation.Dewpoint = sql.NullFloat64{Float64: *obs.Metric.Dewpt, Valid: true}
		}
		if obs.Metric.Pressure != nil {
			observation.Pressure = sql.NullFloat64{Float64: *obs.Metric.Pressure, Valid: true}
		}
		if obs.Metric.WindSpeed != nil {
			observation.WindSpeed = sql.NullFloat64{Float64: *obs.Metric.WindSpeed, Valid: true}
		}
		if obs.Metric.WindGust != nil {
			observation.WindGust = sql.NullFloat64{Float64: *obs.Metric.WindGust, Valid: true}
		}
		if obs.Metric.PrecipRate != nil {
			observation.PrecipRate = sql.NullFloat64{Float64: *obs.Metric.PrecipRate, Valid: true}
		}
		if obs.Metric.PrecipTotal != nil {
			observation.PrecipTotal = sql.NullFloat64{Float64: *obs.Metric.PrecipTotal, Valid: true}
		}
		if obs.Metric.HeatIndex != nil {
			observation.HeatIndex = sql.NullFloat64{Float64: *obs.Metric.HeatIndex, Valid: true}
		}
		if obs.Metric.WindChill != nil {
			observation.WindChill = sql.NullFloat64{Float64: *obs.Metric.WindChill, Valid: true}
		}
	}

	// Validate and set quality flags
	flags := ValidateObservation(observation)
	if len(flags) > 0 {
		observation.QualityFlags = sql.NullString{String: QualityFlagsToJSON(flags), Valid: true}
	}

	return observation, string(body), result, nil
}

type HistoryResponse struct {
	Observations []HistoryObservation `json:"observations"`
}

type HistoryObservation struct {
	StationID    string   `json:"stationID"`
	Tz           string   `json:"tz"`
	ObsTimeUtc   string   `json:"obsTimeUtc"`
	ObsTimeLocal string   `json:"obsTimeLocal"`
	Epoch        int64    `json:"epoch"`
	Lat          float64  `json:"lat"`
	Lon          float64  `json:"lon"`
	HumidityHigh *int     `json:"humidityHigh"`
	HumidityLow  *int     `json:"humidityLow"`
	HumidityAvg  *int     `json:"humidityAvg"`
	WinddirAvg   *int     `json:"winddirAvg"`
	UVHigh       *float64 `json:"uvHigh"`
	SolarRadHigh *float64 `json:"solarRadiationHigh"`
	QCStatus     int      `json:"qcStatus"`
	Metric       *struct {
		TempHigh      *float64 `json:"tempHigh"`
		TempLow       *float64 `json:"tempLow"`
		TempAvg       *float64 `json:"tempAvg"`
		DewptHigh     *float64 `json:"dewptHigh"`
		DewptLow      *float64 `json:"dewptLow"`
		DewptAvg      *float64 `json:"dewptAvg"`
		WindspeedHigh *float64 `json:"windspeedHigh"`
		WindspeedLow  *float64 `json:"windspeedLow"`
		WindspeedAvg  *float64 `json:"windspeedAvg"`
		WindgustHigh  *float64 `json:"windgustHigh"`
		WindgustLow   *float64 `json:"windgustLow"`
		WindgustAvg   *float64 `json:"windgustAvg"`
		PressureMax   *float64 `json:"pressureMax"`
		PressureMin   *float64 `json:"pressureMin"`
		PrecipRate    *float64 `json:"precipRate"`
		PrecipTotal   *float64 `json:"precipTotal"`
		HeatindexHigh *float64 `json:"heatindexHigh"`
		HeatindexLow  *float64 `json:"heatindexLow"`
		HeatindexAvg  *float64 `json:"heatindexAvg"`
		WindchillHigh *float64 `json:"windchillHigh"`
		WindchillLow  *float64 `json:"windchillLow"`
		WindchillAvg  *float64 `json:"windchillAvg"`
	} `json:"metric"`
}

func (p *PWS) FetchHistory1Day(stationID string) ([]models.Observation, error) {
	return p.fetchHistory(stationID, "all/1day")
}

func (p *PWS) FetchHistory7Day(stationID string) ([]models.Observation, error) {
	return p.fetchHistory(stationID, "hourly/7day")
}

func (p *PWS) fetchHistory(stationID, endpoint string) ([]models.Observation, error) {
	url := fmt.Sprintf("https://api.weather.com/v2/pws/observations/%s?stationId=%s&format=json&units=m&apiKey=%s", endpoint, stationID, p.apiKey)
	start := time.Now()

	var body []byte
	operation := func() error {
		resp, err := p.client.Get(url)
		if err != nil {
			return fmt.Errorf("fetch history: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			metrics.PWSAPICallsTotal.WithLabelValues(stationID, "history", "rate_limited").Inc()
			return fmt.Errorf("rate limited: status %d", resp.StatusCode)
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			metrics.PWSAPICallsTotal.WithLabelValues(stationID, "history", "auth_error").Inc()
			b, _ := io.ReadAll(resp.Body)
			return backoff.Permanent(fmt.Errorf("auth error: status %d: %s", resp.StatusCode, truncateBody(b)))
		}
		if resp.StatusCode >= 500 {
			metrics.PWSAPICallsTotal.WithLabelValues(stationID, "history", "server_error").Inc()
			b, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("server error: status %d: %s", resp.StatusCode, truncateBody(b))
		}
		if resp.StatusCode != http.StatusOK {
			metrics.PWSAPICallsTotal.WithLabelValues(stationID, "history", "client_error").Inc()
			b, _ := io.ReadAll(resp.Body)
			return backoff.Permanent(fmt.Errorf("client error: status %d: %s", resp.StatusCode, truncateBody(b)))
		}

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read body: %w", err)
		}
		return nil
	}

	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = 2 * time.Minute
	if err := backoff.Retry(operation, bo); err != nil {
		return nil, err
	}

	metrics.PWSAPICallsTotal.WithLabelValues(stationID, "history", "success").Inc()
	metrics.PWSAPILatency.WithLabelValues(stationID, "history").Observe(time.Since(start).Seconds())

	var data HistoryResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	var results []models.Observation
	for _, obs := range data.Observations {
		observedAt := time.Unix(obs.Epoch, 0).UTC()

		result := models.Observation{
			StationID:         obs.StationID,
			ObservedAt:        observedAt,
			QCStatus:          obs.QCStatus,
			ObsType:           models.ObsTypeHourlyAggregate,
			AggregationPeriod: sql.NullInt64{Int64: 60, Valid: true},
		}

		if obs.HumidityAvg != nil {
			result.Humidity = sql.NullInt64{Int64: int64(*obs.HumidityAvg), Valid: true}
		}
		if obs.UVHigh != nil {
			result.UV = sql.NullFloat64{Float64: *obs.UVHigh, Valid: true}
		}
		if obs.WinddirAvg != nil {
			result.WindDir = sql.NullInt64{Int64: int64(*obs.WinddirAvg), Valid: true}
		}
		if obs.SolarRadHigh != nil {
			result.SolarRadiation = sql.NullFloat64{Float64: *obs.SolarRadHigh, Valid: true}
		}

		if obs.Metric != nil {
			if obs.Metric.TempAvg != nil {
				result.Temp = sql.NullFloat64{Float64: *obs.Metric.TempAvg, Valid: true}
			}
			if obs.Metric.DewptAvg != nil {
				result.Dewpoint = sql.NullFloat64{Float64: *obs.Metric.DewptAvg, Valid: true}
			}
			if obs.Metric.PressureMax != nil {
				result.Pressure = sql.NullFloat64{Float64: *obs.Metric.PressureMax, Valid: true}
			}
			if obs.Metric.WindspeedAvg != nil {
				result.WindSpeed = sql.NullFloat64{Float64: *obs.Metric.WindspeedAvg, Valid: true}
			}
			if obs.Metric.WindgustHigh != nil {
				result.WindGust = sql.NullFloat64{Float64: *obs.Metric.WindgustHigh, Valid: true}
			}
			if obs.Metric.PrecipRate != nil {
				result.PrecipRate = sql.NullFloat64{Float64: *obs.Metric.PrecipRate, Valid: true}
			}
			if obs.Metric.PrecipTotal != nil {
				result.PrecipTotal = sql.NullFloat64{Float64: *obs.Metric.PrecipTotal, Valid: true}
			}
			if obs.Metric.HeatindexAvg != nil {
				result.HeatIndex = sql.NullFloat64{Float64: *obs.Metric.HeatindexAvg, Valid: true}
			}
			if obs.Metric.WindchillAvg != nil {
				result.WindChill = sql.NullFloat64{Float64: *obs.Metric.WindchillAvg, Valid: true}
			}
		}

		// Validate and set quality flags
		flags := ValidateObservation(&result)
		if len(flags) > 0 {
			result.QualityFlags = sql.NullString{String: QualityFlagsToJSON(flags), Valid: true}
		}

		results = append(results, result)
	}

	return results, nil
}
