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

## Project Structure

- `cmd/wandiweather/main.go` - Entry point, station config, CLI flags
- `internal/api/server.go` - HTTP handlers
- `internal/api/templates/` - HTML templates (HTMX)
- `internal/ingest/` - Weather data ingestion (PWS, forecasts)
- `internal/ingest/daily.go` - Daily jobs (summaries, verification, regimes)
- `internal/forecast/` - Forecast correction (bias, regimes, nowcast)
- `internal/store/` - SQLite storage and migrations
- `internal/models/` - Data structures
- `docs/plans/` - Implementation plans

## Conventions

- Use stdlib where possible (net/http, html/template, database/sql)
- Templates use HTMX for interactivity
- Migrations are numbered in `internal/store/migrations.go`
- Stations defined in `cmd/wandiweather/main.go`

## Environment

- `PWS_API_KEY` - Weather Underground API key (required)

## Database

SQLite with WAL mode. Schema managed via migrations.

```bash
# Check current data
sqlite3 data/wandiweather.db "SELECT * FROM stations WHERE active = 1"
```
