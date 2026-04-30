package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/lox/wandiweather/internal/ecowitt"
)

// UpsertAirQualityReadings stores Ecowitt WH41 readings and enriches existing rows
// when a later live poll adds AQI fields to a timestamp that was backfilled from history.
func (s *Store) UpsertAirQualityReadings(readings []ecowitt.AirQualityReading) (int, error) {
	if len(readings) == 0 {
		return 0, nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO air_quality_readings (
			observed_at,
			pm25,
			real_time_aqi,
			aqi_24h,
			pm25_avg_24h,
			source_field_key
		)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(observed_at) DO UPDATE SET
			pm25 = excluded.pm25,
			real_time_aqi = COALESCE(excluded.real_time_aqi, air_quality_readings.real_time_aqi),
			aqi_24h = COALESCE(excluded.aqi_24h, air_quality_readings.aqi_24h),
			pm25_avg_24h = COALESCE(excluded.pm25_avg_24h, air_quality_readings.pm25_avg_24h),
			source_field_key = COALESCE(excluded.source_field_key, air_quality_readings.source_field_key)
	`)
	if err != nil {
		tx.Rollback()
		return 0, fmt.Errorf("prepare air quality upsert: %w", err)
	}
	defer stmt.Close()

	stored := 0
	for _, reading := range readings {
		if _, err := stmt.Exec(
			reading.ObservedAt.UTC(),
			reading.PM25,
			nullAQI(reading.HasRealTimeAQI, reading.RealTimeAQI),
			nullFloat(reading.HasAQI24H, reading.AQI24H),
			nullFloat(reading.HasPM25Avg24H, reading.PM25Avg24H),
			nullString(reading.SourceFieldKey),
		); err != nil {
			tx.Rollback()
			return 0, fmt.Errorf("upsert air quality reading at %s: %w", reading.ObservedAt.UTC().Format(time.RFC3339), err)
		}
		stored++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit air quality upserts: %w", err)
	}

	return stored, nil
}

// GetLatestAirQualityReading returns the most recent stored WH41 reading.
func (s *Store) GetLatestAirQualityReading() (*ecowitt.AirQualityReading, error) {
	row := s.db.QueryRow(`
		SELECT observed_at, pm25, real_time_aqi, aqi_24h, pm25_avg_24h, source_field_key
		FROM air_quality_readings
		ORDER BY observed_at DESC
		LIMIT 1
	`)

	reading, err := scanAirQualityReading(row.Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return reading, nil
}

// GetAirQualityReadings returns stored WH41 readings for the given time range.
func (s *Store) GetAirQualityReadings(start, end time.Time) ([]ecowitt.AirQualityReading, error) {
	rows, err := s.db.Query(`
		SELECT observed_at, pm25, real_time_aqi, aqi_24h, pm25_avg_24h, source_field_key
		FROM air_quality_readings
		WHERE observed_at >= ? AND observed_at <= ?
		ORDER BY observed_at ASC
	`, start.UTC(), end.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	readings := make([]ecowitt.AirQualityReading, 0)
	for rows.Next() {
		reading, err := scanAirQualityReading(rows.Scan)
		if err != nil {
			return nil, err
		}
		readings = append(readings, *reading)
	}
	return readings, rows.Err()
}

func scanAirQualityReading(scan func(dest ...any) error) (*ecowitt.AirQualityReading, error) {
	reading := &ecowitt.AirQualityReading{}
	var realTimeAQI sql.NullInt64
	var aqi24H sql.NullFloat64
	var pm25Avg24H sql.NullFloat64
	var sourceFieldKey sql.NullString

	if err := scan(
		&reading.ObservedAt,
		&reading.PM25,
		&realTimeAQI,
		&aqi24H,
		&pm25Avg24H,
		&sourceFieldKey,
	); err != nil {
		return nil, err
	}

	if realTimeAQI.Valid {
		reading.RealTimeAQI = int(realTimeAQI.Int64)
		reading.HasRealTimeAQI = true
		reading.Category, reading.CategoryClass = ecowitt.ClassifyAQI(reading.RealTimeAQI)
	}
	if aqi24H.Valid {
		reading.AQI24H = aqi24H.Float64
		reading.HasAQI24H = true
	}
	if pm25Avg24H.Valid {
		reading.PM25Avg24H = pm25Avg24H.Float64
		reading.HasPM25Avg24H = true
	}
	if sourceFieldKey.Valid {
		reading.SourceFieldKey = sourceFieldKey.String
	}

	return reading, nil
}

func nullAQI(valid bool, value int) sql.NullInt64 {
	if !valid {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(value), Valid: true}
}

func nullFloat(valid bool, value float64) sql.NullFloat64 {
	if !valid {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: value, Valid: true}
}

func nullString(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}
