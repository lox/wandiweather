# WandiWeather

Hyperlocal weather service for Wandiligong/Bright, Victoria. Aggregates data from local Personal Weather Stations (PWS) via Weather Underground API.

> 🤖  **Note:** This project was ["vibe engineered"](https://simonwillison.net/2025/Oct/7/vibe-engineering/) with [Amp](https://ampcode.com) and Claude Opus 4.5 and others as part of my ongoing effort to demonstrate that AI-assisted development can produce high-quality software when paired with rigorous design documentation, comprehensive tests, and careful human review.

## Features

- Real-time conditions from 4 local stations
- 5-day forecast from Weather Underground + BOM
- Forecast accuracy tracking and verification
- Bias correction with regime-aware adjustments
- ML-ready data collection (audit trail, raw payload storage)
- Simple HTMX-powered dashboard

## Quick Start

Requires [mise](https://mise.jdx.dev/) for toolchain and task management.

```bash
# Install the pinned toolchain
mise trust
mise install

# Set your Weather Underground API key
export PWS_API_KEY=your_key_here

# Run local dev server with hot reload
mise run dev
```

Visit http://localhost:8080

## Tasks

Tasks are defined in `mise.toml`:

| Command | Description |
|---------|-------------|
| `mise run dev` | Run local dev server with hot reload (no API polling) |
| `mise run run` | Run server with polling enabled |
| `mise run build` | Build the binary |
| `mise run test` | Run all tests |
| `mise run lint` | Run linter |
| `mise run check` | Run build, test, and lint |
| `mise run once` | Run single ingestion and exit |
| `mise run daily` | Run daily jobs manually |
| `mise run pull-db` | Pull production database from Fly.io |

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
  ingest/             # PWS and forecast ingestion + scheduling
  forecast/           # Bias correction, regimes, nowcast
  models/             # Data structures
  store/              # SQLite storage + migrations
data/                 # SQLite database
docs/plans/           # Implementation plans
```

## Deployment

Deployed on Fly.io. The Dockerfile uses mise for Go toolchain management.

```bash
mise run deploy
```
