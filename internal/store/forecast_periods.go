package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/lox/wandiweather/internal/models"
)

const (
	maxRainObservationGap      = 90 * time.Minute
	maxRainResetObservationGap = 15 * time.Minute
)

func rainPeriodBounds(validDate time.Time, period string, loc *time.Location) (time.Time, time.Time, error) {
	localDate := time.Date(validDate.Year(), validDate.Month(), validDate.Day(), 0, 0, 0, 0, loc)
	year, month, day := localDate.Date()

	var start, end time.Time
	switch period {
	case "daily":
		start = localDate
		end = time.Date(year, month, day+1, 0, 0, 0, 0, loc)
	case "day":
		start = time.Date(year, month, day, 6, 0, 0, 0, loc)
		end = time.Date(year, month, day, 18, 0, 0, 0, loc)
	case "night":
		start = time.Date(year, month, day, 18, 0, 0, 0, loc)
		end = time.Date(year, month, day+1, 6, 0, 0, 0, loc)
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("unsupported rain period %q", period)
	}
	return start, end, nil
}

// ComputeObservedRainPeriod derives rainfall from cumulative gauge readings.
// A falling gauge value is treated as a reset, with the new value contributing
// rainfall since the reset.
func (s *Store) ComputeObservedRainPeriod(stationID string, validDate time.Time, period string) (*models.ObservedPeriod, error) {
	start, end, err := rainPeriodBounds(validDate, period, s.loc)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(`
		SELECT observed_at, precip_total
		FROM observations
		WHERE station_id = ?
		  AND observed_at >= ?
		  AND observed_at <= ?
		  AND precip_total IS NOT NULL
		ORDER BY observed_at
	`, stationID, start.UTC(), end.UTC())
	if err != nil {
		return nil, fmt.Errorf("query rain observations: %w", err)
	}
	defer rows.Close()

	result := &models.ObservedPeriod{
		StationID:   stationID,
		ValidDate:   time.Date(validDate.Year(), validDate.Month(), validDate.Day(), 0, 0, 0, 0, time.UTC),
		Period:      period,
		PeriodStart: start.UTC(),
		PeriodEnd:   end.UTC(),
		ComputedAt:  time.Now().UTC(),
	}

	var previousAt time.Time
	var previousTotal float64
	var precipTotal float64
	var unsafeReset bool
	for rows.Next() {
		var observedAt time.Time
		var total float64
		if err := rows.Scan(&observedAt, &total); err != nil {
			return nil, fmt.Errorf("scan rain observation: %w", err)
		}
		result.ObservationCount++
		if !previousAt.IsZero() {
			gap := observedAt.Sub(previousAt)
			gaugeReset := total < previousTotal
			if gaugeReset && gap > maxRainResetObservationGap {
				unsafeReset = true
			}
			if gap > 0 && gap <= maxRainObservationGap && (!gaugeReset || gap <= maxRainResetObservationGap) {
				result.CoverageMinutes += int(gap / time.Minute)
			}
			if !gaugeReset {
				precipTotal += total - previousTotal
			} else {
				precipTotal += total
			}
		}
		previousAt = observedAt
		previousTotal = total
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rain observations: %w", err)
	}
	if result.ObservationCount >= 2 {
		result.PrecipTotal = sql.NullFloat64{Float64: precipTotal, Valid: true}
	}
	durationMinutes := int(result.PeriodEnd.Sub(result.PeriodStart) / time.Minute)
	result.IsComplete = result.PrecipTotal.Valid && !unsafeReset && durationMinutes > 0 &&
		result.CoverageMinutes*100 >= durationMinutes*95

	return result, nil
}

