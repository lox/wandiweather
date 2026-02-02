package api

import (
	"github.com/lox/wandiweather/internal/emergency"
	"github.com/lox/wandiweather/internal/firedanger"
	"github.com/lox/wandiweather/internal/forecast"
	"github.com/lox/wandiweather/internal/models"
	"github.com/lox/wandiweather/internal/store"
	"time"
)

// CurrentData contains all the data needed to render the current conditions view.
type CurrentData struct {
	Primary        *models.Observation
	ValleyTemp     float64
	TempChangeRate *float64
	FeelsLike      *float64
	Stations       map[string]*models.Observation
	StationMeta    map[string]models.Station
	AllStations    []StationReading
	ValleyFloor    []StationReading
	MidSlope       []StationReading
	Upper          []StationReading
	Inversion      *InversionStatus
	TodayForecast  *TodayForecast
	TodayStats     *TodayStats
	LastUpdated    time.Time
	Moon           *MoonData
	Alerts         []emergency.Alert
	UrgentAlerts   []emergency.Alert
	FireDanger     *firedanger.DayForecast
}

// MoonData contains moon phase information for display.
type MoonData struct {
	Phase        string // e.g., "Waxing Gibbous"
	Illumination int    // 0-100 percentage
	Emoji        string // 🌑🌒🌓🌔🌕🌖🌗🌘
}

// IndexData wraps CurrentData with additional page-level data.
type IndexData struct {
	*CurrentData
	Palette         forecast.Palette
	WeatherOverride string // Optional override for testing (e.g., "storm_night")
}

// TodayForecast contains the processed forecast for today.
type TodayForecast struct {
	TempMax           float64
	TempMin           float64
	TempMaxRaw        float64
	NowcastApplied    bool
	NowcastAdjustment float64
	PrecipChance      int64
	PrecipAmount      float64
	Narrative         string
	HasPrecip         bool
	Explanation       ForecastExplanation
}

// ForecastExplanation tracks how the forecast was calculated.
type ForecastExplanation struct {
	MaxSource       string  // "bom" or "wu"
	MaxRaw          float64 // raw forecast value
	MaxBiasApplied  float64 // bias correction applied
	MaxBiasDayUsed  int     // which day's bias was used (-1 if none)
	MaxBiasSamples  int     // how many samples the bias is based on
	MaxBiasFallback bool    // true if fallback day was used
	MaxNowcast      float64 // nowcast adjustment (if any)
	MaxFinal        float64 // final displayed value
	MinSource       string
	MinRaw          float64
	MinBiasApplied  float64
	MinBiasDayUsed  int  // which day's bias was used (-1 if none)
	MinBiasSamples  int  // how many samples the bias is based on
	MinBiasFallback bool // true if fallback day was used
	MinFinal        float64
}

// TodayStats contains observed statistics for today.
type TodayStats struct {
	MinTemp     float64
	MaxTemp     float64
	MinTempTime string
	MaxTempTime string
	RainTotal   float64
	HasRain     bool
	MaxWind     float64
	MaxGust     float64
	HasWind     bool
}

// StationReading pairs a station with its latest observation.
type StationReading struct {
	Station models.Station
	Obs     *models.Observation
}

// InversionStatus indicates whether a temperature inversion is active.
type InversionStatus struct {
	Active    bool
	Strength  float64
	ValleyAvg float64
	MidAvg    float64
	UpperAvg  float64
}

// ForecastData contains multi-day forecast information.
type ForecastData struct {
	Days     []ForecastDay
	WUStats  *models.VerificationStats
	BOMStats *models.VerificationStats
	HasStats bool
}

// ForecastDay represents a single day's forecast.
type ForecastDay struct {
	Date               time.Time
	DayName            string
	DateStr            string
	IsToday            bool
	WU                 *models.Forecast
	BOM                *models.Forecast
	WUCorrectedMax     *float64 `json:"wu_corrected_max,omitempty"`
	WUCorrectedMin     *float64 `json:"wu_corrected_min,omitempty"`
	BOMCorrectedMax    *float64 `json:"bom_corrected_max,omitempty"`
	BOMCorrectedMin    *float64 `json:"bom_corrected_min,omitempty"`
	DisplayMax         *float64 `json:"display_max,omitempty"`
	DisplayMin         *float64 `json:"display_min,omitempty"`
	GeneratedNarrative string   `json:"generated_narrative"`
}

// ChartData contains data for the temperature chart.
type ChartData struct {
	Labels []string      `json:"labels"`
	Series []ChartSeries `json:"series"`
}

// ChartSeries represents a single series in the chart.
type ChartSeries struct {
	Name  string    `json:"name"`
	Data  []float64 `json:"data"`
	Color string    `json:"color"`
}

// AccuracyData contains forecast verification statistics.
type AccuracyData struct {
	WUStats        *models.VerificationStats
	BOMStats       *models.VerificationStats
	CorrectedStats *store.CorrectedAccuracyStats
	UniqueDays     int
	History        []VerificationRow
	ChartLabels    []string
	ChartWUMax     []float64
	ChartWUMin     []float64
	ChartBOMMax    []float64
	ChartBOMMin    []float64
	LeadTimeData   []LeadTimeRow
	RegimeStats    []RegimeRow
}

// VerificationRow represents a single verification entry.
type VerificationRow struct {
	Date         string
	Source       string
	ForecastMax  float64
	ForecastMin  float64
	ActualMax    float64
	ActualMin    float64
	BiasMax      float64
	MaxBiasClass string
	Regime       string
	RegimeBadge  string
}

// RegimeRow represents regime-based accuracy statistics.
type RegimeRow struct {
	Regime    string
	Label     string
	Badge     string
	Color     string
	WUMAEMax  float64
	WUMAEMin  float64
	BOMMAEMax float64
	BOMMAEMin float64
	WUDays    int
	BOMDays   int
}

// LeadTimeRow represents accuracy by forecast lead time.
type LeadTimeRow struct {
	LeadTime  int
	WUMAEMax  float64
	WUMAEMin  float64
	BOMMAEMax float64
	BOMMAEMin float64
	WUDays    int
	BOMDays   int
}

// DataPageData contains data health and statistics.
type DataPageData struct {
	SchemaVersion     int
	TotalObservations int64
	TotalForecasts    int64
	RawPayloadCount   int64
	RawPayloadSizeKB  int64
	DatabaseSizeMB    float64
	IngestHealth      []store.IngestHealthSummary
	ObsTypes          []store.ObsTypeCount
	ForecastCoverage  []store.ForecastCoverage
	RecentErrors      []store.RecentIngestError
	ObsWithFlags      int64
	CleanObservations int64
	ParseErrors24h    int64
	UpdatedAt         string
}

// HealthStatus represents the health check response.
type HealthStatus struct {
	Status   string          `json:"status"`
	Stations []StationHealth `json:"stations"`
	Errors   []string        `json:"errors,omitempty"`
}

// StationHealth represents the health of a single station.
type StationHealth struct {
	StationID  string    `json:"station_id"`
	LastSeen   time.Time `json:"last_seen"`
	AgeMinutes int       `json:"age_minutes"`
	Stale      bool      `json:"stale"`
}
