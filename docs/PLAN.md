# WandiWeather Implementation Plan

> **Last updated**: December 26, 2025

## Vision

A hyperlocal weather service for Wandiligong/Bright that provides:
- Accurate current conditions from local PWS network
- Short-term forecasts corrected for valley microclimate
- Historical data and trends
- Inversion/frost detection using elevation gradient

---

## Current Status

### ✅ Phase 1: Data Collection (COMPLETE)
- 9 active PWS stations ingesting every 5 minutes
- WU 5-day forecast ingestion (GRAF model)
- BOM 7-day forecast ingestion (Wangaratta via FTP)
- SQLite storage with migrations system
- 7+ days of historical data

### ✅ Phase 2: Daily Processing (COMPLETE)
- Daily summary computation with min/max/avg
- Inversion detection (valley vs upper overnight temps)
- Forecast verification (WU/BOM vs actuals)
- Automated daily jobs at 6am Melbourne time
- CLI flags: `--daily`, `--backfill-daily`

### 🔄 Phase 3: Forecast Correction (IN PROGRESS)
- Need 2-4 weeks of verification data
- Initial findings: WU already accurate, BOM has +8°C warm bias

### 📋 Phase 4: Enhanced Features (TODO)
- Dashboard improvements (show both forecasts, verification stats)
- Frost/heat alerts
- Historical comparisons

---

## CLI Usage

```bash
# Normal operation (server + scheduler)
go run ./cmd/wandiweather --db data/wandiweather.db

# Backfill historical observations
go run ./cmd/wandiweather --db data/wandiweather.db --backfill7d

# Run daily jobs manually (summaries + verification)
go run ./cmd/wandiweather --db data/wandiweather.db --daily

# Backfill all daily summaries and verification
go run ./cmd/wandiweather --db data/wandiweather.db --backfill-daily

# Single ingestion and exit
go run ./cmd/wandiweather --db data/wandiweather.db --once
```

---

## Phase 1: Data Collection Foundation

**Goal**: Build reliable data ingestion and storage

### 1.1 Database Schema
```sql
-- Core observations from PWS stations
CREATE TABLE observations (
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

-- Station metadata
CREATE TABLE stations (
    station_id TEXT PRIMARY KEY,
    name TEXT,
    latitude REAL,
    longitude REAL,
    elevation REAL,
    elevation_tier TEXT,  -- 'valley_floor', 'mid_slope', 'upper'
    is_primary BOOLEAN DEFAULT FALSE,
    active BOOLEAN DEFAULT TRUE
);

-- WU forecast snapshots (to measure bias)
CREATE TABLE forecasts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    fetched_at DATETIME NOT NULL,
    valid_date DATE NOT NULL,
    day_of_forecast INTEGER,  -- 0=today, 1=tomorrow, etc.
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

-- Daily summaries (computed from observations)
CREATE TABLE daily_summaries (
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
    inversion_strength REAL,  -- temp diff between valley and upper
    PRIMARY KEY (date, station_id)
);

-- Forecast verification & bias tracking
CREATE TABLE forecast_verification (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    forecast_id INTEGER REFERENCES forecasts(id),
    valid_date DATE,
    forecast_temp_max REAL,
    forecast_temp_min REAL,
    actual_temp_max REAL,
    actual_temp_min REAL,
    bias_temp_max REAL,  -- forecast - actual
    bias_temp_min REAL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Indexes
CREATE INDEX idx_obs_station_time ON observations(station_id, observed_at);
CREATE INDEX idx_obs_time ON observations(observed_at);
CREATE INDEX idx_forecasts_valid ON forecasts(valid_date);
```

### 1.2 Ingestion Service

**Primary stations to ingest**:
| Station | Elevation | Tier | Priority |
|---------|-----------|------|----------|
| IWANDI23 | 117m | valley_floor | ⭐ PRIMARY |
| IWANDI24 | 161m | valley_floor | backup |
| IVICTORI162 | 400m | upper | inversion ref |
| IBRIGH180 | 96m | valley_floor | Bright ref |

