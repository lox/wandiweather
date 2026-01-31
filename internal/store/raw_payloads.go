package store

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"time"
)

// RawPayload represents a stored API response payload.
type RawPayload struct {
	ID                int64
	IngestRunID       sql.NullInt64
	FetchedAt         time.Time
	Source            string
	Endpoint          string
	StationID         sql.NullString
	LocationID        sql.NullString
	PayloadCompressed []byte
	PayloadHash       string
	SchemaVersion     int
}

// StoreRawPayload stores a compressed API response payload.
// Returns the payload ID, or 0 if the payload was a duplicate (same hash).
func (s *Store) StoreRawPayload(runID *int64, source, endpoint string,
	stationID, locationID *string, payload []byte) (int64, error) {

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(payload); err != nil {
		return 0, fmt.Errorf("compress payload: %w", err)
	}
	if err := gz.Close(); err != nil {
		return 0, fmt.Errorf("close gzip: %w", err)
	}
	compressed := buf.Bytes()

	hash := sha256.Sum256(payload)
	hashHex := hex.EncodeToString(hash[:])

	var ingestRunID sql.NullInt64
	if runID != nil {
		ingestRunID = sql.NullInt64{Int64: *runID, Valid: true}
	}

	var stationIDNull, locationIDNull sql.NullString
	if stationID != nil {
		stationIDNull = sql.NullString{String: *stationID, Valid: true}
	}
	if locationID != nil {
		locationIDNull = sql.NullString{String: *locationID, Valid: true}
	}

	result, err := s.db.Exec(`
		INSERT INTO raw_payloads 
		(ingest_run_id, fetched_at, source, endpoint, station_id, location_id, 
		 payload_compressed, payload_hash, schema_version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1)
		ON CONFLICT(payload_hash) DO NOTHING
	`, ingestRunID, time.Now().UTC(), source, endpoint, stationIDNull, locationIDNull,
		compressed, hashHex)
	if err != nil {
		return 0, fmt.Errorf("insert raw payload: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	return id, nil
}

// GetRawPayload retrieves and decompresses a stored payload by ID.
func (s *Store) GetRawPayload(id int64) ([]byte, error) {
	var compressed []byte
	err := s.db.QueryRow(`SELECT payload_compressed FROM raw_payloads WHERE id = ?`, id).
		Scan(&compressed)
	if err != nil {
		return nil, err
	}

	gz, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("create gzip reader: %w", err)
	}
	defer gz.Close()

	return io.ReadAll(gz)
}

// GetRawPayloadByHash retrieves a payload by its hash (for deduplication checks).
func (s *Store) GetRawPayloadByHash(hash string) (*RawPayload, error) {
	row := s.db.QueryRow(`
		SELECT id, ingest_run_id, fetched_at, source, endpoint, station_id, location_id,
		       payload_compressed, payload_hash, schema_version
		FROM raw_payloads WHERE payload_hash = ?
	`, hash)

	var p RawPayload
	err := row.Scan(&p.ID, &p.IngestRunID, &p.FetchedAt, &p.Source, &p.Endpoint,
		&p.StationID, &p.LocationID, &p.PayloadCompressed, &p.PayloadHash, &p.SchemaVersion)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// RawPayloadStats contains storage statistics for raw payloads.
type RawPayloadStats struct {
	TotalCount       int
	TotalSizeBytes   int64
	OldestFetchedAt  time.Time
	NewestFetchedAt  time.Time
	CountBySource    map[string]int
	SizeBySource     map[string]int64
}

// GetRawPayloadStats returns storage statistics for raw payloads.
func (s *Store) GetRawPayloadStats() (*RawPayloadStats, error) {
	stats := &RawPayloadStats{
		CountBySource: make(map[string]int),
		SizeBySource:  make(map[string]int64),
	}

	row := s.db.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(LENGTH(payload_compressed)), 0),
		       MIN(fetched_at), MAX(fetched_at)
		FROM raw_payloads
	`)
	var oldest, newest sql.NullTime
	if err := row.Scan(&stats.TotalCount, &stats.TotalSizeBytes, &oldest, &newest); err != nil {
		return nil, err
	}
	if oldest.Valid {
		stats.OldestFetchedAt = oldest.Time
	}
	if newest.Valid {
		stats.NewestFetchedAt = newest.Time
	}

	rows, err := s.db.Query(`
		SELECT source, COUNT(*), SUM(LENGTH(payload_compressed))
		FROM raw_payloads
		GROUP BY source
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var source string
		var count int
		var size int64
		if err := rows.Scan(&source, &count, &size); err != nil {
			return nil, err
		}
		stats.CountBySource[source] = count
		stats.SizeBySource[source] = size
	}

	return stats, rows.Err()
}

// CleanupOldRawPayloads deletes raw payloads older than the specified number of days.
// Returns the number of deleted records.
func (s *Store) CleanupOldRawPayloads(retentionDays int) (int64, error) {
	result, err := s.db.Exec(`
		DELETE FROM raw_payloads
		WHERE fetched_at < DATE('now', '-' || ? || ' days')
	`, retentionDays)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
