package ingest

import (
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/lox/wandiweather/internal/models"
)

func TestValidateObservation(t *testing.T) {
	tests := []struct {
		name      string
		obs       *models.Observation
		wantFlags []string
	}{
		{
			name: "valid observation - no flags",
			obs: &models.Observation{
				Temp:           sql.NullFloat64{Float64: 25.0, Valid: true},
				Humidity:       sql.NullInt64{Int64: 60, Valid: true},
				WindDir:        sql.NullInt64{Int64: 180, Valid: true},
				WindSpeed:      sql.NullFloat64{Float64: 15.0, Valid: true},
				Pressure:       sql.NullFloat64{Float64: 1013.0, Valid: true},
				SolarRadiation: sql.NullFloat64{Float64: 500.0, Valid: true},
				PrecipRate:     sql.NullFloat64{Float64: 0.0, Valid: true},
				PrecipTotal:    sql.NullFloat64{Float64: 5.0, Valid: true},
			},
			wantFlags: nil,
		},
		{
			name: "temp too cold",
			obs: &models.Observation{
				Temp: sql.NullFloat64{Float64: -15.0, Valid: true},
			},
			wantFlags: []string{FlagTempOutOfRange},
		},
		{
			name: "temp too hot",
			obs: &models.Observation{
				Temp: sql.NullFloat64{Float64: 55.0, Valid: true},
			},
			wantFlags: []string{FlagTempOutOfRange},
		},
		{
			name: "temp at cold boundary - valid",
			obs: &models.Observation{
				Temp: sql.NullFloat64{Float64: -10.0, Valid: true},
			},
			wantFlags: nil,
		},
		{
			name: "temp at hot boundary - valid",
			obs: &models.Observation{
				Temp: sql.NullFloat64{Float64: 50.0, Valid: true},
			},
			wantFlags: nil,
		},
		{
			name: "humidity negative",
			obs: &models.Observation{
				Humidity: sql.NullInt64{Int64: -5, Valid: true},
			},
			wantFlags: []string{FlagHumidityInvalid},
		},
		{
			name: "humidity over 100",
			obs: &models.Observation{
				Humidity: sql.NullInt64{Int64: 105, Valid: true},
			},
			wantFlags: []string{FlagHumidityInvalid},
		},
		{
			name: "wind direction negative",
			obs: &models.Observation{
				WindDir: sql.NullInt64{Int64: -10, Valid: true},
			},
			wantFlags: []string{FlagWindDirInvalid},
		},
		{
			name: "wind direction over 360",
			obs: &models.Observation{
				WindDir: sql.NullInt64{Int64: 400, Valid: true},
			},
			wantFlags: []string{FlagWindDirInvalid},
		},
		{
			name: "wind speed negative",
			obs: &models.Observation{
				WindSpeed: sql.NullFloat64{Float64: -5.0, Valid: true},
			},
			wantFlags: []string{FlagWindSpeedUnlikely},
		},
		{
			name: "wind speed over 200",
			obs: &models.Observation{
				WindSpeed: sql.NullFloat64{Float64: 250.0, Valid: true},
			},
			wantFlags: []string{FlagWindSpeedUnlikely},
		},
		{
			name: "pressure too low",
			obs: &models.Observation{
				Pressure: sql.NullFloat64{Float64: 850.0, Valid: true},
			},
			wantFlags: []string{FlagPressureOutOfRange},
		},
		{
			name: "pressure too high",
			obs: &models.Observation{
				Pressure: sql.NullFloat64{Float64: 1150.0, Valid: true},
			},
			wantFlags: []string{FlagPressureOutOfRange},
		},
		{
			name: "solar radiation negative",
			obs: &models.Observation{
				SolarRadiation: sql.NullFloat64{Float64: -10.0, Valid: true},
			},
			wantFlags: []string{FlagSolarNegative},
		},
		{
			name: "precip rate negative",
			obs: &models.Observation{
				PrecipRate: sql.NullFloat64{Float64: -1.0, Valid: true},
			},
			wantFlags: []string{FlagPrecipNegative},
		},
		{
			name: "precip total negative",
			obs: &models.Observation{
				PrecipTotal: sql.NullFloat64{Float64: -2.0, Valid: true},
			},
			wantFlags: []string{FlagPrecipNegative},
		},
		{
			name: "multiple flags - temp and humidity",
			obs: &models.Observation{
				Temp:     sql.NullFloat64{Float64: 60.0, Valid: true},
				Humidity: sql.NullInt64{Int64: 150, Valid: true},
			},
			wantFlags: []string{FlagTempOutOfRange, FlagHumidityInvalid},
		},
		{
			name: "null fields - no flags",
			obs: &models.Observation{
				Temp:     sql.NullFloat64{Valid: false},
				Humidity: sql.NullInt64{Valid: false},
			},
			wantFlags: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateObservation(tt.obs)
			sort.Strings(got)
			want := append([]string(nil), tt.wantFlags...)
			sort.Strings(want)
			if len(got) != len(want) {
				t.Errorf("ValidateObservation() = %v, want %v", got, want)
				return
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("ValidateObservation() = %v, want %v", got, want)
					return
				}
			}
		})
	}
}

