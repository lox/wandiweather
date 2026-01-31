# ML-Ready Data Collection Plan

> **Created**: January 31, 2026  
> **Status**: Phases 1-6 complete (schema at v23)  
> **Goal**: Make data collection "rock solid" for future ML-based forecast correction

## Overview

Before implementing ML models for forecast correction, we need to ensure data collection is auditable, complete, and semantically consistent. This plan addresses gaps identified in the current ingestion pipeline that would undermine ML training.

**Current state**: Heuristic-based correction (bias + regimes + nowcast) with ~30 days of data  
**Target state**: Clean, reprocessable dataset suitable for ML model training

---

## Problem Summary

| Issue | Impact on ML | Priority |
|-------|--------------|----------|
| Raw JSON payloads discarded | Cannot reparse/backfill when bugs found | Critical |
| No ingest audit trail | Can't distinguish "no data" from "ingest failed" | Critical |
| Mixed observation semantics (instant vs aggregate) | Pollutes training labels | High |
| Silent parse failures | Biased/incomplete training sets | High |
| No location ID on forecasts | Can't scale to multiple locations | Medium |
| `Fetch7Day()` calls 5-day endpoint | Naming confusion | Low |

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        Data Collection Pipeline                          │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  API Response ──▶ Raw Payload Store ──▶ Parse ──▶ Derived Tables        │
│       │                  │                            │                  │
│       │                  ▼                            ▼                  │
│       │          raw_payloads              observations, forecasts       │
│       │          (compressed JSON)         daily_summaries               │
│       │                                                                  │
│       └──────────────▶ ingest_runs (audit log)                          │
│                                                                          │
│  Benefits:                                                               │
│  - Reparse history when bugs fixed                                       │
│  - Add new derived fields without re-fetching                            │
│  - Debug data gaps via audit log                                         │
│  - ML training uses consistent, versioned derivations                    │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Phase 1: Ingest Audit Trail

### 1.1 Schema

```sql
CREATE TABLE ingest_runs (
    id INTEGER PRIMARY KEY,
    started_at DATETIME NOT NULL,
    finished_at DATETIME,
    source TEXT NOT NULL,           -- 'wu', 'bom'
    endpoint TEXT NOT NULL,         -- 'pws/observations/current', 'forecast/daily/5day', etc.
    station_id TEXT,                -- For PWS endpoints
    location_id TEXT,               -- For forecast endpoints (geocode or BOM location)
    http_status INTEGER,
    response_size_bytes INTEGER,
    records_parsed INTEGER,
    records_stored INTEGER,
    success BOOLEAN NOT NULL,
    error_message TEXT,
    UNIQUE(started_at, source, endpoint, station_id)
);

CREATE INDEX idx_ingest_runs_source_date ON ingest_runs(source, started_at);
```

### 1.2 Implementation

Wrap each fetch call with audit logging:

```go
// internal/ingest/audit.go

type IngestRun struct {
    ID             int64
    StartedAt      time.Time
    FinishedAt     sql.NullTime
    Source         string
    Endpoint       string
    StationID      sql.NullString
    LocationID     sql.NullString
    HTTPStatus     sql.NullInt64
    ResponseSize   sql.NullInt64
    RecordsParsed  sql.NullInt64
    RecordsStored  sql.NullInt64
    Success        bool
    ErrorMessage   sql.NullString
}

func (s *Store) StartIngestRun(source, endpoint string, stationID, locationID *string) (*IngestRun, error)
func (s *Store) CompleteIngestRun(run *IngestRun, success bool, err error) error
```

Update `pws.go` and `forecast.go` to use audit wrapper:

```go
func (c *PWSClient) FetchCurrent(stationID string) (*Observation, error) {
    run, _ := c.store.StartIngestRun("wu", "pws/observations/current", &stationID, nil)
    defer func() {
        c.store.CompleteIngestRun(run, run.Success, nil)
    }()
    
    // ... existing fetch logic ...
    run.HTTPStatus = sql.NullInt64{Int64: int64(resp.StatusCode), Valid: true}
    run.ResponseSize = sql.NullInt64{Int64: int64(len(body)), Valid: true}
    // ...
}
```

### 1.3 Observability Queries

