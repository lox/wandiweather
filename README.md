# WandiWeather

Hyperlocal weather service for Wandiligong/Bright, Victoria. Aggregates data from local Personal Weather Stations (PWS) via Weather Underground API.

## Features

- Real-time conditions from 4 local stations
- 5-day forecast from Weather Underground
- Forecast accuracy tracking and verification
- Simple HTMX-powered dashboard

## Quick Start

```bash
# Set your Weather Underground API key
export PWS_API_KEY=your_key_here

# Run with polling (production)
go run ./cmd/wandiweather --db data/wandiweather.db

# Run without polling (local dev)
go run ./cmd/wandiweather --db data/wandiweather.db --no-poll
```

Visit http://localhost:8080

## CLI Flags

| Flag | Description |
|------|-------------|
| `--db` | Path to SQLite database (default: `data/wandiweather.db`) |
| `--port` | HTTP server port (default: `8080`) |
| `--no-poll` | Disable API polling (server only) |
| `--once` | Ingest once and exit |
| `--daily` | Run daily jobs and exit |
| `--backfill-daily` | Backfill all daily summaries |

## Architecture

```
cmd/wandiweather/     # Entry point
internal/
  api/                # HTTP handlers + templates
  ingest/             # PWS and forecast ingestion
  models/             # Data structures
  store/              # SQLite storage + migrations
data/                 # SQLite database
```

## Deployment

Deployed on Fly.io. The Dockerfile uses Hermit for Go toolchain management.

```bash
fly deploy
```