func TestQualityFlagsToJSON(t *testing.T) {
	tests := []struct {
		name      string
		flags     []string
		wantEmpty bool
		wantFlags []string
	}{
		{
			name:      "empty flags",
			flags:     []string{},
			wantEmpty: true,
		},
		{
			name:      "nil flags",
			flags:     nil,
			wantEmpty: true,
		},
		{
			name:      "single flag",
			flags:     []string{FlagTempOutOfRange},
			wantFlags: []string{FlagTempOutOfRange},
		},
		{
			name:      "multiple flags",
			flags:     []string{FlagTempOutOfRange, FlagHumidityInvalid},
			wantFlags: []string{FlagTempOutOfRange, FlagHumidityInvalid},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := QualityFlagsToJSON(tt.flags)
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("QualityFlagsToJSON() = %q, want empty", got)
				}
				return
			}
			var parsed []string
			if err := json.Unmarshal([]byte(got), &parsed); err != nil {
				t.Fatalf("failed to unmarshal result: %v", err)
			}
			sort.Strings(parsed)
			want := append([]string(nil), tt.wantFlags...)
			sort.Strings(want)
			if len(parsed) != len(want) {
				t.Errorf("QualityFlagsToJSON() parsed = %v, want %v", parsed, want)
				return
			}
			for i := range want {
				if parsed[i] != want[i] {
					t.Errorf("QualityFlagsToJSON() parsed = %v, want %v", parsed, want)
					return
				}
			}
		})
	}
}

func TestParseCurrentResponse(t *testing.T) {
	tests := []struct {
		name       string
		jsonData   string
		wantLen    int
		wantErr    bool
		checkFirst func(t *testing.T, obs CurrentObservation)
	}{
		{
			name: "valid response with all fields",
			jsonData: `{
				"observations": [{
					"stationID": "IWANDI23",
					"obsTimeUtc": "2025-01-15T10:00:00Z",
					"obsTimeLocal": "2025-01-15T21:00:00+1100",
					"humidity": 65,
					"uv": 3.5,
					"winddir": 180,
					"solarRadiation": 450.5,
					"qcStatus": 1,
					"metric": {
						"temp": 25.5,
						"heatIndex": 26.0,
						"dewpt": 18.0,
						"windSpeed": 12.5,
						"windGust": 18.0,
						"pressure": 1015.2,
						"precipRate": 0.0,
						"precipTotal": 0.0
					}
				}]
			}`,
			wantLen: 1,
			wantErr: false,
			checkFirst: func(t *testing.T, obs CurrentObservation) {
				if obs.StationID != "IWANDI23" {
					t.Errorf("StationID = %q, want IWANDI23", obs.StationID)
				}
				if obs.Humidity == nil || *obs.Humidity != 65 {
					t.Errorf("Humidity = %v, want 65", obs.Humidity)
				}
				if obs.Metric == nil || obs.Metric.Temp == nil || *obs.Metric.Temp != 25.5 {
					t.Error("Metric.Temp not parsed correctly")
				}
			},
		},
		{
			name: "response with null fields",
			jsonData: `{
				"observations": [{
					"stationID": "IWANDI23",
					"obsTimeUtc": "2025-01-15T10:00:00Z",
					"humidity": null,
					"uv": null,
					"metric": {
						"temp": 20.0,
						"windSpeed": null
					}
				}]
			}`,
			wantLen: 1,
			wantErr: false,
			checkFirst: func(t *testing.T, obs CurrentObservation) {
				if obs.Humidity != nil {
					t.Error("Humidity should be nil")
				}
				if obs.Metric == nil || obs.Metric.Temp == nil || *obs.Metric.Temp != 20.0 {
					t.Error("Metric.Temp should be 20.0")
				}
				if obs.Metric.WindSpeed != nil {
					t.Error("Metric.WindSpeed should be nil")
				}
			},
		},
		{
			name:     "empty observations array",
			jsonData: `{"observations": []}`,
			wantLen:  0,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp CurrentResponse
			err := json.Unmarshal([]byte(tt.jsonData), &resp)
			if (err != nil) != tt.wantErr {
				t.Errorf("Unmarshal error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(resp.Observations) != tt.wantLen {
				t.Errorf("len(Observations) = %d, want %d", len(resp.Observations), tt.wantLen)
				return
			}
			if tt.checkFirst != nil && len(resp.Observations) > 0 {
				tt.checkFirst(t, resp.Observations[0])
			}
		})
	}
}

