package store

import (
	"database/sql"
	"time"
)

// IngestRun represents a single API fetch operation for auditing.
type IngestRun struct {
	ID                int64
	StartedAt         time.Time
	FinishedAt        sql.NullTime
	Source            string // "wu", "bom"
	Endpoint          string // "pws/observations/current", "forecast/daily/5day", etc.
	StationID         sql.NullString
	LocationID        sql.NullString
	HTTPStatus        sql.NullInt64
	ResponseSizeBytes sql.NullInt64
	RecordsParsed     sql.NullInt64
	RecordsStored     sql.NullInt64
	ParseErrors       sql.NullInt64 // Number of records that failed to parse
	Success           bool
	ErrorMessage      sql.NullString
}

// StartIngestRun creates a new ingest run record and returns it.
func (s *Store) StartIngestRun(source, endpoint string, stationID, locationID *string) (*IngestRun, error) {
	run := &IngestRun{
		StartedAt: time.Now().UTC(),
		Source:    source,
		Endpoint:  endpoint,
	}
	if stationID != nil {
		run.StationID = sql.NullString{String: *stationID, Valid: true}
	}
	if locationID != nil {
		run.LocationID = sql.NullString{String: *locationID, Valid: true}
	}

	result, err := s.db.Exec(`
		INSERT INTO ingest_runs (started_at, source, endpoint, station_id, location_id, success)
		VALUES (?, ?, ?, ?, ?, FALSE)
	`, run.StartedAt, run.Source, run.Endpoint, run.StationID, run.LocationID)
	if err != nil {
		return nil, err
	}

	run.ID, err = result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return run, nil
}

// CompleteIngestRun updates the ingest run with results.
func (s *Store) CompleteIngestRun(run *IngestRun) error {
	if run == nil {
		return nil
	}

	run.FinishedAt = sql.NullTime{Time: time.Now().UTC(), Valid: true}

	_, err := s.db.Exec(`
		UPDATE ingest_runs SET
			finished_at = ?,
			http_status = ?,
			response_size_bytes = ?,
			records_parsed = ?,
			records_stored = ?,
			parse_errors = ?,
			success = ?,
			error_message = ?
		WHERE id = ?
	`, run.FinishedAt, run.HTTPStatus, run.ResponseSizeBytes, run.RecordsParsed,
		run.RecordsStored, run.ParseErrors, run.Success, run.ErrorMessage, run.ID)
	return err
}

// IngestHealthSummary represents a daily ingest health summary.
type IngestHealthSummary struct {
	Date            string
	Source          string
	Endpoint        string
	TotalRuns       int
	SuccessRuns     int
	FailedRuns      int
	TotalRecords    int64
	TotalParseErrors int64
}

// GetIngestHealth returns ingest health summaries for the last N days.
func (s *Store) GetIngestHealth(days int) ([]IngestHealthSummary, error) {
	rows, err := s.db.Query(`
		SELECT 
			DATE(SUBSTR(started_at, 1, 19)) as date,
			source,
			endpoint,
			COUNT(*) as total_runs,
			SUM(CASE WHEN success THEN 1 ELSE 0 END) as success_runs,
			SUM(CASE WHEN NOT success THEN 1 ELSE 0 END) as failed_runs,
			COALESCE(SUM(records_stored), 0) as total_records,
			COALESCE(SUM(parse_errors), 0) as total_parse_errors
		FROM ingest_runs
		WHERE SUBSTR(started_at, 1, 19) > datetime('now', '-' || ? || ' days')
		GROUP BY date, source, endpoint
		ORDER BY date DESC, source, endpoint
	`, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []IngestHealthSummary
	for rows.Next() {
		var h IngestHealthSummary
		if err := rows.Scan(&h.Date, &h.Source, &h.Endpoint, &h.TotalRuns,
			&h.SuccessRuns, &h.FailedRuns, &h.TotalRecords, &h.TotalParseErrors); err != nil {
			return nil, err
		}
		results = append(results, h)
	}
	return results, rows.Err()
}

// GetRecentIngestErrors returns recent failed ingest runs.
func (s *Store) GetRecentIngestErrors(limit int) ([]IngestRun, error) {
	rows, err := s.db.Query(`
		SELECT id, started_at, finished_at, source, endpoint, station_id, location_id,
			   http_status, response_size_bytes, records_parsed, records_stored, 
			   success, error_message
		FROM ingest_runs
		WHERE success = FALSE
		ORDER BY started_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []IngestRun
	for rows.Next() {
		var r IngestRun
		if err := rows.Scan(&r.ID, &r.StartedAt, &r.FinishedAt, &r.Source, &r.Endpoint,
			&r.StationID, &r.LocationID, &r.HTTPStatus, &r.ResponseSizeBytes,
			&r.RecordsParsed, &r.RecordsStored, &r.Success, &r.ErrorMessage); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
