# WandiWeather Implementation Plan

> **Last updated**: February 1, 2026

## Vision

A hyperlocal weather service for Wandiligong/Bright that provides:
- Accurate current conditions from local PWS network
- Short-term forecasts corrected for valley microclimate
- Historical data and trends
- Inversion/frost detection using elevation gradient

---

## Current Status

### ✅ Phase 1: Data Collection (COMPLETE)
- 4 active PWS stations ingesting every 5 minutes
- WU 5-day forecast ingestion (GRAF model)
- BOM 7-day forecast ingestion (Wangaratta via FTP)
- SQLite storage with WAL mode and migrations
- 13+ days of historical data (Dec 19 - Jan 1)

### ✅ Phase 2: Daily Processing (COMPLETE)
- Daily summary computation with min/max/avg
- Inversion detection (valley vs upper overnight temps)
- Forecast verification with proper lock-in methodology:
  - Uses day-before forecasts (fetched before valid date starts)
  - Verifies by lead time (1-day, 2-day ahead separately)
  - Both WU and BOM sources verified
- Automated daily jobs at 6am Melbourne time
- CLI flags: `--daily`, `--backfill-daily`

### 🔄 Phase 3: Forecast Correction (IN PROGRESS)
- Verification data being collected (WU and BOM)
- Initial findings (hot spell conditions, both sources under-predict):
  - WU: ~-6°C bias on max temps during heat
  - BOM: ~-4°C bias on max temps (closer than WU)
- Level 0 bias correction implemented and active
- Three-level correction strategy planned (see Phase 3 details)

### ✅ Phase 4: ML-Ready Data Collection (COMPLETE)
- Ingest audit trail (`ingest_runs` table) for all API fetches
- Raw payload storage (`raw_payloads` table) with 90-day retention
- Observation type tracking (`obs_type`: instant vs hourly_aggregate)
- Parse error tracking (separate from fatal errors)
- Daily health check logging
- See `docs/plans/ml-data-collection.md` for details

### 📋 Phase 5: Enhanced Features (TODO)
- Dashboard improvements (show corrected forecasts, verification stats)
- Frost/heat alerts
- Historical comparisons
- ML-based forecast correction (when sufficient training data)

---

## CLI Usage

```bash
# Normal operation (server + scheduler)
go run ./cmd/wandiweather --db data/wandiweather.db

# Local dev (server only, no API calls)
go run ./cmd/wandiweather --db data/wandiweather.db --no-poll

# Backfill 7-day observation history
go run ./cmd/wandiweather --db data/wandiweather.db --backfill

# Run daily jobs manually (summaries + verification)
go run ./cmd/wandiweather --db data/wandiweather.db --daily

# Backfill all daily summaries and verification
go run ./cmd/wandiweather --db data/wandiweather.db --backfill-daily

# Single ingestion and exit
go run ./cmd/wandiweather --db data/wandiweather.db --once
```

---

## Active Stations

| Station | Name | Elevation | Role |
|---------|------|-----------|------|
| IWANDI23 | Wandiligong (Primary) | 386m | ⭐ Primary - ground truth |
| IWANDI25 | Wandiligong (Shade) | 386m | Shade reference |
| IVICTORI162 | Wandiligong | 392m | Upper - inversion detection |
| IBRIGH180 | Bright | 313m | Bright comparison |

> **Note**: Elevations from Open-Elevation API. WU metadata is incorrect (reports ~100-160m lower).

---

## Phase 3: Forecast Correction

**Goal**: Learn and apply local bias corrections using Model Output Statistics (MOS) approach

### 3.1 Three-Level Correction Strategy

**Level 0 – Bias Tables (implement now, ~6 days data)**
- Rolling mean bias per source/target/day-of-forecast
- Simple correction: `corrected = forecast - mean_bias`
- Store in `forecast_correction_stats` table
- Immediate improvement with minimal code

**Level 1 – Regime-Aware Correction (~30 days data)**
- Classify weather conditions into regimes:
  - `inversion_night`: valley colder than upper slopes overnight
  - `clear_calm`: low wind + high solar radiation previous day
  - `heatwave`: forecast max ≥32°C or recent actuals ≥30°C
- Compute separate bias for each regime
- Apply regime-specific correction based on current conditions

**Level 2 – Linear Models (~60-90 days data)**
- Simple OLS regression per target per lead-day:
  ```
  error = β₀ + β₁*forecast_temp + β₂*prev_day_max + β₃*prev_night_min
        + β₄*inversion_flag + β₅*clear_calm + β₆*heatwave_flag
        + β₇*sin(doy) + β₈*cos(doy)
  ```
