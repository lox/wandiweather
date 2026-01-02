package store

import (
	"database/sql"
	"time"

	"github.com/lox/wandiweather/internal/models"
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) UpsertStation(st models.Station) error {
	_, err := s.db.Exec(`
		INSERT INTO stations (station_id, name, latitude, longitude, elevation, elevation_tier, is_primary, active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(station_id) DO UPDATE SET
			name = excluded.name,
			latitude = excluded.latitude,
			longitude = excluded.longitude,
			elevation = excluded.elevation,
			elevation_tier = excluded.elevation_tier,
			is_primary = excluded.is_primary,
			active = excluded.active
	`, st.StationID, st.Name, st.Latitude, st.Longitude, st.Elevation, st.ElevationTier, st.IsPrimary, st.Active)
	return err
}

func (s *Store) GetActiveStations() ([]models.Station, error) {
	rows, err := s.db.Query(`SELECT station_id, name, latitude, longitude, elevation, elevation_tier, is_primary, active FROM stations WHERE active = TRUE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stations []models.Station
	for rows.Next() {
		var st models.Station
		if err := rows.Scan(&st.StationID, &st.Name, &st.Latitude, &st.Longitude, &st.Elevation, &st.ElevationTier, &st.IsPrimary, &st.Active); err != nil {
			return nil, err
		}
		stations = append(stations, st)
	}
	return stations, rows.Err()
}

func (s *Store) InsertObservation(obs models.Observation) error {
	_, err := s.db.Exec(`
		INSERT INTO observations (station_id, observed_at, temp, humidity, dewpoint, pressure, wind_speed, wind_gust, wind_dir, precip_rate, precip_total, solar_radiation, uv, heat_index, wind_chill, qc_status, raw_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(station_id, observed_at) DO NOTHING
	`, obs.StationID, obs.ObservedAt, obs.Temp, obs.Humidity, obs.Dewpoint, obs.Pressure, obs.WindSpeed, obs.WindGust, obs.WindDir, obs.PrecipRate, obs.PrecipTotal, obs.SolarRadiation, obs.UV, obs.HeatIndex, obs.WindChill, obs.QCStatus, obs.RawJSON)
	return err
}

func (s *Store) GetLatestObservation(stationID string) (*models.Observation, error) {
	row := s.db.QueryRow(`
		SELECT id, station_id, observed_at, temp, humidity, dewpoint, pressure, wind_speed, wind_gust, wind_dir, precip_rate, precip_total, solar_radiation, uv, heat_index, wind_chill, qc_status, raw_json, created_at
		FROM observations
		WHERE station_id = ?
		ORDER BY observed_at DESC
		LIMIT 1
	`, stationID)

	var obs models.Observation
	err := row.Scan(&obs.ID, &obs.StationID, &obs.ObservedAt, &obs.Temp, &obs.Humidity, &obs.Dewpoint, &obs.Pressure, &obs.WindSpeed, &obs.WindGust, &obs.WindDir, &obs.PrecipRate, &obs.PrecipTotal, &obs.SolarRadiation, &obs.UV, &obs.HeatIndex, &obs.WindChill, &obs.QCStatus, &obs.RawJSON, &obs.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &obs, nil
}

func (s *Store) GetObservations(stationID string, start, end time.Time) ([]models.Observation, error) {
	rows, err := s.db.Query(`
		SELECT id, station_id, observed_at, temp, humidity, dewpoint, pressure, wind_speed, wind_gust, wind_dir, precip_rate, precip_total, solar_radiation, uv, heat_index, wind_chill, qc_status, raw_json, created_at
		FROM observations
		WHERE station_id = ? AND observed_at >= ? AND observed_at <= ?
		ORDER BY observed_at ASC
	`, stationID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var observations []models.Observation
	for rows.Next() {
		var obs models.Observation
		if err := rows.Scan(&obs.ID, &obs.StationID, &obs.ObservedAt, &obs.Temp, &obs.Humidity, &obs.Dewpoint, &obs.Pressure, &obs.WindSpeed, &obs.WindGust, &obs.WindDir, &obs.PrecipRate, &obs.PrecipTotal, &obs.SolarRadiation, &obs.UV, &obs.HeatIndex, &obs.WindChill, &obs.QCStatus, &obs.RawJSON, &obs.CreatedAt); err != nil {
			return nil, err
		}
		observations = append(observations, obs)
	}
	return observations, rows.Err()
}

func (s *Store) InsertForecast(f models.Forecast) error {
	source := f.Source
	if source == "" {
		source = "wu"
	}
	_, err := s.db.Exec(`
		INSERT INTO forecasts (source, fetched_at, valid_date, day_of_forecast, temp_max, temp_min, humidity, precip_chance, precip_amount, precip_range, wind_speed, wind_dir, narrative, raw_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source, fetched_at, valid_date) DO NOTHING
	`, source, f.FetchedAt, f.ValidDate, f.DayOfForecast, f.TempMax, f.TempMin, f.Humidity, f.PrecipChance, f.PrecipAmount, f.PrecipRange, f.WindSpeed, f.WindDir, f.Narrative, f.RawJSON)
	return err
}

func (s *Store) ComputeDailySummary(stationID string, date time.Time) (*models.DailySummary, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := startOfDay.Add(24 * time.Hour)

	row := s.db.QueryRow(`
		SELECT 
			MAX(temp) as temp_max,
			MIN(temp) as temp_min,
			AVG(temp) as temp_avg,
			AVG(humidity) as humidity_avg,
			AVG(pressure) as pressure_avg,
			SUM(precip_total) as precip_total,
			MAX(wind_gust) as wind_max_gust
		FROM observations
		WHERE station_id = ? AND observed_at >= ? AND observed_at < ? AND temp IS NOT NULL
	`, stationID, startOfDay, endOfDay)

	var summary models.DailySummary
	summary.Date = startOfDay
	summary.StationID = stationID

	err := row.Scan(&summary.TempMax, &summary.TempMin, &summary.TempAvg, &summary.HumidityAvg, &summary.PressureAvg, &summary.PrecipTotal, &summary.WindMaxGust)
	if err != nil {
		return nil, err
	}

	// Get time of max temp
	s.db.QueryRow(`SELECT observed_at FROM observations WHERE station_id = ? AND observed_at >= ? AND observed_at < ? AND temp = ? LIMIT 1`,
		stationID, startOfDay, endOfDay, summary.TempMax).Scan(&summary.TempMaxTime)
	// Get time of min temp
	s.db.QueryRow(`SELECT observed_at FROM observations WHERE station_id = ? AND observed_at >= ? AND observed_at < ? AND temp = ? LIMIT 1`,
		stationID, startOfDay, endOfDay, summary.TempMin).Scan(&summary.TempMinTime)

	return &summary, nil
}

func (s *Store) UpsertDailySummary(ds models.DailySummary) error {
	_, err := s.db.Exec(`
		INSERT INTO daily_summaries (date, station_id, temp_max, temp_max_time, temp_min, temp_min_time, temp_avg, humidity_avg, pressure_avg, precip_total, wind_max_gust, inversion_detected, inversion_strength)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(date, station_id) DO UPDATE SET
			temp_max = excluded.temp_max,
			temp_max_time = excluded.temp_max_time,
			temp_min = excluded.temp_min,
			temp_min_time = excluded.temp_min_time,
			temp_avg = excluded.temp_avg,
			humidity_avg = excluded.humidity_avg,
			pressure_avg = excluded.pressure_avg,
			precip_total = excluded.precip_total,
			wind_max_gust = excluded.wind_max_gust,
			inversion_detected = excluded.inversion_detected,
			inversion_strength = excluded.inversion_strength
	`, ds.Date, ds.StationID, ds.TempMax, ds.TempMaxTime, ds.TempMin, ds.TempMinTime, ds.TempAvg, ds.HumidityAvg, ds.PressureAvg, ds.PrecipTotal, ds.WindMaxGust, ds.InversionDetected, ds.InversionStrength)
	return err
}

func (s *Store) GetDailySummaries(stationID string, start, end time.Time) ([]models.DailySummary, error) {
	rows, err := s.db.Query(`
		SELECT date, station_id, temp_max, temp_max_time, temp_min, temp_min_time, temp_avg, humidity_avg, pressure_avg, precip_total, wind_max_gust, inversion_detected, inversion_strength
		FROM daily_summaries
		WHERE station_id = ? AND date >= ? AND date <= ?
		ORDER BY date ASC
	`, stationID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []models.DailySummary
	for rows.Next() {
		var ds models.DailySummary
		if err := rows.Scan(&ds.Date, &ds.StationID, &ds.TempMax, &ds.TempMaxTime, &ds.TempMin, &ds.TempMinTime, &ds.TempAvg, &ds.HumidityAvg, &ds.PressureAvg, &ds.PrecipTotal, &ds.WindMaxGust, &ds.InversionDetected, &ds.InversionStrength); err != nil {
			return nil, err
		}
		summaries = append(summaries, ds)
	}
	return summaries, rows.Err()
}

func (s *Store) GetStationsByTier(tier string) ([]models.Station, error) {
	rows, err := s.db.Query(`SELECT station_id, name, latitude, longitude, elevation, elevation_tier, is_primary, active FROM stations WHERE elevation_tier = ? AND active = TRUE ORDER BY elevation ASC`, tier)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stations []models.Station
	for rows.Next() {
		var st models.Station
		if err := rows.Scan(&st.StationID, &st.Name, &st.Latitude, &st.Longitude, &st.Elevation, &st.ElevationTier, &st.IsPrimary, &st.Active); err != nil {
			return nil, err
		}
		stations = append(stations, st)
	}
	return stations, rows.Err()
}

func (s *Store) GetObservationDates(stationID string) ([]time.Time, error) {
	rows, err := s.db.Query(`SELECT DISTINCT SUBSTR(observed_at, 1, 10) as date FROM observations WHERE station_id = ? ORDER BY date ASC`, stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dates []time.Time
	for rows.Next() {
		var dateStr string
		if err := rows.Scan(&dateStr); err != nil {
			return nil, err
		}
		date, _ := time.Parse("2006-01-02", dateStr)
		dates = append(dates, date)
	}
	return dates, rows.Err()
}

func (s *Store) GetOvernightMinByTier(date time.Time) (map[string]float64, error) {
	startUTC := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC).Add(-11 * time.Hour)
	endUTC := startUTC.Add(8 * time.Hour)

	rows, err := s.db.Query(`
		SELECT s.elevation_tier, MIN(o.temp) as min_temp
		FROM observations o
		JOIN stations s ON o.station_id = s.station_id
		WHERE s.active = TRUE
		  AND o.temp IS NOT NULL
		  AND o.observed_at >= ? AND o.observed_at < ?
		GROUP BY s.elevation_tier
	`, startUTC, endUTC)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]float64)
	for rows.Next() {
		var tier string
		var minTemp float64
		if err := rows.Scan(&tier, &minTemp); err != nil {
			return nil, err
		}
		result[tier] = minTemp
	}
	return result, rows.Err()
}

func (s *Store) GetForecastsForDate(validDate time.Time) ([]models.Forecast, error) {
	dateStr := validDate.Format("2006-01-02")
	rows, err := s.db.Query(`
		SELECT id, source, fetched_at, valid_date, day_of_forecast, temp_max, temp_min, humidity, precip_chance, precip_amount, wind_speed, wind_dir, narrative
		FROM forecasts
		WHERE SUBSTR(valid_date, 1, 10) = ?
		ORDER BY fetched_at DESC
	`, dateStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var forecasts []models.Forecast
	for rows.Next() {
		var f models.Forecast
		if err := rows.Scan(&f.ID, &f.Source, &f.FetchedAt, &f.ValidDate, &f.DayOfForecast, &f.TempMax, &f.TempMin, &f.Humidity, &f.PrecipChance, &f.PrecipAmount, &f.WindSpeed, &f.WindDir, &f.Narrative); err != nil {
			return nil, err
		}
		forecasts = append(forecasts, f)
	}
	return forecasts, rows.Err()
}

type DayActuals struct {
	TempMax   sql.NullFloat64
	TempMin   sql.NullFloat64
	WindGust  sql.NullFloat64
	PrecipSum sql.NullFloat64
}

func (s *Store) GetActualsForDate(stationID string, date time.Time) (*DayActuals, error) {
	startUTC := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC).Add(-11 * time.Hour)
	endUTC := startUTC.Add(24 * time.Hour)

	var a DayActuals
	err := s.db.QueryRow(`
		SELECT MAX(temp), MIN(temp), MAX(wind_gust), MAX(precip_total)
		FROM observations
		WHERE station_id = ? AND observed_at >= ? AND observed_at < ? AND temp IS NOT NULL
	`, stationID, startUTC, endUTC).Scan(&a.TempMax, &a.TempMin, &a.WindGust, &a.PrecipSum)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) InsertForecastVerification(v models.ForecastVerification) error {
	_, err := s.db.Exec(`
		INSERT INTO forecast_verification (
			forecast_id, valid_date, 
			forecast_temp_max, forecast_temp_min, actual_temp_max, actual_temp_min, bias_temp_max, bias_temp_min,
			forecast_wind_speed, actual_wind_gust, bias_wind,
			forecast_precip, actual_precip, bias_precip
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, v.ForecastID, v.ValidDate,
		v.ForecastTempMax, v.ForecastTempMin, v.ActualTempMax, v.ActualTempMin, v.BiasTempMax, v.BiasTempMin,
		v.ForecastWindSpeed, v.ActualWindGust, v.BiasWind,
		v.ForecastPrecip, v.ActualPrecip, v.BiasPrecip)
	return err
}

func (s *Store) HasVerificationForDate(validDate time.Time) (bool, error) {
	var count int
	dateStr := validDate.Format("2006-01-02")
	err := s.db.QueryRow(`SELECT COUNT(*) FROM forecast_verification WHERE SUBSTR(valid_date, 1, 10) = ?`, dateStr).Scan(&count)
	return count > 0, err
}

func (s *Store) GetPrimaryStation() (*models.Station, error) {
	row := s.db.QueryRow(`SELECT station_id, name, latitude, longitude, elevation, elevation_tier, is_primary, active FROM stations WHERE is_primary = TRUE LIMIT 1`)
	var st models.Station
	err := row.Scan(&st.StationID, &st.Name, &st.Latitude, &st.Longitude, &st.Elevation, &st.ElevationTier, &st.IsPrimary, &st.Active)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *Store) GetTodayStats(stationID string, localDate time.Time) (minTemp, maxTemp, rainTotal sql.NullFloat64, err error) {
	startUTC := time.Date(localDate.Year(), localDate.Month(), localDate.Day(), 0, 0, 0, 0, time.UTC).Add(-11 * time.Hour)
	endUTC := time.Now().UTC()

	err = s.db.QueryRow(`
		SELECT MIN(temp), MAX(temp), MAX(precip_total)
		FROM observations
		WHERE station_id = ? AND observed_at >= ? AND observed_at <= ? AND temp IS NOT NULL
	`, stationID, startUTC, endUTC).Scan(&minTemp, &maxTemp, &rainTotal)
	return
}

func (s *Store) GetLatestForecasts() (map[string][]models.Forecast, error) {
	today := time.Now().UTC().Format("2006-01-02")
	rows, err := s.db.Query(`
		WITH latest AS (
			SELECT source, MAX(fetched_at) as max_fetched
			FROM forecasts
			GROUP BY source
		)
		SELECT f.id, f.source, f.fetched_at, f.valid_date, f.day_of_forecast, 
		       f.temp_max, f.temp_min, f.precip_chance, f.precip_amount, f.precip_range, 
		       f.wind_speed, f.wind_dir, f.narrative
		FROM forecasts f
		JOIN latest l ON f.source = l.source AND f.fetched_at = l.max_fetched
		WHERE SUBSTR(f.valid_date, 1, 10) >= ?
		ORDER BY f.valid_date, f.source
	`, today)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]models.Forecast)
	for rows.Next() {
		var f models.Forecast
		if err := rows.Scan(&f.ID, &f.Source, &f.FetchedAt, &f.ValidDate, &f.DayOfForecast,
			&f.TempMax, &f.TempMin, &f.PrecipChance, &f.PrecipAmount, &f.PrecipRange,
			&f.WindSpeed, &f.WindDir, &f.Narrative); err != nil {
			return nil, err
		}
		result[f.Source] = append(result[f.Source], f)
	}
	return result, rows.Err()
}

