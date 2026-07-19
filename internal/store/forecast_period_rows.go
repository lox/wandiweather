package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/lox/wandiweather/internal/models"
)

const forecastPeriodSelectColumns = `p.id, p.forecast_id, p.source, p.fetched_at, p.valid_date,
	p.day_of_forecast, p.period, p.period_start, p.period_end, p.is_night,
	p.location_id, p.raw_period_key`

const forecastComponentSelectColumns = `c.id, c.metric, c.value, c.value_min, c.value_max,
	c.value_text, c.unit`

func forecastPeriodBase(forecast models.Forecast, forecastID int64, loc *time.Location) (models.ForecastPeriod, time.Time) {
	validLocal := time.Date(forecast.ValidDate.Year(), forecast.ValidDate.Month(), forecast.ValidDate.Day(), 0, 0, 0, 0, loc)
	base := models.ForecastPeriod{
		ForecastID:    sql.NullInt64{Int64: forecastID, Valid: forecastID != 0},
		Source:        forecast.Source,
		FetchedAt:     forecast.FetchedAt,
		ValidDate:     forecast.ValidDate,
		DayOfForecast: forecast.DayOfForecast,
		LocationID:    forecast.LocationID,
	}
	return base, validLocal
}

func forecastPeriodsForForecast(forecast models.Forecast, forecastID int64, loc *time.Location) []models.ForecastPeriod {
	base, validLocal := forecastPeriodBase(forecast, forecastID, loc)

	if forecast.Source == "wu" {
		day := base
		day.Period = "day"
		day.PeriodStart = time.Date(validLocal.Year(), validLocal.Month(), validLocal.Day(), 6, 0, 0, 0, loc).UTC()
		day.PeriodEnd = time.Date(validLocal.Year(), validLocal.Month(), validLocal.Day(), 18, 0, 0, 0, loc).UTC()
		day.RawPeriodKey = "day"
		day.Components = rainComponents(forecast.PrecipChanceDay, forecast.PrecipAmountDay, sql.NullFloat64{}, sql.NullFloat64{}, "")

		night := base
		night.Period = "night"
		night.PeriodStart = time.Date(validLocal.Year(), validLocal.Month(), validLocal.Day(), 18, 0, 0, 0, loc).UTC()
		night.PeriodEnd = time.Date(validLocal.Year(), validLocal.Month(), validLocal.Day()+1, 6, 0, 0, 0, loc).UTC()
		night.IsNight = true
		night.RawPeriodKey = "night"
		night.Components = rainComponents(forecast.PrecipChanceNight, forecast.PrecipAmountNight, sql.NullFloat64{}, sql.NullFloat64{}, "")

		periods := make([]models.ForecastPeriod, 0, 2)
		if len(day.Components) > 0 {
			periods = append(periods, day)
		}
		if len(night.Components) > 0 {
			periods = append(periods, night)
		}
		if len(periods) == 0 {
			if daily, ok := dailyForecastPeriod(forecast, forecastID, loc); ok {
				return []models.ForecastPeriod{daily}
			}
		}
		return periods
	}

	if daily, ok := dailyForecastPeriod(forecast, forecastID, loc); ok {
		return []models.ForecastPeriod{daily}
	}
	return nil
}

func dailyForecastPeriod(forecast models.Forecast, forecastID int64, loc *time.Location) (models.ForecastPeriod, bool) {
	components := rainComponents(
		forecast.PrecipChance,
		forecast.PrecipAmount,
		forecast.PrecipMin,
		forecast.PrecipMax,
		forecast.PrecipUnits.String,
	)
	if len(components) == 0 {
		return models.ForecastPeriod{}, false
	}

	daily, validLocal := forecastPeriodBase(forecast, forecastID, loc)
	daily.Period = "daily"
	daily.PeriodStart = validLocal.UTC()
	daily.PeriodEnd = validLocal.AddDate(0, 0, 1).UTC()
	daily.RawPeriodKey = "daily"
	daily.Components = components
	return daily, true
}

func rainComponents(chance sql.NullInt64, amount, amountMin, amountMax sql.NullFloat64, amountUnit string) []models.ForecastComponent {
	components := make([]models.ForecastComponent, 0, 2)
	if chance.Valid {
		components = append(components, numericComponent(
			models.ForecastMetricPrecipChance,
			sql.NullFloat64{Float64: float64(chance.Int64), Valid: true},
			sql.NullFloat64{},
			sql.NullFloat64{},
			models.ForecastUnitPercent,
		))
	}
	if amount.Valid || amountMin.Valid || amountMax.Valid {
		if strings.TrimSpace(amountUnit) == "" {
			amountUnit = models.ForecastUnitMillimetres
		}
		if amountMin.Valid || amountMax.Valid {
			amount = sql.NullFloat64{}
		}
		components = append(components, numericComponent(
			models.ForecastMetricPrecipAmount,
			amount,
			amountMin,
			amountMax,
			amountUnit,
		))
	}
	return components
}

