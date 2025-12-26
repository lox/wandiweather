package store

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

type migration struct {
	Version     int
	Description string
	SQL         string
}

var migrations = []migration{
	{
		Version:     1,
		Description: "Initial schema",
		SQL: `
CREATE TABLE IF NOT EXISTS stations (
    station_id TEXT PRIMARY KEY,
    name TEXT,
    latitude REAL,
    longitude REAL,
    elevation REAL,
    elevation_tier TEXT,
    is_primary BOOLEAN DEFAULT FALSE,
    active BOOLEAN DEFAULT TRUE
);

CREATE TABLE IF NOT EXISTS observations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    station_id TEXT NOT NULL,
    observed_at DATETIME NOT NULL,
    temp REAL,
    humidity INTEGER,
    dewpoint REAL,
    pressure REAL,
    wind_speed REAL,
    wind_gust REAL,
    wind_dir INTEGER,
    precip_rate REAL,
    precip_total REAL,
    solar_radiation REAL,
    uv REAL,
    heat_index REAL,
    wind_chill REAL,
    qc_status INTEGER,
    raw_json TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(station_id, observed_at)
);

CREATE TABLE IF NOT EXISTS forecasts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    fetched_at DATETIME NOT NULL,
    valid_date DATE NOT NULL,
    day_of_forecast INTEGER,
    temp_max REAL,
    temp_min REAL,
    humidity INTEGER,
    precip_chance INTEGER,
    precip_amount REAL,
    wind_speed REAL,
    wind_dir TEXT,
    narrative TEXT,
    raw_json TEXT,
    UNIQUE(fetched_at, valid_date)
);

CREATE TABLE IF NOT EXISTS daily_summaries (
    date DATE NOT NULL,
    station_id TEXT NOT NULL,
    temp_max REAL,
    temp_max_time DATETIME,
    temp_min REAL,
    temp_min_time DATETIME,
    temp_avg REAL,
    humidity_avg INTEGER,
    pressure_avg REAL,
    precip_total REAL,
    wind_max_gust REAL,
    inversion_detected BOOLEAN,
    inversion_strength REAL,
    PRIMARY KEY (date, station_id)
);

CREATE TABLE IF NOT EXISTS forecast_verification (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    forecast_id INTEGER REFERENCES forecasts(id),
    valid_date DATE,
    forecast_temp_max REAL,
    forecast_temp_min REAL,
    actual_temp_max REAL,
    actual_temp_min REAL,
    bias_temp_max REAL,
    bias_temp_min REAL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_obs_station_time ON observations(station_id, observed_at);
CREATE INDEX IF NOT EXISTS idx_obs_time ON observations(observed_at);
CREATE INDEX IF NOT EXISTS idx_forecasts_valid ON forecasts(valid_date);
`,
	},
	{
		Version:     2,
		Description: "Add source field to forecasts for BOM vs WU distinction",
		SQL: `
CREATE TABLE forecasts_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL DEFAULT 'wu',
    fetched_at DATETIME NOT NULL,
    valid_date DATE NOT NULL,
    day_of_forecast INTEGER,
    temp_max REAL,
    temp_min REAL,
    humidity INTEGER,
    precip_chance INTEGER,
    precip_amount REAL,
    wind_speed REAL,
    wind_dir TEXT,
    narrative TEXT,
    raw_json TEXT,
    UNIQUE(source, fetched_at, valid_date)
);

INSERT INTO forecasts_new (id, source, fetched_at, valid_date, day_of_forecast, temp_max, temp_min, humidity, precip_chance, precip_amount, wind_speed, wind_dir, narrative, raw_json)
SELECT id, 'wu', fetched_at, valid_date, day_of_forecast, temp_max, temp_min, humidity, precip_chance, precip_amount, wind_speed, wind_dir, narrative, raw_json
FROM forecasts;

DROP TABLE forecasts;

ALTER TABLE forecasts_new RENAME TO forecasts;

CREATE INDEX IF NOT EXISTS idx_forecasts_valid ON forecasts(valid_date);
`,
	},
}

func (s *Store) Migrate() error {
	if err := s.ensureMigrationsTable(); err != nil {
		return fmt.Errorf("ensure migrations table: %w", err)
	}

	applied, err := s.getAppliedMigrations()
	if err != nil {
		return fmt.Errorf("get applied migrations: %w", err)
	}

	for _, m := range migrations {
		if applied[m.Version] {
			continue
		}

		log.Printf("migrations: applying %d - %s", m.Version, m.Description)

		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("begin tx for migration %d: %w", m.Version, err)
		}

		if _, err := tx.Exec(m.SQL); err != nil {
			tx.Rollback()
			return fmt.Errorf("execute migration %d: %w", m.Version, err)
		}

		if _, err := tx.Exec(
			"INSERT INTO schema_migrations (version, description, applied_at) VALUES (?, ?, ?)",
			m.Version, m.Description, time.Now().UTC(),
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", m.Version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.Version, err)
		}

		log.Printf("migrations: completed %d", m.Version)
	}

	return nil
}

func (s *Store) ensureMigrationsTable() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			description TEXT,
			applied_at DATETIME
		)
	`)
	return err
}

func (s *Store) getAppliedMigrations() (map[int]bool, error) {
	rows, err := s.db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}
	return applied, rows.Err()
}

func (s *Store) MigrationVersion() (int, error) {
	var version sql.NullInt64
	err := s.db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version)
	if err != nil {
		return 0, err
	}
	if !version.Valid {
		return 0, nil
	}
	return int(version.Int64), nil
}