```sql
-- Daily ingest health check
SELECT 
    DATE(started_at) as date,
    source,
    endpoint,
    COUNT(*) as runs,
    SUM(CASE WHEN success THEN 1 ELSE 0 END) as successes,
    SUM(records_stored) as total_records
FROM ingest_runs
WHERE started_at > DATE('now', '-7 days')
GROUP BY date, source, endpoint
ORDER BY date DESC, source, endpoint;

-- Find gaps
SELECT DATE(started_at) as date, source
FROM ingest_runs
GROUP BY date, source
HAVING SUM(CASE WHEN success THEN 1 ELSE 0 END) = 0;
```

---

## Phase 2: Raw Payload Storage

### 2.1 Schema

```sql
CREATE TABLE raw_payloads (
    id INTEGER PRIMARY KEY,
    ingest_run_id INTEGER NOT NULL REFERENCES ingest_runs(id),
    fetched_at DATETIME NOT NULL,
    source TEXT NOT NULL,           -- 'wu', 'bom'
    endpoint TEXT NOT NULL,
    station_id TEXT,
    location_id TEXT,
    payload_compressed BLOB NOT NULL,  -- gzip compressed JSON
    payload_hash TEXT NOT NULL,         -- SHA256 for dedup
    schema_version INTEGER DEFAULT 1,   -- Bump when parse logic changes
    UNIQUE(payload_hash)
);

CREATE INDEX idx_raw_payloads_source_date ON raw_payloads(source, fetched_at);
CREATE INDEX idx_raw_payloads_ingest_run ON raw_payloads(ingest_run_id);
```

### 2.2 Implementation

```go
// internal/store/raw_payloads.go

import (
    "bytes"
    "compress/gzip"
    "crypto/sha256"
    "encoding/hex"
)

func (s *Store) StoreRawPayload(runID int64, source, endpoint string, 
    stationID, locationID *string, payload []byte) (int64, error) {
    
    // Compress payload
    var buf bytes.Buffer
    gz := gzip.NewWriter(&buf)
    gz.Write(payload)
    gz.Close()
    compressed := buf.Bytes()
    
    // Hash for dedup
    hash := sha256.Sum256(payload)
    hashHex := hex.EncodeToString(hash[:])
    
    // Insert (ignore duplicates)
    result, err := s.db.Exec(`
        INSERT OR IGNORE INTO raw_payloads 
        (ingest_run_id, fetched_at, source, endpoint, station_id, location_id, 
         payload_compressed, payload_hash, schema_version)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1)
    `, runID, time.Now().UTC(), source, endpoint, stationID, locationID, 
       compressed, hashHex)
    
    return result.LastInsertId()
}

func (s *Store) GetRawPayload(id int64) ([]byte, error) {
    var compressed []byte
    err := s.db.QueryRow(`SELECT payload_compressed FROM raw_payloads WHERE id = ?`, id).
        Scan(&compressed)
    if err != nil {
        return nil, err
    }
    
    gz, _ := gzip.NewReader(bytes.NewReader(compressed))
    defer gz.Close()
    return io.ReadAll(gz)
}
```

### 2.3 Update Fetch Functions

In `forecast.go`:

```go
func (c *ForecastClient) Fetch7Day(geocode string) ([]Forecast, error) {
    // ... fetch ...
    
    // Store raw payload BEFORE parsing
    payloadID, _ := c.store.StoreRawPayload(run.ID, "wu", "forecast/daily/5day",
        nil, &geocode, body)
    
    // Parse with error tracking
    forecasts, parseErrors := c.parseForecasts(body)
    run.RecordsParsed = sql.NullInt64{Int64: int64(len(forecasts)), Valid: true}
    
    if len(parseErrors) > 0 {
        log.Printf("Parse errors for payload %d: %v", payloadID, parseErrors)
    }
    
    // ... store forecasts ...
}
```

### 2.4 Storage Estimates

| Source | Frequency | Raw Size | Compressed | Daily | Monthly |
|--------|-----------|----------|------------|-------|---------|
| WU PWS current | 5 min | ~2 KB | ~400 B | ~115 KB | ~3.5 MB |
| WU PWS history | hourly | ~5 KB | ~1 KB | ~24 KB | ~720 KB |
| WU forecast | 3 hours | ~15 KB | ~3 KB | ~24 KB | ~720 KB |
| BOM forecast | 3 hours | ~10 KB | ~2 KB | ~16 KB | ~480 KB |
| **Total** | | | | ~180 KB | ~5.4 MB |

