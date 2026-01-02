# WandiWeather

Hyperlocal weather service for Wandiligong/Bright, Victoria. Aggregates data from local Personal Weather Stations (PWS) via Weather Underground API.

> 🤖  **Note:** This project was ["vibe engineered"](https://simonwillison.net/2025/Oct/7/vibe-engineering/) with [Amp](https://ampcode.com) and Claude Opus 4.5 and others as part of my ongoing effort to demonstrate that AI-assisted development can produce high-quality software when paired with rigorous design documentation, comprehensive tests, and careful human review.

## Features

- Real-time conditions from 4 local stations
- 5-day forecast from Weather Underground
- Forecast accuracy tracking and verification
- Simple HTMX-powered dashboard

## Quick Start

Requires [Hermit](https://cashapp.github.io/hermit/) for toolchain management.

```bash
# Activate hermit environment
source bin/activate-hermit

# Set your Weather Underground API key
export PWS_API_KEY=your_key_here

# Run local dev server with hot reload
task dev
```

Visit http://localhost:8080

## Tasks

Uses [Task](https://taskfile.dev) (installed via Hermit):

| Command | Description |
|---------|-------------|
| `task dev` | Run local dev server with hot reload (no API polling) |
| `task run` | Run server with polling enabled |
| `task build` | Build the binary |
| `task test` | Run all tests |
| `task once` | Run single ingestion and exit |
| `task daily` | Run daily jobs manually |
| `task pull-db` | Pull production database from Fly.io |

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
