# Agent Instructions

## Commands

Uses [Task](https://taskfile.dev) (installed via Hermit):

```bash
task dev       # Run local dev server with hot reload (no API polling)
task run       # Run server with polling enabled
task build     # Build the binary
task test      # Run all tests
task lint      # Run linter (go vet + staticcheck)
task check     # Run build, test, and lint
task once      # Run single ingestion and exit
task daily     # Run daily jobs manually
task pull-db   # Pull production database from Fly.io
```

## Production Commands

```bash
# Deploy to Fly.io
fly deploy

# Run backfill on prod (must specify --db path)
fly ssh console -C "/app/wandiweather --db /data/wandiweather.db --backfill-daily"

# Run daily jobs on prod
fly ssh console -C "/app/wandiweather --db /data/wandiweather.db --daily"
```

## Project Structure

- `cmd/wandiweather/main.go` - Entry point, station config, CLI flags
- `internal/api/server.go` - HTTP handlers
- `internal/api/templates/` - HTML templates (HTMX)
- `internal/ingest/` - Weather data ingestion (PWS, forecasts, BOM)
- `internal/ingest/pws.go` - PWS current observations and history
- `internal/ingest/forecast.go` - WU forecast fetching (`Fetch5Day`)
- `internal/ingest/bom.go` - BOM forecast fetching via FTP
- `internal/ingest/daily.go` - Daily jobs (summaries, verification, cleanup)
- `internal/ingest/scheduler.go` - Scheduler with cron-based forecast timing
- `internal/forecast/` - Forecast correction (bias, regimes, nowcast)
- `internal/store/` - SQLite storage and migrations
- `internal/store/ingest.go` - Ingest audit trail (`ingest_runs` table)
- `internal/store/raw_payloads.go` - Raw API response storage
- `internal/models/` - Data structures
- `docs/plans/` - Implementation plans

## Key Tables

| Table | Purpose |
|-------|---------|
| `observations` | PWS readings with `obs_type` and `quality_flags` for ML filtering |
| `forecasts` | WU and BOM forecasts with `source` and `location_id` |
| `daily_summaries` | Computed daily stats with regime flags |
| `forecast_verification` | Bias tracking per forecast |
| `ingest_runs` | Audit trail for all API fetches |
| `raw_payloads` | Compressed raw API responses (90-day retention) |

## Conventions

- Use stdlib where possible (net/http, html/template, database/sql)
- Templates use HTMX for interactivity
- Migrations are numbered in `internal/store/migrations.go` (currently v23)
- Stations defined in `cmd/wandiweather/main.go`
- All ingest operations log to `ingest_runs` for auditing
- Raw API payloads stored compressed for ML training/debugging

## Scheduling

Forecast fetching uses `robfig/cron` for fixed-time collection (Melbourne timezone):

| Time | Purpose |
|------|---------|
| 5am  | **Critical**: Captures full day-0 forecast with temp_min before sunrise |
| 11am | Mid-morning update |
| 5pm  | Evening update |
| 11pm | Night update |
| 6am  | Daily jobs (summaries, verification) |

Observations poll every 5 minutes. Alerts poll every 5 minutes. Fire danger polls every 30 minutes.

## Data Collection (ML-Ready)

The system is designed for future ML-based forecast correction:

1. **Audit Trail**: Every API fetch logged with HTTP status, response size, record counts
2. **Raw Payloads**: Compressed JSON/XML stored for re-parsing if needed
3. **Observation Types**: `obs_type` column distinguishes instant vs aggregated readings
4. **Parse Errors**: Tracked separately from fatal errors for data quality monitoring
5. **Location IDs**: Forecasts tagged with geocode (WU) or AAC code (BOM) for multi-location
6. **Quality Flags**: Observations validated and flagged for out-of-range values

See `docs/plans/ml-data-collection.md` for the full plan (Phases 1-6 complete).

## Environment

- `PWS_API_KEY` - Weather Underground API key (required)

## Database

SQLite with WAL mode. Schema managed via migrations.

```bash
# Check current schema version
sqlite3 data/wandiweather.db "SELECT MAX(version) FROM schema_migrations"

# Check ingest health (last 24h)
sqlite3 data/wandiweather.db "SELECT source, endpoint, COUNT(*), SUM(success) FROM ingest_runs WHERE started_at > datetime('now', '-1 day') GROUP BY source, endpoint"

# Check raw payload storage
sqlite3 data/wandiweather.db "SELECT source, COUNT(*), SUM(LENGTH(payload_compressed))/1024 as kb FROM raw_payloads GROUP BY source"
```