func numericComponent(metric string, value, valueMin, valueMax sql.NullFloat64, unit string) models.ForecastComponent {
	return models.ForecastComponent{
		Metric:   metric,
		Value:    value,
		ValueMin: valueMin,
		ValueMax: valueMax,
		Unit:     sql.NullString{String: unit, Valid: unit != ""},
	}
}

type forecastMetricRule struct {
	unit       string
	text       bool
	allowRange bool
	min        float64
	max        float64
	hasMin     bool
	hasMax     bool
}

var forecastMetricRules = map[string]forecastMetricRule{
	models.ForecastMetricPrecipChance: {unit: models.ForecastUnitPercent, min: 0, max: 100, hasMin: true, hasMax: true},
	models.ForecastMetricPrecipAmount: {unit: models.ForecastUnitMillimetres, allowRange: true, min: 0, hasMin: true},
	models.ForecastMetricTemperature:  {unit: models.ForecastUnitCelsius},
	models.ForecastMetricFeelsLike:    {unit: models.ForecastUnitCelsius},
	models.ForecastMetricDewpoint:     {unit: models.ForecastUnitCelsius},
	models.ForecastMetricHumidity:     {unit: models.ForecastUnitPercent, min: 0, max: 100, hasMin: true, hasMax: true},
	models.ForecastMetricWindSpeed:    {unit: models.ForecastUnitKilometresPerHour, min: 0, hasMin: true},
	models.ForecastMetricWindGust:     {unit: models.ForecastUnitKilometresPerHour, min: 0, hasMin: true},
	models.ForecastMetricWindDir:      {text: true},
}

func validateForecastPeriod(period models.ForecastPeriod) error {
	if strings.TrimSpace(period.Source) == "" {
		return fmt.Errorf("forecast period source is required")
	}
	if period.FetchedAt.IsZero() || period.ValidDate.IsZero() {
		return fmt.Errorf("forecast period source and valid timestamps are required")
	}
	if period.DayOfForecast < 0 {
		return fmt.Errorf("forecast period lead time cannot be negative")
	}
	if period.PeriodStart.IsZero() || !period.PeriodEnd.After(period.PeriodStart) {
		return fmt.Errorf("invalid %s forecast period bounds", period.Period)
	}
	if strings.TrimSpace(period.RawPeriodKey) == "" {
		return fmt.Errorf("forecast period raw key is required")
	}
	if len(period.Components) == 0 {
		return fmt.Errorf("forecast period has no components")
	}

	seen := make(map[string]struct{}, len(period.Components))
	for _, component := range period.Components {
		if _, exists := seen[component.Metric]; exists {
			return fmt.Errorf("duplicate forecast component %q", component.Metric)
		}
		seen[component.Metric] = struct{}{}
		if err := validateForecastComponent(component); err != nil {
			return err
		}
	}
	return nil
}

func validateForecastComponent(component models.ForecastComponent) error {
	rule, ok := forecastMetricRules[component.Metric]
	if !ok {
		return fmt.Errorf("unsupported forecast component metric %q", component.Metric)
	}

	hasNumeric := component.Value.Valid || component.ValueMin.Valid || component.ValueMax.Valid
	hasText := component.TextValue.Valid && strings.TrimSpace(component.TextValue.String) != ""
	if hasNumeric == hasText {
		return fmt.Errorf("forecast component %q must contain either numeric or text data", component.Metric)
	}
	if rule.text != hasText {
		return fmt.Errorf("forecast component %q has the wrong value type", component.Metric)
	}
	if hasText {
		if component.Unit.Valid {
			return fmt.Errorf("text forecast component %q cannot have a unit", component.Metric)
		}
		return nil
	}
	if !component.Unit.Valid || component.Unit.String != rule.unit {
		return fmt.Errorf("forecast component %q requires unit %q", component.Metric, rule.unit)
	}
	if !rule.allowRange && (component.ValueMin.Valid || component.ValueMax.Valid) {
		return fmt.Errorf("forecast component %q does not support a range", component.Metric)
	}
	if component.Value.Valid && (component.ValueMin.Valid || component.ValueMax.Valid) {
		return fmt.Errorf("forecast component %q cannot contain both a scalar and a range", component.Metric)
	}
	if component.ValueMin.Valid && component.ValueMax.Valid && component.ValueMax.Float64 < component.ValueMin.Float64 {
		return fmt.Errorf("forecast component %q has an inverted range", component.Metric)
	}
	for _, value := range []sql.NullFloat64{component.Value, component.ValueMin, component.ValueMax} {
		if !value.Valid {
			continue
		}
		if rule.hasMin && value.Float64 < rule.min {
			return fmt.Errorf("forecast component %q is below %.0f", component.Metric, rule.min)
		}
		if rule.hasMax && value.Float64 > rule.max {
			return fmt.Errorf("forecast component %q is above %.0f", component.Metric, rule.max)
		}
	}
	return nil
}