Retention: Keep 90 days of raw payloads, then archive to cold storage or delete.

---

## Phase 3: Observation Semantics

### 3.1 Problem

Current `Observation` model mixes:
- **Instant** readings from `FetchCurrent()` 
- **Hourly aggregates** from `fetchHistory()` (e.g., `HumidityAvg`, `PressureMax`)

This ambiguity corrupts ML labels that expect consistent semantics.

### 3.2 Solution: Add Observation Type

```sql
-- Migration
ALTER TABLE observations ADD COLUMN obs_type TEXT DEFAULT 'instant';
-- Values: 'instant', 'hourly_aggregate', 'daily_aggregate'

ALTER TABLE observations ADD COLUMN aggregation_period_minutes INTEGER;
-- NULL for instant, 60 for hourly, 1440 for daily
```

Update model:

```go
type Observation struct {
    // ... existing fields ...
    
    ObsType             string        // 'instant', 'hourly_aggregate', 'daily_aggregate'
    AggregationPeriod   sql.NullInt64 // Minutes (60 for hourly, 1440 for daily)
}
```

### 3.3 Update Ingest

In `pws.go`:

```go
func (c *PWSClient) FetchCurrent(stationID string) (*Observation, error) {
    // ... existing logic ...
    obs.ObsType = "instant"
    obs.AggregationPeriod = sql.NullInt64{} // NULL
    return obs, nil
}

func (c *PWSClient) fetchHistory(stationID string, date time.Time) ([]Observation, error) {
    // ... existing logic ...
    for _, h := range history {
        obs := Observation{
            // ... existing mapping ...
            ObsType:           "hourly_aggregate",
            AggregationPeriod: sql.NullInt64{Int64: 60, Valid: true},
        }
        observations = append(observations, obs)
    }
    return observations, nil
}
```

### 3.4 ML Training Queries

```sql
-- Get only instant observations for daily max/min calculation
SELECT DATE(observed_at) as date,
       MAX(temp) as actual_max,
       MIN(temp) as actual_min
FROM observations
WHERE station_id = ? 
  AND obs_type = 'instant'
  AND temp IS NOT NULL
GROUP BY date;

-- Or use hourly aggregates with explicit understanding
SELECT DATE(observed_at) as date,
       MAX(temp) as actual_max  -- This is max of hourly averages
FROM observations
WHERE station_id = ?
  AND obs_type = 'hourly_aggregate'
GROUP BY date;
```

---

## Phase 4: Parse Error Handling

### 4.1 Problem

Current code silently skips unparseable records:

```go
validTime, err := time.Parse("2006-01-02T15:04:05-0700", data.ValidTimeLocal[i])
if err != nil {
    continue  // Silent skip!
}
```

### 4.2 Solution: Log and Count Errors

```go
type ParseError struct {
    Field    string
    Index    int
    RawValue string
    Error    string
}

func (c *ForecastClient) parseForecasts(body []byte) ([]Forecast, []ParseError) {
    var forecasts []Forecast
    var errors []ParseError
    
    // ... parse JSON ...
    
    for i := range data.ValidTimeLocal {
        validTime, err := time.Parse("2006-01-02T15:04:05-0700", data.ValidTimeLocal[i])
        if err != nil {
            errors = append(errors, ParseError{
                Field:    "validTimeLocal",
                Index:    i,
                RawValue: data.ValidTimeLocal[i],
                Error:    err.Error(),
            })
            continue  // Still skip, but now logged
        }
        // ... rest of parsing ...
    }
    
    return forecasts, errors
}
```

Store parse errors in ingest_runs:

```go
run.RecordsParsed = sql.NullInt64{Int64: int64(len(forecasts) + len(errors)), Valid: true}
run.RecordsStored = sql.NullInt64{Int64: int64(len(forecasts)), Valid: true}
if len(errors) > 0 {
    run.ErrorMessage = sql.NullString{
        String: fmt.Sprintf("%d parse errors: %v", len(errors), errors[0]),
        Valid:  true,
    }
}
```

---

## Phase 5: Location/Station ID on Forecasts

### 5.1 Problem

`models.Forecast` has no location identifier. Currently single-location, but ML needs explicit "where" in training data.