**Ingestion schedule**:
- Current observations: every 5 minutes
- Forecast fetch: every 6 hours (0:00, 6:00, 12:00, 18:00)
- Daily summary computation: 00:05 each day

### 1.3 Tasks
- [ ] Set up project structure (Python/TypeScript TBD)
- [ ] Create SQLite database with schema
- [ ] Build WU API client
- [ ] Implement observation ingestion cron job
- [ ] Implement forecast ingestion cron job
- [ ] Add basic health monitoring/alerting

---

## Phase 2: API & Basic Display

**Goal**: Expose data via API, build simple dashboard

### 2.1 API Endpoints

```
GET /api/current
  → Latest observations from primary station + neighbours
  → Includes inversion status if detected

GET /api/current/:stationId
  → Latest observation from specific station

GET /api/history?start=&end=&station=
  → Historical observations
  → Default: last 24 hours from primary station

GET /api/daily?start=&end=
  → Daily summaries (min/max/precip)

GET /api/forecast
  → Bias-corrected forecast for next 7 days

GET /api/stations
  → List of active stations with metadata

GET /api/stats
  → Monthly/yearly statistics, records
```

### 2.2 Dashboard Features (MVP)
- Current conditions card (temp, feels like, humidity, wind, rain)
- 24-hour temperature chart
- 7-day forecast (corrected)
- Inversion indicator
- Comparison: "X°C warmer/colder than Bright"

### 2.3 Tasks
- [ ] Choose web framework (FastAPI? Next.js?)
- [ ] Implement API endpoints
- [ ] Build basic frontend dashboard
- [ ] Deploy somewhere (Vercel? Fly.io? Home server?)

---

## Phase 3: Forecast Correction

**Goal**: Learn and apply local bias corrections

### 3.1 Bias Calculation

After 2-4 weeks of data collection:

```python
def calculate_bias(metric: str, days: int = 30) -> dict:
    """
    Compare forecasts to actuals, return bias statistics.
    
    Returns:
        {
            'mean_bias': -2.3,      # forecast was 2.3°C too warm
            'std_dev': 1.1,
            'sample_size': 28,
            'by_condition': {
                'clear_calm': -4.1,  # bigger bias on clear nights
                'cloudy': -0.8,
            }
        }
    """
```

### 3.2 Correction Factors

| Condition | Temp Max Bias | Temp Min Bias | Notes |
|-----------|---------------|---------------|-------|
| Baseline | TBD | TBD | After data collection |
| Clear + calm night | - | TBD (expect -3 to -5°C) | Valley cooling |
| Cloudy night | - | TBD (expect -1°C) | Less radiative loss |
| NW wind | TBD | - | Foehn effect possible |
| Winter | TBD | TBD | Inversions more common |

### 3.3 Inversion Detection

```python
def detect_inversion(observations: dict) -> dict:
    """
    Compare valley floor temps to upper stations.
    
    Normal: valley warmer during day, cooler at night (but not by much)
    Inversion: valley significantly colder than upper slopes
    """
    valley_temp = observations['IWANDI23']['temp']  # 117m
    upper_temp = observations['IVICTORI162']['temp']  # 400m
    
    # Normal lapse rate: ~6.5°C per 1000m
    expected_diff = (400 - 117) / 1000 * 6.5  # ~1.8°C
    actual_diff = upper_temp - valley_temp
    
    if actual_diff > expected_diff + 2:  # Upper is warmer than expected
        return {
            'inversion': True,
            'strength': actual_diff - expected_diff,
            'valley_temp': valley_temp,
            'upper_temp': upper_temp
        }
    return {'inversion': False}
```

### 3.4 Tasks
- [ ] Build bias calculation pipeline
- [ ] Implement corrected forecast generation
- [ ] Add inversion detection
- [ ] Create frost warning logic
- [ ] Backtest corrections against historical data

---

## Phase 4: Enhanced Features

**Goal**: Add value beyond raw data

### 4.1 Alerts & Notifications
- Frost warning (temp forecast < 2°C + clear + calm)
- Heat warning (temp > 35°C)
- Rain incoming (based on pressure trend + forecast)
- Inversion forming/breaking