func insertForecastPeriodTx(tx *sql.Tx, period models.ForecastPeriod) error {
	if err := validateForecastPeriod(period); err != nil {
		return err
	}

	result, err := tx.Exec(`
		INSERT INTO forecast_periods (
			forecast_id, source, fetched_at, valid_date, day_of_forecast,
			period, period_start, period_end, is_night, location_id, raw_period_key
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING
	`, period.ForecastID, period.Source, period.FetchedAt, period.ValidDate,
		period.DayOfForecast, period.Period, period.PeriodStart, period.PeriodEnd,
		period.IsNight, period.LocationID, period.RawPeriodKey)
	if err != nil {
		return err
	}

	periodID, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get forecast period id: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get forecast period rows affected: %w", err)
	}
	if rowsAffected == 0 {
		if err := tx.QueryRow(`
			SELECT id FROM forecast_periods
			WHERE source = ? AND fetched_at = ? AND period_start = ? AND period = ?
			  AND COALESCE(location_id, '') = COALESCE(?, '')
		`, period.Source, period.FetchedAt, period.PeriodStart, period.Period, period.LocationID).Scan(&periodID); err != nil {
			return fmt.Errorf("find existing forecast period: %w", err)
		}
	}

	for _, component := range period.Components {
		if _, err := tx.Exec(`
			INSERT INTO forecast_components (
				forecast_period_id, metric, value, value_min, value_max, value_text, unit
			) VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(forecast_period_id, metric) DO NOTHING
		`, periodID, component.Metric, component.Value, component.ValueMin,
			component.ValueMax, component.TextValue, component.Unit); err != nil {
			return fmt.Errorf("insert forecast component %s: %w", component.Metric, err)
		}
	}
	return nil
}

func (s *Store) InsertForecastPeriod(period models.ForecastPeriod) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin forecast period insert: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := insertForecastPeriodTx(tx, period); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit forecast period insert: %w", err)
	}
	return nil
}

func (s *Store) insertForecastPeriodsForForecast(tx *sql.Tx, forecast models.Forecast, forecastID int64) error {
	for _, period := range forecastPeriodsForForecast(forecast, forecastID, s.loc) {
		if err := insertForecastPeriodTx(tx, period); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) GetLatestForecastPeriods(source, period string, start, end time.Time, limit int) ([]models.ForecastPeriod, error) {
	rows, err := s.db.Query(`
		WITH selected_periods AS (
			SELECT id
			FROM forecast_periods
			WHERE source = ? AND period = ?
			  AND fetched_at = (SELECT MAX(fetched_at) FROM forecast_periods WHERE source = ? AND period = ?)
			  AND period_start >= ? AND period_start < ?
			ORDER BY period_start
			LIMIT ?
		)
		SELECT `+forecastPeriodSelectColumns+`, `+forecastComponentSelectColumns+`
		FROM forecast_periods p
		JOIN forecast_components c ON c.forecast_period_id = p.id
		WHERE p.id IN (SELECT id FROM selected_periods)
		ORDER BY p.period_start, c.metric
	`, source, period, source, period, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanForecastPeriods(rows)
}

type forecastPeriodRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanForecastPeriods(rows forecastPeriodRows) ([]models.ForecastPeriod, error) {
	var results []models.ForecastPeriod
	periodIndexes := make(map[int64]int)
	for rows.Next() {
		var period models.ForecastPeriod
		var component models.ForecastComponent
		if err := rows.Scan(
			&period.ID, &period.ForecastID, &period.Source, &period.FetchedAt,
			&period.ValidDate, &period.DayOfForecast, &period.Period,
			&period.PeriodStart, &period.PeriodEnd, &period.IsNight,
			&period.LocationID, &period.RawPeriodKey,
			&component.ID, &component.Metric, &component.Value, &component.ValueMin,
			&component.ValueMax, &component.TextValue, &component.Unit,
		); err != nil {
			return nil, err
		}

		index, exists := periodIndexes[period.ID]
		if !exists {
			index = len(results)
			periodIndexes[period.ID] = index
			results = append(results, period)
		}
		component.ForecastPeriodID = period.ID
		results[index].Components = append(results[index].Components, component)
	}
	return results, rows.Err()
}