func (s *Store) GetVerificationStats() (map[string]models.VerificationStats, error) {
	rows, err := s.db.Query(`
		SELECT 
			f.source,
			COUNT(*) as count,
			AVG(v.bias_temp_max) as avg_max_bias,
			AVG(v.bias_temp_min) as avg_min_bias,
			AVG(ABS(v.bias_temp_max)) as mae_max,
			AVG(ABS(v.bias_temp_min)) as mae_min,
			AVG(v.bias_wind) as avg_wind_bias,
			AVG(ABS(v.bias_wind)) as mae_wind,
			AVG(v.bias_precip) as avg_precip_bias,
			AVG(ABS(v.bias_precip)) as mae_precip
		FROM forecast_verification v
		JOIN forecasts f ON v.forecast_id = f.id
		WHERE v.bias_temp_max IS NOT NULL
		GROUP BY f.source
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]models.VerificationStats)
	for rows.Next() {
		var source string
		var stats models.VerificationStats
		if err := rows.Scan(&source, &stats.Count, &stats.AvgMaxBias, &stats.AvgMinBias,
			&stats.MAEMax, &stats.MAEMin, &stats.AvgWindBias, &stats.MAEWind,
			&stats.AvgPrecipBias, &stats.MAEPrecip); err != nil {
			return nil, err
		}
		result[source] = stats
	}
	return result, rows.Err()
}

func (s *Store) GetVerificationHistory(source string, limit int) ([]models.ForecastVerification, error) {
	rows, err := s.db.Query(`
		SELECT v.id, v.forecast_id, v.valid_date, v.forecast_temp_max, v.forecast_temp_min,
		       v.actual_temp_max, v.actual_temp_min, v.bias_temp_max, v.bias_temp_min, v.created_at
		FROM forecast_verification v
		JOIN forecasts f ON v.forecast_id = f.id
		WHERE f.source = ? AND v.bias_temp_max IS NOT NULL
		ORDER BY v.valid_date DESC
		LIMIT ?
	`, source, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.ForecastVerification
	for rows.Next() {
		var v models.ForecastVerification
		if err := rows.Scan(&v.ID, &v.ForecastID, &v.ValidDate, &v.ForecastTempMax, &v.ForecastTempMin,
			&v.ActualTempMax, &v.ActualTempMin, &v.BiasTempMax, &v.BiasTempMin, &v.CreatedAt); err != nil {
			return nil, err
		}
		results = append(results, v)
	}
	return results, rows.Err()
}

type BiasRow struct {
	Source        string
	DayOfForecast int
	AvgBiasMax    float64
	AvgBiasMin    float64
	MAEMax        float64
	MAEMin        float64
	CountMax      int
	CountMin      int
}

func (s *Store) GetBiasStatsFromVerification(windowDays int) ([]BiasRow, error) {
	cutoff := time.Now().AddDate(0, 0, -windowDays).Format("2006-01-02")
	rows, err := s.db.Query(`
		SELECT 
			f.source,
			f.day_of_forecast,
			COALESCE(AVG(v.bias_temp_max), 0) as avg_bias_max,
			COALESCE(AVG(v.bias_temp_min), 0) as avg_bias_min,
			COALESCE(AVG(ABS(v.bias_temp_max)), 0) as mae_max,
			COALESCE(AVG(ABS(v.bias_temp_min)), 0) as mae_min,
			COUNT(v.bias_temp_max) as count_max,
			COUNT(v.bias_temp_min) as count_min
		FROM forecast_verification v
		JOIN forecasts f ON v.forecast_id = f.id
		WHERE SUBSTR(v.valid_date, 1, 10) >= ?
		  AND (v.bias_temp_max IS NOT NULL OR v.bias_temp_min IS NOT NULL)
		GROUP BY f.source, f.day_of_forecast
	`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []BiasRow
	for rows.Next() {
		var r BiasRow
		if err := rows.Scan(&r.Source, &r.DayOfForecast, &r.AvgBiasMax, &r.AvgBiasMin,
			&r.MAEMax, &r.MAEMin, &r.CountMax, &r.CountMin); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

type CorrectionStats struct {
	Source        string
	Target        string
	DayOfForecast int
	Regime        string
	WindowDays    int
	SampleSize    int
	MeanBias      float64
	MAE           float64
	UpdatedAt     time.Time
}

func (s *Store) UpsertCorrectionStats(stats CorrectionStats) error {
	_, err := s.db.Exec(`
		INSERT INTO forecast_correction_stats (source, target, day_of_forecast, regime, window_days, sample_size, mean_bias, mae, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source, target, day_of_forecast, regime) DO UPDATE SET
			window_days = excluded.window_days,
			sample_size = excluded.sample_size,
			mean_bias = excluded.mean_bias,
			mae = excluded.mae,
			updated_at = excluded.updated_at
	`, stats.Source, stats.Target, stats.DayOfForecast, stats.Regime, stats.WindowDays, stats.SampleSize, stats.MeanBias, stats.MAE, stats.UpdatedAt)
	return err
}

func (s *Store) GetCorrectionStats(source, target string, dayOfForecast int) (*CorrectionStats, error) {
	row := s.db.QueryRow(`
		SELECT source, target, day_of_forecast, regime, window_days, sample_size, mean_bias, mae, updated_at
		FROM forecast_correction_stats
		WHERE source = ? AND target = ? AND day_of_forecast = ? AND regime = 'all'
	`, source, target, dayOfForecast)

	var stats CorrectionStats
	err := row.Scan(&stats.Source, &stats.Target, &stats.DayOfForecast, &stats.Regime,
		&stats.WindowDays, &stats.SampleSize, &stats.MeanBias, &stats.MAE, &stats.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &stats, nil
}

func (s *Store) GetAllCorrectionStats() (map[string]map[string]map[int]*CorrectionStats, error) {
	rows, err := s.db.Query(`
		SELECT source, target, day_of_forecast, regime, window_days, sample_size, mean_bias, mae, updated_at
		FROM forecast_correction_stats
		WHERE regime = 'all'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]map[string]map[int]*CorrectionStats)
	for rows.Next() {
		var stats CorrectionStats
		if err := rows.Scan(&stats.Source, &stats.Target, &stats.DayOfForecast, &stats.Regime,
			&stats.WindowDays, &stats.SampleSize, &stats.MeanBias, &stats.MAE, &stats.UpdatedAt); err != nil {
			return nil, err
		}

		if result[stats.Source] == nil {
			result[stats.Source] = make(map[string]map[int]*CorrectionStats)
		}
		if result[stats.Source][stats.Target] == nil {
			result[stats.Source][stats.Target] = make(map[int]*CorrectionStats)
		}
		s := stats
		result[stats.Source][stats.Target][stats.DayOfForecast] = &s
	}
	return result, rows.Err()
}

