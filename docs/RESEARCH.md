# WandiWeather Research Summary

> **Last updated**: February 1, 2026

## Key Findings

### Forecast Accuracy (6 days: Dec 26-31, 2025)

Summer hot spell conditions. WU (GRAF model) is the only active forecast source.

| Source | Days | Bias Max | Bias Min | MAE Max | MAE Min |
|--------|------|----------|----------|---------|---------|
| WU | 6 | -6.0°C | -0.5°C | 6.0°C | 0.8°C |

**Key insight**: WU significantly under-predicts max temps during hot spells (-6°C bias), but overnight lows are accurate (MAE 0.8°C). Bias correction needed for max temps.

### Station Network

**Active stations** (4):
| Station | Name | Elevation | Role |
|---------|------|-----------|------|
| IWANDI23 | Wandiligong (Primary) | 386m | Ground truth |
| IWANDI25 | Wandiligong (Shade) | 386m | Shade reference |
| IVICTORI162 | Wandiligong | 392m | Upper - inversion detection |
| IBRIGH180 | Bright | 313m | Bright comparison |

**Note**: Elevations sourced from [Open-Elevation API](https://open-elevation.com/), not WU. WU station metadata reports incorrect elevations (e.g., IWANDI23 as 117m when actual is 386m).

**Inactive stations** (7): IWANDI8 (rain gauge broken), IWANDI10, IWANDI22, IWANDI24, IBRIGH55, IBRIGH127, IBRIGH169

### Inversion Detection

Valley floor inversions observed but weaker than expected:
- Typical overnight difference: ~1°C (valley vs upper)
- Detection threshold: upper station warmer than valley by >1°C
- Clear nights (Dec 23-26) showed consistent but mild inversions

---

## Data Sources

### Weather Underground PWS API ✅

**Base URL**: `https://api.weather.com/v2/pws/`

**Authentication**: API key via `apiKey` query param (`PWS_API_KEY` env var)

**Endpoints**:
| Endpoint | Description | Resolution |
|----------|-------------|------------|
| `/observations/current?stationId={id}` | Current conditions | Real-time |
| `/observations/all/1day?stationId={id}` | Today's history | ~5 min intervals |
| `/observations/hourly/7day?stationId={id}` | 7-day history | Hourly summaries |

### WU Forecast API ✅

**Endpoint**: `https://api.weather.com/v3/wx/forecast/daily/5day`

**Parameters**:
- `geocode=-36.794,146.977` (Wandiligong coords)
- `format=json`, `units=m`, `language=en-AU`
- `apiKey={key}`

**Forecast Source**: The Weather Company's **GRAF** model:
- Resolution: 3.5km over land
- Updates: Hourly
- Inputs: GFS, ECMWF, NAM + PWS network data

**Why bias correction needed**: GRAF doesn't account for valley cold air drainage, local inversions, or sheltered microclimate effects.

### BOM Forecast API ✅

**FTP Access**: `ftp://ftp.bom.gov.au/anon/gen/fwo/IDV10753.xml`

Uses Wangaratta forecast (35km from Wandiligong). Initially discontinued Dec 27 but reactivated for comparison.

**Key behavior**: BOM drops min temp from forecast once the day starts (overnight min has already occurred). Verification must use day-before snapshot.

---

## Valley Microclimate

Wandiligong sits in the Ovens Valley (~350-400m elevation):

1. **Cold Air Drainage**: Valley floors trap cold air at night
2. **Temperature Inversions**: Valley can be colder than surrounding slopes
3. **Rain Shadow**: Precipitation varies significantly over short distances
4. **Wind Shelter**: Valley protected from prevailing winds

### Elevation Model

All active stations are at similar elevations (313-392m) making traditional elevation-tier inversion detection less effective. Current approach compares IWANDI23 (valley floor) with IVICTORI162 (upper).

---

## Technical Notes

### Database

SQLite with WAL mode. Schema version 19 includes:
- `observations`: 5-minute PWS readings with `obs_type` (instant/hourly_aggregate/unknown)
- `forecasts`: WU and BOM forecasts with `source` column
- `daily_summaries`: Computed daily stats with inversion detection and regime flags
- `forecast_verification`: Bias tracking with wind and precip fields
- `ingest_runs`: Audit trail for all API fetches (HTTP status, record counts, parse errors)
- `raw_payloads`: Compressed raw API responses for ML training (90-day retention)

### Forecast Verification Methodology

**Lock-in rule**: For valid date D, verify forecasts fetched before start of day D (local time).

**Why**: 
- BOM drops min temp once the day starts
- WU adjusts forecasts throughout the day (same-day "nowcasting")
- Using day-before forecasts measures true advance prediction skill

**Lead times**: Verify separately by `day_of_forecast` (1-day, 2-day, 3-day ahead) to track how accuracy degrades with lead time.

### API Implementation

- Go stdlib (`net/http`, `html/template`)
- HTMX for interactivity
- Chart.js for temperature graphs
- Deployed on Fly.io with persistent volume
