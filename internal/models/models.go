package models

import (
	"database/sql"
	"time"
)

type Station struct {
	StationID     string
	Name          string
	Latitude      float64
	Longitude     float64
	Elevation     float64
	ElevationTier string // "valley_floor", "mid_slope", "upper"
	IsPrimary     bool
	Active        bool
}

type Observation struct {
	ID             int64
	StationID      string
	ObservedAt     time.Time
	Temp           sql.NullFloat64
	Humidity       sql.NullInt64
	Dewpoint       sql.NullFloat64
	Pressure       sql.NullFloat64
	WindSpeed      sql.NullFloat64
	WindGust       sql.NullFloat64
	WindDir        sql.NullInt64
	PrecipRate     sql.NullFloat64
	PrecipTotal    sql.NullFloat64
	SolarRadiation sql.NullFloat64
	UV             sql.NullFloat64
	HeatIndex      sql.NullFloat64
	WindChill      sql.NullFloat64
	QCStatus       int
	RawJSON        string
	CreatedAt      time.Time

	// Observation type for ML training data quality
	// Values: "instant", "hourly_aggregate", "daily_aggregate", "unknown"
	ObsType           string
	AggregationPeriod sql.NullInt64 // Minutes: NULL for instant, 60 for hourly, 1440 for daily

	// Quality flags for ML data filtering (Phase 6)
	// JSON array of flag strings: ["temp_out_of_range", "humidity_invalid", etc.]
	QualityFlags sql.NullString
}

// Observation type constants
const (
	ObsTypeInstant         = "instant"
	ObsTypeHourlyAggregate = "hourly_aggregate"
	ObsTypeDailyAggregate  = "daily_aggregate"
	ObsTypeUnknown         = "unknown"
)

type Forecast struct {
	ID            int64
	Source        string // "wu" or "bom"
	FetchedAt     time.Time
	ValidDate     time.Time
	DayOfForecast int
	TempMax       sql.NullFloat64
	TempMin       sql.NullFloat64
	Humidity      sql.NullInt64
	PrecipChance  sql.NullInt64
	PrecipAmount  sql.NullFloat64
	PrecipRange   sql.NullString // BOM format: "1 to 5 mm"
	WindSpeed     sql.NullFloat64
	WindDir       sql.NullString
	Narrative     sql.NullString
	RawJSON       string
	LocationID    sql.NullString // Geocode (WU) or AAC code (BOM)
}

type DailySummary struct {
	Date              time.Time
	StationID         string
	TempMax           sql.NullFloat64
	TempMaxTime       sql.NullTime
	TempMin           sql.NullFloat64
	TempMinTime       sql.NullTime
	TempAvg           sql.NullFloat64
	HumidityAvg       sql.NullFloat64
	PressureAvg       sql.NullFloat64
	PrecipTotal       sql.NullFloat64
	WindMaxGust       sql.NullFloat64
	InversionDetected sql.NullBool
	InversionStrength sql.NullFloat64
	RegimeHeatwave    sql.NullBool
	RegimeInversion   sql.NullBool
	RegimeClearCalm   sql.NullBool

	// Extended features for regime classification
	WindMeanNight               sql.NullFloat64
	WindMeanEvening             sql.NullFloat64
	WindMeanAfternoon           sql.NullFloat64
	CalmFractionNight           sql.NullFloat64
	SolarIntegral               sql.NullFloat64
	SolarMax                    sql.NullFloat64
	SolarMiddayAvg              sql.NullFloat64
	DewpointMin                 sql.NullFloat64
	DewpointAvg                 sql.NullFloat64
	DewpointDepressionAfternoon sql.NullFloat64
	PressureChange24h           sql.NullFloat64
	TempRise9to12               sql.NullFloat64
	DiurnalRange                sql.NullFloat64
	MiddayGradient              sql.NullFloat64
}

type ForecastVerification struct {
	ID                int64
	ForecastID        int64
	ValidDate         time.Time
	ForecastTempMax   sql.NullFloat64
	ForecastTempMin   sql.NullFloat64
	ActualTempMax     sql.NullFloat64
	ActualTempMin     sql.NullFloat64
	BiasTempMax       sql.NullFloat64
	BiasTempMin       sql.NullFloat64
	ForecastWindSpeed sql.NullFloat64
	ActualWindGust    sql.NullFloat64
	BiasWind          sql.NullFloat64
	ForecastPrecip    sql.NullFloat64
	ActualPrecip      sql.NullFloat64
	BiasPrecip        sql.NullFloat64
	CreatedAt         time.Time
}

type VerificationStats struct {
	Count        int
	AvgMaxBias   sql.NullFloat64
	AvgMinBias   sql.NullFloat64
	MAEMax       sql.NullFloat64
	MAEMin       sql.NullFloat64
	AvgWindBias  sql.NullFloat64
	MAEWind      sql.NullFloat64
	AvgPrecipBias sql.NullFloat64
	MAEPrecip    sql.NullFloat64
}

type DisplayedForecast struct {
	ID               int64
	DisplayedAt      time.Time
	ValidDate        time.Time
	DayOfForecast    int
	WUForecastID     sql.NullInt64
	BOMForecastID    sql.NullInt64
	RawTempMax       sql.NullFloat64
	RawTempMin       sql.NullFloat64
	CorrectedTempMax sql.NullFloat64
	CorrectedTempMin sql.NullFloat64
	BiasAppliedMax   sql.NullFloat64
	BiasAppliedMin   sql.NullFloat64
	BiasDayUsedMax   sql.NullInt64
	BiasDayUsedMin   sql.NullInt64
	BiasSamplesMax   sql.NullInt64
	BiasSamplesMin   sql.NullInt64
	BiasFallbackMax  sql.NullBool
	BiasFallbackMin  sql.NullBool
	SourceMax        sql.NullString
	SourceMin        sql.NullString
}
