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
}

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
	WindSpeed     sql.NullFloat64
	WindDir       sql.NullString
	Narrative     sql.NullString
	RawJSON       string
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
}

type ForecastVerification struct {
	ID              int64
	ForecastID      int64
	ValidDate       time.Time
	ForecastTempMax sql.NullFloat64
	ForecastTempMin sql.NullFloat64
	ActualTempMax   sql.NullFloat64
	ActualTempMin   sql.NullFloat64
	BiasTempMax     sql.NullFloat64
	BiasTempMin     sql.NullFloat64
	CreatedAt       time.Time
}

type VerificationStats struct {
	Count      int
	AvgMaxBias sql.NullFloat64
	AvgMinBias sql.NullFloat64
	MAEMax     sql.NullFloat64
	MAEMin     sql.NullFloat64
}