### 5.2 Solution

```sql
-- Migration
ALTER TABLE forecasts ADD COLUMN location_id TEXT;
-- For WU: geocode like "-36.89,147.15"
-- For BOM: location code like "VIC_PT042"

CREATE INDEX idx_forecasts_location ON forecasts(location_id, valid_date);
```

Update model and ingest:

```go
type Forecast struct {
    // ... existing fields ...
    LocationID  string  // Geocode or BOM location code
}

func (c *ForecastClient) Fetch7Day(geocode string) ([]Forecast, error) {
    // ... existing logic ...
    for i := range forecasts {
        forecasts[i].LocationID = geocode
    }
    return forecasts, nil
}
```

---

## Phase 6: Data Quality Flags

### 6.1 Add Range Check Flags

Non-destructive validation at ingest time:

```sql
ALTER TABLE observations ADD COLUMN quality_flags TEXT;
-- JSON array: ["temp_out_of_range", "humidity_invalid", "wind_spike"]
```

```go
func validateObservation(obs *Observation) []string {
    var flags []string
    
    if obs.Temp.Valid {
        if obs.Temp.Float64 < -10 || obs.Temp.Float64 > 50 {
            flags = append(flags, "temp_out_of_range")
        }
    }
    if obs.Humidity.Valid {
        if obs.Humidity.Float64 < 0 || obs.Humidity.Float64 > 100 {
            flags = append(flags, "humidity_invalid")
        }
    }
    if obs.WindDir.Valid {
        if obs.WindDir.Float64 < 0 || obs.WindDir.Float64 > 360 {
            flags = append(flags, "wind_dir_invalid")
        }
    }
    // ... more checks ...
    
    return flags
}
```

### 6.2 Use WU QCStatus

Already stored but unused. Add filtering helper:

```go
func (s *Store) GetCleanObservations(stationID string, start, end time.Time) ([]Observation, error) {
    return s.db.Query(`
        SELECT * FROM observations
        WHERE station_id = ?
          AND observed_at BETWEEN ? AND ?
          AND qc_status IN (0, 1)  -- Only good QC
          AND (quality_flags IS NULL OR quality_flags = '[]')
        ORDER BY observed_at
    `, stationID, start, end)
}
```

---

## Phase 7: Naming Fixes

### 7.1 Rename `Fetch7Day` → `Fetch5Day`

The function calls a 5-day endpoint but is named 7-day:

```go
// Before
func (c *ForecastClient) Fetch7Day(geocode string) ([]Forecast, error)

// After
func (c *ForecastClient) Fetch5Day(geocode string) ([]Forecast, error)
```

Update all callers.

---

## Implementation Tasks

### Phase 1: Ingest Audit (Day 1) ✓
- [x] Add `ingest_runs` table migration
- [x] Implement `StartIngestRun()` and `CompleteIngestRun()`
- [x] Update `pws.go` to return `FetchResult` with audit metadata
- [x] Update `forecast.go` to return `FetchResult` (renamed to `Fetch5Day`)
- [x] Update `bom.go` to return `FetchResult`
- [x] Update scheduler to log all ingest runs
- [ ] Add health check query to daily job output

### Phase 2: Raw Payload Storage (Day 1-2) ✓
- [x] Add `raw_payloads` table migration (v17)
- [x] Implement `StoreRawPayload()` and `GetRawPayload()` in `raw_payloads.go`
- [x] Implement `GetRawPayloadStats()` and `CleanupOldRawPayloads()`
- [x] Update WU forecast fetch to store raw JSON
- [x] Update BOM forecast fetch to store raw XML
- [x] Update PWS observations fetch to store raw JSON
- [x] Add retention cleanup job (90 days) in daily jobs

### Phase 3: Observation Semantics (Day 2) ✓
- [x] Add `obs_type` and `aggregation_period_minutes` columns (migration v18)
- [x] Add `ObsType` and `AggregationPeriod` fields to `models.Observation`
- [x] Add constants: `ObsTypeInstant`, `ObsTypeHourlyAggregate`, `ObsTypeUnknown`
- [x] Update `FetchCurrent()` to set `obs_type = 'instant'`
- [x] Update `fetchHistory()` to set `obs_type = 'hourly_aggregate'` with 60-min period
- [x] Update `InsertObservation()`, `GetLatestObservation()`, `GetObservations()`
- [x] Backfill existing data as `obs_type = 'unknown'`