### 4.2 Historical Analysis
- "This day in history" - compare to past years
- Monthly/seasonal summaries
- Records tracking (hottest, coldest, wettest)
- Trend analysis (is it getting warmer?)

### 4.3 Nowcasting (0-6 hours)
- Blend current conditions with forecast
- Trend extrapolation for very short term
- "Rain likely in next 2 hours" based on pressure/humidity trends

### 4.4 Tasks
- [ ] Implement notification system
- [ ] Build historical comparison features
- [ ] Add nowcasting logic
- [ ] Create public "conditions" page for visitors

---

## Technical Stack

### Backend: Go

```
wandiweather/
├── cmd/
│   └── wandiweather/
│       └── main.go           # Entry point
├── internal/
│   ├── api/
│   │   └── handlers.go       # HTTP handlers
│   ├── ingest/
│   │   ├── pws.go            # Weather Underground client
│   │   └── scheduler.go      # Cron scheduling
│   ├── models/
│   │   └── models.go         # Data structures
│   ├── store/
│   │   └── sqlite.go         # Database operations
│   └── forecast/
│       └── correction.go     # Bias correction logic
├── web/
│   ├── templates/
│   │   ├── base.html
│   │   ├── index.html
│   │   └── partials/
│   │       ├── current.html
│   │       └── forecast.html
│   └── static/
│       └── style.css
├── data/
│   └── wandiweather.db       # SQLite database
├── go.mod
└── go.sum
```

### Dependencies (minimal)

```go
// go.mod
module github.com/lox/wandiweather

go 1.22

require (
    github.com/mattn/go-sqlite3 v1.14.22  // SQLite driver
    // That's it - use stdlib for everything else
)
```

**Key decisions:**
- `net/http` - standard library router (no framework needed)
- `html/template` - server-rendered templates
- `database/sql` + sqlite3 - simple persistence
- `time.Ticker` in goroutine - scheduled ingestion (no cron library needed)
- **HTMX** - interactive UI without JavaScript complexity

### Frontend: Go Templates + HTMX

HTMX lets us build interactive UIs with HTML attributes instead of JavaScript:

```html
<!-- Refresh current conditions every 60 seconds -->
<div hx-get="/partials/current" 
     hx-trigger="load, every 60s"
     hx-swap="innerHTML">
  Loading...
</div>

<!-- Click to load history chart -->
<button hx-get="/partials/history?hours=24" 
        hx-target="#chart">
  Last 24 Hours
</button>
```

**Why HTMX:**
- No build step, no npm, no node_modules
- Just add `<script src="htmx.min.js">` (14kb)
- Server returns HTML fragments, not JSON
- Perfect for Go templates
- Single binary deployment

### For charts
- **Chart.js** via CDN (or lightweight alternative like uPlot)
- Data passed as JSON in a `<script>` tag or fetched via API

---

## Timeline

| Phase | Duration | Outcome |
|-------|----------|---------|
| Phase 1 | 1-2 weeks | Data flowing into SQLite |
| Phase 2 | 1-2 weeks | Basic API + dashboard live |
| Phase 3 | 2-4 weeks | Need data history first, then corrections |
| Phase 4 | Ongoing | Add features as needed |

**Critical path**: Start Phase 1 ASAP - we need 2-4 weeks of forecast vs actual data before we can calculate meaningful bias corrections.

---

## Success Metrics

1. **Data reliability**: >99% uptime on ingestion
2. **Forecast accuracy**: Beat WU raw forecast for overnight lows by >1°C MAE
3. **Usefulness**: Actually check it before going outside!

---

## Deployment

Single binary + SQLite file = simple deployment:

```bash
# Build
go build -o wandiweather ./cmd/wandiweather

# Run
./wandiweather --db ./data/wandiweather.db --port 8080
```

**Options:**
- Home server / Raspberry Pi (ideal - always on, local)
- Fly.io free tier (easy, but needs persistent volume for SQLite)
- Any VPS

---

## Open Questions

1. Where to host? (Home server vs cloud)
2. Public or private? (Share with neighbours?)
3. Integration with home automation? (Home Assistant?)