func (s *Store) GetRecentDailySummaries(stationID string, days int) ([]models.DailySummary, error) {
	rows, err := s.db.Query(`
		SELECT date, station_id, temp_max, temp_max_time, temp_min, temp_min_time, temp_avg, 
		       humidity_avg, pressure_avg, precip_total, wind_max_gust, inversion_detected, inversion_strength,
		       regime_heatwave, regime_inversion, regime_clear_calm
		FROM daily_summaries
		WHERE station_id = ?
		ORDER BY date DESC
		LIMIT ?
	`, stationID, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []models.DailySummary
	for rows.Next() {
		var ds models.DailySummary
		if err := rows.Scan(&ds.Date, &ds.StationID, &ds.TempMax, &ds.TempMaxTime, &ds.TempMin, &ds.TempMinTime,
			&ds.TempAvg, &ds.HumidityAvg, &ds.PressureAvg, &ds.PrecipTotal, &ds.WindMaxGust,
			&ds.InversionDetected, &ds.InversionStrength,
			&ds.RegimeHeatwave, &ds.RegimeInversion, &ds.RegimeClearCalm); err != nil {
			return nil, err
		}
		summaries = append(summaries, ds)
	}
	return summaries, rows.Err()
}

type NowcastLog struct {
	ID                   int64
	Date                 time.Time
	StationID            string
	ObservedMorning      sql.NullFloat64
	ForecastMorning      sql.NullFloat64
	Delta                sql.NullFloat64
	Adjustment           sql.NullFloat64
	ForecastMaxRaw       sql.NullFloat64
	ForecastMaxCorrected sql.NullFloat64
	ActualMax            sql.NullFloat64
	CreatedAt            time.Time
}

func (s *Store) UpsertNowcastLog(log NowcastLog) error {
	_, err := s.db.Exec(`
		INSERT INTO nowcast_log (date, station_id, observed_morning, forecast_morning, delta, adjustment, forecast_max_raw, forecast_max_corrected, actual_max)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(date, station_id) DO UPDATE SET
			observed_morning = excluded.observed_morning,
			forecast_morning = excluded.forecast_morning,
			delta = excluded.delta,
			adjustment = excluded.adjustment,
			forecast_max_raw = excluded.forecast_max_raw,
			forecast_max_corrected = excluded.forecast_max_corrected,
			actual_max = excluded.actual_max
	`, log.Date, log.StationID, log.ObservedMorning, log.ForecastMorning, log.Delta, log.Adjustment,
		log.ForecastMaxRaw, log.ForecastMaxCorrected, log.ActualMax)
	return err
}

func (s *Store) GetNowcastLog(stationID string, date time.Time) (*NowcastLog, error) {
	dateStr := date.Format("2006-01-02")
	row := s.db.QueryRow(`
		SELECT id, date, station_id, observed_morning, forecast_morning, delta, adjustment, 
		       forecast_max_raw, forecast_max_corrected, actual_max, created_at
		FROM nowcast_log
		WHERE station_id = ? AND SUBSTR(date, 1, 10) = ?
	`, stationID, dateStr)

	var log NowcastLog
	err := row.Scan(&log.ID, &log.Date, &log.StationID, &log.ObservedMorning, &log.ForecastMorning,
		&log.Delta, &log.Adjustment, &log.ForecastMaxRaw, &log.ForecastMaxCorrected, &log.ActualMax, &log.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &log, nil
}

func (s *Store) UpdateNowcastActualMax(stationID string, date time.Time, actualMax float64) error {
	dateStr := date.Format("2006-01-02")
	_, err := s.db.Exec(`
		UPDATE nowcast_log SET actual_max = ? WHERE station_id = ? AND SUBSTR(date, 1, 10) = ?
	`, actualMax, stationID, dateStr)
	return err
}

func (s *Store) GetMorningObservations(stationID string, date time.Time) ([]models.Observation, error) {
	loc, _ := time.LoadLocation("Australia/Melbourne")
	localDate := date.In(loc)
	morningStart := time.Date(localDate.Year(), localDate.Month(), localDate.Day(), 9, 0, 0, 0, loc)
	morningEnd := time.Date(localDate.Year(), localDate.Month(), localDate.Day(), 11, 0, 0, 0, loc)

	return s.GetObservations(stationID, morningStart.UTC(), morningEnd.UTC())
}

func (s *Store) UpsertDailySummaryWithRegimes(ds models.DailySummary) error {
	_, err := s.db.Exec(`
		INSERT INTO daily_summaries (date, station_id, temp_max, temp_max_time, temp_min, temp_min_time, 
		    temp_avg, humidity_avg, pressure_avg, precip_total, wind_max_gust, 
		    inversion_detected, inversion_strength, regime_heatwave, regime_inversion, regime_clear_calm)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(date, station_id) DO UPDATE SET
			temp_max = excluded.temp_max,
			temp_max_time = excluded.temp_max_time,
			temp_min = excluded.temp_min,
			temp_min_time = excluded.temp_min_time,
			temp_avg = excluded.temp_avg,
			humidity_avg = excluded.humidity_avg,
			pressure_avg = excluded.pressure_avg,
			precip_total = excluded.precip_total,
			wind_max_gust = excluded.wind_max_gust,
			inversion_detected = excluded.inversion_detected,
			inversion_strength = excluded.inversion_strength,
			regime_heatwave = excluded.regime_heatwave,
			regime_inversion = excluded.regime_inversion,
			regime_clear_calm = excluded.regime_clear_calm
	`, ds.Date, ds.StationID, ds.TempMax, ds.TempMaxTime, ds.TempMin, ds.TempMinTime,
		ds.TempAvg, ds.HumidityAvg, ds.PressureAvg, ds.PrecipTotal, ds.WindMaxGust,
		ds.InversionDetected, ds.InversionStrength, ds.RegimeHeatwave, ds.RegimeInversion, ds.RegimeClearCalm)
	return err
}

func (s *Store) GetCorrectionStatsForRegime(source, target string, dayOfForecast int, regime string) (*CorrectionStats, error) {
	row := s.db.QueryRow(`
		SELECT source, target, day_of_forecast, regime, window_days, sample_size, mean_bias, mae, updated_at
		FROM forecast_correction_stats
		WHERE source = ? AND target = ? AND day_of_forecast = ? AND regime = ?
	`, source, target, dayOfForecast, regime)

	var stats CorrectionStats
	err := row.Scan(&stats.Source, &stats.Target, &stats.DayOfForecast, &stats.Regime,
		&stats.WindowDays, &stats.SampleSize, &stats.MeanBias, &stats.MAE, &stats.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &stats, nil
}