- At forecast time: `corrected = forecast + predicted_error`
- Implement OLS in pure Go (no external ML libs)

### 3.2 Key Features from PWS Network

**For overnight lows (frost prediction):**
- Valley vs upper slope temp difference (inversion strength)
- Evening wind speed 18:00-00:00 (calm = inversion risk)
- Previous day max solar radiation (clear vs cloudy proxy)
- Pressure trend (stable high = radiative cooling)
- Evening dewpoint (frost risk when temp approaches dewpoint)

**For max temps (heat prediction):**
- Previous 1-2 days max temps (heat persistence)
- Morning temp 09:00-12:00 (nowcasting)
- Wind direction (NW föhn vs SE cool change)
- Morning humidity/dewpoint (dry air heats faster)
- Pressure trend (dropping = frontal)

### 3.3 Database Schema Additions

```sql
-- Precomputed bias statistics (Level 0/1)
CREATE TABLE forecast_correction_stats (
    source TEXT NOT NULL,           -- 'wu'
    target TEXT NOT NULL,           -- 'tmin', 'tmax'
    day_of_forecast INTEGER NOT NULL,
    regime TEXT NOT NULL,           -- 'all', 'inversion', 'heatwave', etc.
    window_days INTEGER NOT NULL,   -- e.g. 30
    sample_size INTEGER NOT NULL,
    mean_bias REAL NOT NULL,
    mae REAL NOT NULL,
    updated_at DATETIME NOT NULL,
    PRIMARY KEY (source, target, day_of_forecast, regime)
);

-- Linear model coefficients (Level 2)
CREATE TABLE forecast_correction_models (
    source TEXT NOT NULL,
    target TEXT NOT NULL,
    day_of_forecast INTEGER NOT NULL,
    model_type TEXT NOT NULL,       -- 'linear_v1'
    features TEXT NOT NULL,         -- JSON list of feature names
    coefficients TEXT NOT NULL,     -- JSON array of βs
    intercept REAL NOT NULL,
    sample_size INTEGER NOT NULL,
    updated_at DATETIME NOT NULL,
    PRIMARY KEY (source, target, day_of_forecast, model_type)
);
```

### 3.4 Architecture

New package: `internal/forecast/correction.go`

```go
type Corrector interface {
    Correct(fc models.Forecast, ctx ForecastContext) CorrectedForecast
}

type BiasTableCorrector struct { ... }   // Level 0/1
type LinearModelCorrector struct { ... } // Level 2
```

Daily job flow:
1. Compute daily summaries
2. Verify forecasts against actuals
3. Update correction stats (rolling window)
4. Retrain linear models (when enough data)

API serves both raw and corrected forecasts for comparison.

### 3.5 Tasks
- [ ] Create `forecast_correction_stats` table migration
- [ ] Implement `BiasTableCorrector` (Level 0)
- [ ] Add regime classification logic (Level 1)
- [ ] Update daily job to compute correction stats
- [ ] Serve corrected forecasts via API
- [ ] Add forecast accuracy display to dashboard
- [ ] Implement `LinearModelCorrector` (Level 2, after 60+ days)

### 3.6 Guardrails

- Minimum sample size per regime bucket (N ≥ 10), else fallback to general
- Rolling windows (30-60 days) to handle seasonal drift
- Monitor corrected vs raw MAE; fallback if correction hurts accuracy
- Exclude days with suspect QC status from training

---

## Technical Stack

```
wandiweather/
├── cmd/wandiweather/main.go      # Entry point, CLI flags
├── internal/
│   ├── api/                      # HTTP handlers + templates
│   ├── ingest/                   # PWS client, forecast client, scheduler, daily jobs
│   ├── models/                   # Data structures
│   ├── store/                    # SQLite operations + migrations
│   └── forecast/                 # Bias correction (TODO)
├── web/static/                   # CSS, JS (HTMX, Chart.js)
└── data/wandiweather.db          # SQLite database
```

**Dependencies** (minimal):
- `modernc.org/sqlite` - Pure Go SQLite driver
- stdlib for everything else (`net/http`, `html/template`, `database/sql`)
- HTMX + Chart.js via CDN (no npm/build step)

---

## Deployment

Deployed on Fly.io with persistent volume for SQLite.

```bash
# Build
go build -o wandiweather ./cmd/wandiweather

# Run
./wandiweather --db ./data/wandiweather.db --port 8080
```

---

## Success Metrics

1. **Data reliability**: >99% uptime on ingestion
2. **Forecast accuracy**: Beat WU raw forecast for overnight lows by >1°C MAE
3. **Usefulness**: Actually check it before going outside!