func (s *Store) UpsertObservedPeriod(period models.ObservedPeriod) error {
	_, err := s.db.Exec(`
		INSERT INTO observed_periods (
			station_id, valid_date, period, period_start, period_end,
			precip_total, observation_count, coverage_minutes, is_complete, computed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(station_id, valid_date, period) DO UPDATE SET
			period_start = excluded.period_start,
			period_end = excluded.period_end,
			precip_total = excluded.precip_total,
			observation_count = excluded.observation_count,
			coverage_minutes = excluded.coverage_minutes,
			is_complete = excluded.is_complete,
			computed_at = excluded.computed_at
	`, period.StationID, period.ValidDate, period.Period, period.PeriodStart, period.PeriodEnd,
		period.PrecipTotal, period.ObservationCount, period.CoverageMinutes, period.IsComplete, period.ComputedAt)
	return err
}

func (s *Store) GetObservedPeriod(stationID string, validDate time.Time, period string) (*models.ObservedPeriod, error) {
	var result models.ObservedPeriod
	err := s.db.QueryRow(`
		SELECT id, station_id, valid_date, period, period_start, period_end,
		       precip_total, observation_count, coverage_minutes, is_complete, computed_at
		FROM observed_periods
		WHERE station_id = ? AND SUBSTR(valid_date, 1, 10) = ? AND period = ?
	`, stationID, validDate.Format("2006-01-02"), period).Scan(
		&result.ID, &result.StationID, &result.ValidDate, &result.Period,
		&result.PeriodStart, &result.PeriodEnd, &result.PrecipTotal,
		&result.ObservationCount, &result.CoverageMinutes, &result.IsComplete, &result.ComputedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *Store) DeleteForecastComponentVerifications(observedPeriodID int64, verificationKind, verifierVersion string) error {
	_, err := s.db.Exec(`
		DELETE FROM forecast_component_verification
		WHERE observed_period_id = ?
		  AND verification_kind = ?
		  AND verifier_version = ?
	`, observedPeriodID, verificationKind, verifierVersion)
	return err
}

func (s *Store) UpsertForecastComponentVerification(v models.ForecastComponentVerification) error {
	_, err := s.db.Exec(`
		INSERT INTO forecast_component_verification (
			forecast_component_id, observed_period_id, verification_kind,
			actual_value, forecast_threshold, actual_threshold, verifier_version, hit_class
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(forecast_component_id, observed_period_id, verification_kind, verifier_version) DO UPDATE SET
			actual_value = excluded.actual_value,
			forecast_threshold = excluded.forecast_threshold,
			actual_threshold = excluded.actual_threshold,
			hit_class = excluded.hit_class,
			created_at = CURRENT_TIMESTAMP
	`, v.ForecastComponentID, v.ObservedPeriodID, v.VerificationKind,
		v.ActualValue, v.ForecastThreshold, v.ActualThreshold, v.VerifierVersion, v.HitClass)
	return err
}

func (s *Store) GetForecastComponentVerifications(validDate time.Time) ([]models.ForecastComponentVerification, error) {
	rows, err := s.db.Query(`
		SELECT verification.id, verification.forecast_component_id,
		       verification.observed_period_id, verification.verification_kind,
		       period.valid_date, period.source, period.day_of_forecast, period.period,
		       component.metric, component.value, component.value_min, component.value_max,
		       verification.actual_value, verification.forecast_threshold,
		       verification.actual_threshold, verification.verifier_version,
		       verification.hit_class, verification.created_at
		FROM forecast_component_verification verification
		JOIN forecast_components component ON component.id = verification.forecast_component_id
		JOIN forecast_periods period ON period.id = component.forecast_period_id
		WHERE SUBSTR(period.valid_date, 1, 10) = ?
		ORDER BY period.source, period.day_of_forecast, period.period, component.metric
	`, validDate.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.ForecastComponentVerification
	for rows.Next() {
		var result models.ForecastComponentVerification
		if err := rows.Scan(
			&result.ID, &result.ForecastComponentID, &result.ObservedPeriodID,
			&result.VerificationKind, &result.ValidDate, &result.Source,
			&result.DayOfForecast, &result.Period, &result.Metric,
			&result.ForecastValue, &result.ForecastValueMin,
			&result.ForecastValueMax, &result.ActualValue,
			&result.ForecastThreshold, &result.ActualThreshold,
			&result.VerifierVersion, &result.HitClass, &result.CreatedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, rows.Err()
}

// GetRainTimingVerificationPeriods returns the earliest advance WU day/night
// periods for each available lead time.
func (s *Store) GetRainTimingVerificationPeriods(validDate time.Time) ([]models.ForecastPeriod, error) {
	dateStr := validDate.Format("2006-01-02")
	cutoff := time.Date(validDate.Year(), validDate.Month(), validDate.Day(), 0, 0, 0, 0, s.loc).UTC()

	rows, err := s.db.Query(`
		WITH selected_forecasts AS (
			SELECT forecast.id
			FROM forecasts forecast
			INNER JOIN (
			SELECT candidate.source, candidate.day_of_forecast, MIN(candidate.fetched_at) AS first_fetch
			FROM forecasts candidate
			WHERE SUBSTR(candidate.valid_date, 1, 10) = ?
			  AND candidate.source = 'wu'
			  AND candidate.fetched_at < ?
			  AND EXISTS (
			      SELECT 1
			      FROM forecast_periods candidate_period
			      JOIN forecast_components candidate_component
			        ON candidate_component.forecast_period_id = candidate_period.id
			      WHERE candidate_period.forecast_id = candidate.id
			        AND candidate_period.period IN ('day', 'night')
			        AND candidate_component.metric IN ('precip_chance', 'precip_amount')
			  )
			GROUP BY candidate.source, candidate.day_of_forecast
			) selected ON forecast.source = selected.source
			          AND forecast.day_of_forecast = selected.day_of_forecast
			          AND forecast.fetched_at = selected.first_fetch
			WHERE SUBSTR(forecast.valid_date, 1, 10) = ?
		)
		SELECT `+forecastPeriodSelectColumns+`, `+forecastComponentSelectColumns+`
		FROM forecast_periods p
		JOIN forecast_components c ON c.forecast_period_id = p.id
		WHERE p.forecast_id IN (SELECT id FROM selected_forecasts)
		  AND p.period IN ('day', 'night')
		ORDER BY p.day_of_forecast, p.period_start, c.metric
	`, dateStr, cutoff, dateStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanForecastPeriods(rows)
}

type RainTimingVerificationStats struct {
	Period      string
	Samples     int
	Hits        int
	FalseAlarms int
	Misses      int
	CorrectDry  int
}

func (s *Store) GetRainTimingVerificationStats(windowDays int) ([]RainTimingVerificationStats, error) {
	cutoff := time.Now().In(s.loc).AddDate(0, 0, -windowDays).Format("2006-01-02")
	rows, err := s.db.Query(`
		SELECT period.period,
		       COUNT(*),
		       SUM(CASE WHEN verification.hit_class = 'hit' THEN 1 ELSE 0 END),
		       SUM(CASE WHEN verification.hit_class = 'false_alarm' THEN 1 ELSE 0 END),
		       SUM(CASE WHEN verification.hit_class = 'miss' THEN 1 ELSE 0 END),
		       SUM(CASE WHEN verification.hit_class = 'correct_dry' THEN 1 ELSE 0 END)
		FROM forecast_component_verification verification
		JOIN forecast_components component ON component.id = verification.forecast_component_id
		JOIN forecast_periods period ON period.id = component.forecast_period_id
		WHERE verification.verification_kind = 'rain_occurrence'
		  AND verification.verifier_version = ?
		  AND period.source = 'wu'
		  AND period.day_of_forecast = 1
		  AND period.period IN ('day', 'night')
		  AND SUBSTR(period.valid_date, 1, 10) >= ?
		GROUP BY period.period
		ORDER BY CASE period.period WHEN 'day' THEN 1 ELSE 2 END
	`, models.ForecastVerifierRainOccurrenceV1, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []RainTimingVerificationStats
	for rows.Next() {
		var result RainTimingVerificationStats
		if err := rows.Scan(
			&result.Period, &result.Samples, &result.Hits,
			&result.FalseAlarms, &result.Misses, &result.CorrectDry,
		); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, rows.Err()
}