func TestParseHistoryResponse(t *testing.T) {
	jsonData := `{
		"observations": [
			{
				"stationID": "IWANDI23",
				"obsTimeUtc": "2025-01-15T00:00:00Z",
				"epoch": 1736899200,
				"humidityAvg": 70,
				"uvHigh": 5.0,
				"winddirAvg": 225,
				"metric": {
					"tempAvg": 22.5,
					"tempHigh": 25.0,
					"tempLow": 20.0,
					"windspeedAvg": 10.0,
					"windgustHigh": 20.0,
					"precipTotal": 2.5
				}
			},
			{
				"stationID": "IWANDI23",
				"obsTimeUtc": "2025-01-15T01:00:00Z",
				"epoch": 1736902800,
				"humidityAvg": 75,
				"metric": {
					"tempAvg": 21.0
				}
			}
		]
	}`

	var resp HistoryResponse
	if err := json.Unmarshal([]byte(jsonData), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if len(resp.Observations) != 2 {
		t.Fatalf("len(Observations) = %d, want 2", len(resp.Observations))
	}

	obs := resp.Observations[0]
	if obs.StationID != "IWANDI23" {
		t.Errorf("StationID = %q, want IWANDI23", obs.StationID)
	}
	if obs.Epoch != 1736899200 {
		t.Errorf("Epoch = %d, want 1736899200", obs.Epoch)
	}
	if obs.HumidityAvg == nil || *obs.HumidityAvg != 70 {
		t.Errorf("HumidityAvg = %v, want 70", obs.HumidityAvg)
	}
	if obs.Metric == nil || obs.Metric.TempAvg == nil || *obs.Metric.TempAvg != 22.5 {
		t.Error("Metric.TempAvg not parsed correctly")
	}
	if obs.Metric.PrecipTotal == nil || *obs.Metric.PrecipTotal != 2.5 {
		t.Error("Metric.PrecipTotal not parsed correctly")
	}
}

func TestParseForecastResponse(t *testing.T) {
	jsonData := `{
		"dayOfWeek": ["Monday", "Tuesday", "Wednesday"],
		"validTimeLocal": ["2025-01-20T07:00:00+1100", "2025-01-21T07:00:00+1100", "2025-01-22T07:00:00+1100"],
		"calendarDayTemperatureMax": [28.0, 30.0, 25.0],
		"calendarDayTemperatureMin": [15.0, 18.0, 14.0],
		"narrative": ["Partly cloudy", "Sunny and hot", "Showers possible"],
		"daypart": [{
			"daypartName": ["Monday", null, "Tuesday", null, "Wednesday", null],
			"precipChance": [20, 10, 10, 5, 60, 40],
			"qpf": [0.0, 0.0, 0.0, 0.0, 5.0, 2.0],
			"windSpeed": [15, 10, 20, 12, 25, 18],
			"windDirectionCardinal": ["N", "NE", "NW", "W", "S", "SE"]
		}]
	}`

	var resp ForecastResponse
	if err := json.Unmarshal([]byte(jsonData), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if len(resp.ValidTimeLocal) != 3 {
		t.Fatalf("len(ValidTimeLocal) = %d, want 3", len(resp.ValidTimeLocal))
	}
	if len(resp.CalendarDayTempMax) != 3 {
		t.Fatalf("len(CalendarDayTempMax) = %d, want 3", len(resp.CalendarDayTempMax))
	}
	if resp.CalendarDayTempMax[1] != 30.0 {
		t.Errorf("CalendarDayTempMax[1] = %v, want 30.0", resp.CalendarDayTempMax[1])
	}

	if len(resp.Daypart) != 1 {
		t.Fatalf("len(Daypart) = %d, want 1", len(resp.Daypart))
	}
	daypart := resp.Daypart[0]
	if len(daypart.PrecipChance) != 6 {
		t.Errorf("len(PrecipChance) = %d, want 6", len(daypart.PrecipChance))
	}
	if daypart.PrecipChance[4] == nil || *daypart.PrecipChance[4] != 60 {
		t.Error("PrecipChance[4] should be 60")
	}
}

func TestTruncateBody(t *testing.T) {
	t.Run("short string unchanged", func(t *testing.T) {
		input := "hello world"
		got := truncateBody([]byte(input))
		if got != input {
			t.Errorf("truncateBody() = %q, want %q", got, input)
		}
	})

	t.Run("exactly 512 chars unchanged", func(t *testing.T) {
		input := strings.Repeat("a", 512)
		got := truncateBody([]byte(input))
		if got != input {
			t.Errorf("truncateBody() len = %d, want 512", len(got))
		}
	})

	t.Run("over 512 chars truncated", func(t *testing.T) {
		input := strings.Repeat("x", 600)
		got := truncateBody([]byte(input))
		expectedPrefix := strings.Repeat("x", 512)
		expectedSuffix := "...(truncated)"
		if !strings.HasPrefix(got, expectedPrefix) {
			t.Error("truncateBody() should start with 512 'x' characters")
		}
		if !strings.HasSuffix(got, expectedSuffix) {
			t.Errorf("truncateBody() should end with %q", expectedSuffix)
		}
		if len(got) != 512+len(expectedSuffix) {
			t.Errorf("truncateBody() len = %d, want %d", len(got), 512+len(expectedSuffix))
		}
	})
}

func TestParseCurrentResponse_InvalidJSON(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
	}{
		{"malformed JSON", `{`},
		{"wrong type for observations", `{"observations": {}}`},
		{"observations as string", `{"observations": "invalid"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp CurrentResponse
			err := json.Unmarshal([]byte(tt.jsonData), &resp)
			if err == nil {
				t.Error("expected unmarshal error for invalid JSON")
			}
		})
	}
}

func TestValidateObservation_BoundaryValues(t *testing.T) {
	tests := []struct {
		name      string
		obs       *models.Observation
		wantFlags []string
	}{
		{
			name:      "humidity at 0 - valid",
			obs:       &models.Observation{Humidity: sql.NullInt64{Int64: 0, Valid: true}},
			wantFlags: nil,
		},
		{
			name:      "humidity at 100 - valid",
			obs:       &models.Observation{Humidity: sql.NullInt64{Int64: 100, Valid: true}},
			wantFlags: nil,
		},
		{
			name:      "wind dir at 0 - valid",
			obs:       &models.Observation{WindDir: sql.NullInt64{Int64: 0, Valid: true}},
			wantFlags: nil,
		},
		{
			name:      "wind dir at 360 - valid",
			obs:       &models.Observation{WindDir: sql.NullInt64{Int64: 360, Valid: true}},
			wantFlags: nil,
		},
		{
			name:      "pressure at 900 - valid",
			obs:       &models.Observation{Pressure: sql.NullFloat64{Float64: 900, Valid: true}},
			wantFlags: nil,
		},
		{
			name:      "pressure at 1100 - valid",
			obs:       &models.Observation{Pressure: sql.NullFloat64{Float64: 1100, Valid: true}},
			wantFlags: nil,
		},
		{
			name:      "pressure just below 900 - invalid",
			obs:       &models.Observation{Pressure: sql.NullFloat64{Float64: 899.9, Valid: true}},
			wantFlags: []string{FlagPressureOutOfRange},
		},
		{
			name:      "pressure just above 1100 - invalid",
			obs:       &models.Observation{Pressure: sql.NullFloat64{Float64: 1100.1, Valid: true}},
			wantFlags: []string{FlagPressureOutOfRange},
		},
		{
			name:      "wind speed at 0 - valid",
			obs:       &models.Observation{WindSpeed: sql.NullFloat64{Float64: 0, Valid: true}},
			wantFlags: nil,
		},
		{
			name:      "wind speed at 200 - valid",
			obs:       &models.Observation{WindSpeed: sql.NullFloat64{Float64: 200, Valid: true}},
			wantFlags: nil,
		},
		{
			name:      "wind speed just over 200 - invalid",
			obs:       &models.Observation{WindSpeed: sql.NullFloat64{Float64: 200.1, Valid: true}},
			wantFlags: []string{FlagWindSpeedUnlikely},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateObservation(tt.obs)
			if len(got) != len(tt.wantFlags) {
				t.Errorf("ValidateObservation() = %v, want %v", got, tt.wantFlags)
			}
		})
	}
}