### Phase 4: Parse Error Handling (Day 2) ✓
- [x] Add `ParseErrors` and `ParseError` fields to `FetchResult`
- [x] Update WU and BOM parsing to track parse errors separately from fatal errors
- [x] Add `parse_errors` column to `ingest_runs` (migration v19)
- [x] Update scheduler to log parse errors to audit trail
- [x] Add `LogIngestHealth()` to daily jobs for health reporting

### Phase 5: Location ID (Day 2) ✓
- [x] Add `location_id` column to forecasts (migration v20)
- [x] Update forecast ingest to populate location (WU: geocode, BOM: AAC code)
- [x] Backfill existing forecasts (WU: `-36.794,146.977`, BOM: `VIC_PT075`)

### Phase 6: Data Quality Flags (Day 3) ✓
- [x] Add `quality_flags` column to observations (migration v21)
- [x] Implement `ValidateObservation()` in `internal/ingest/validate.go`
- [x] Add validation to ingest pipeline (PWS current and history)
- [x] Create `GetCleanObservations()` helper in store

### Phase 7: Naming Fix (Day 3) ✓
- [x] Renamed `Fetch7Day` → `Fetch5Day` (done in Phase 1)
- [x] All callers updated

### Phase 8: Observation Type Inference (Day 3) ✓
- [x] Migration v22 placeholder (timestamp format issue)
- [x] Migration v23 to infer obs_type from timestamp patterns
- [x] Hourly-aligned observations marked as `hourly_aggregate`
- [x] Non-aligned observations marked as `instant`

---

## Verification

After implementation, verify with:

```sql
-- Check ingest coverage
SELECT DATE(started_at), source, endpoint, 
       SUM(CASE WHEN success THEN 1 ELSE 0 END) as ok,
       SUM(CASE WHEN NOT success THEN 1 ELSE 0 END) as failed
FROM ingest_runs
WHERE started_at > DATE('now', '-7 days')
GROUP BY DATE(started_at), source, endpoint;

-- Check raw payload storage
SELECT DATE(fetched_at), source, COUNT(*), SUM(LENGTH(payload_compressed))
FROM raw_payloads
WHERE fetched_at > DATE('now', '-7 days')
GROUP BY DATE(fetched_at), source;

-- Check observation types
SELECT obs_type, COUNT(*) 
FROM observations 
GROUP BY obs_type;

-- Check quality flags
SELECT quality_flags, COUNT(*)
FROM observations
WHERE quality_flags IS NOT NULL AND quality_flags != '[]'
GROUP BY quality_flags;
```

---

## Success Criteria

| Metric | Target |
|--------|--------|
| Ingest runs logged | 100% of fetches have audit record |
| Raw payloads stored | 100% of successful fetches |
| Observation type coverage | 100% of observations have obs_type |
| Parse error visibility | 0 silent failures |
| Data quality flags | All observations validated |

---

## Future: ML Training Dataset Export

Once data collection is solid, add a nightly job to export clean training data:

```sql
-- Export for ML training
SELECT 
    f.valid_date,
    f.location_id,
    f.source,
    f.temp_max as forecast_max,
    f.temp_min as forecast_min,
    ds.temp_max as actual_max,
    ds.temp_min as actual_min,
    ds.regime_heatwave,
    ds.regime_inversion,
    ds.regime_clear_calm,
    LAG(ds.temp_max) OVER (ORDER BY f.valid_date) as prev_day_max,
    LAG(ds.temp_min) OVER (ORDER BY f.valid_date) as prev_day_min
FROM forecasts f
JOIN daily_summaries ds ON f.valid_date = ds.date AND f.station_id = ds.station_id
WHERE f.fetched_at < f.valid_date  -- Forecast made before the day
  AND ds.temp_max IS NOT NULL
ORDER BY f.valid_date;
```

This clean dataset can then feed into:
1. Linear regression (current plan Phase 4)
2. Gradient boosting (XGBoost/LightGBM)
3. Neural network approaches

---

## References

- [forecast-correction.md](forecast-correction.md) - Current heuristic approach
- Oracle consultation (Jan 31, 2026) - Data collection review
