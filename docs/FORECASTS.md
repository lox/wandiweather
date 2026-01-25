# Forecast System

## Sources

Two forecast sources are ingested and stored:

| Source | Provider | Notes |
|--------|----------|-------|
| `wu` | Weather Underground | Used for front page display |
| `bom` | Bureau of Meteorology | Displayed on forecast comparison page |

## Front Page Forecast

The main page (`/`) displays a **hybrid forecast** using the best source for each variable:

| Variable | Source | Reason |
|----------|--------|--------|
| Max temp | BOM | Better accuracy (MAE 2.86 vs 4.93) |
| Min temp | WU | Better accuracy (MAE 1.76 vs 2.93) |
| Precip | WU | More detail available |

Corrections applied:
1. **Bias correction**: Historical verification stats adjust raw forecasts
2. **Nowcasting** (day-0 only): Morning observations from primary station (IWANDI23) adjust max temp prediction

**Hidden explanation**: Click "Today's Forecast" header to see calculation details.

Code: [`internal/api/server.go`](../internal/api/server.go) in `getCurrentData()` around line 380.

## Accuracy (as of Jan 2026)

Based on ~135 verified forecasts per source:

| Source | Max Temp MAE | Min Temp MAE | Max Temp Bias |
|--------|-------------|--------------|---------------|
| BOM | 2.86°C | 2.93°C | +1.6°C (runs warm) |
| WU | 4.93°C | 1.76°C | -4.9°C (runs cold) |

**Summary**: BOM is more accurate for max temps, WU is better for min temps.

Query to regenerate:
```sql
SELECT 
  f.source,
  COUNT(*) as samples,
  ROUND(AVG(ABS(fv.bias_temp_max)), 2) as mae_max,
  ROUND(AVG(ABS(fv.bias_temp_min)), 2) as mae_min,
  ROUND(AVG(fv.bias_temp_max), 2) as avg_bias_max
FROM forecast_verification fv
JOIN forecasts f ON fv.forecast_id = f.id
WHERE fv.actual_temp_max IS NOT NULL
GROUP BY f.source;
```

## Database Tables

- `forecasts` - Raw forecast data with `source` column (`wu` or `bom`)
- `forecast_verification` - Compares forecasts to actual observations
- `forecast_correction_stats` - Bias correction coefficients by source/day

## Key Files

- [`internal/ingest/forecast.go`](../internal/ingest/forecast.go) - Fetches forecasts from APIs
- [`internal/ingest/daily.go`](../internal/ingest/daily.go) - Daily verification job
- [`internal/forecast/nowcast.go`](../internal/forecast/nowcast.go) - Nowcasting logic
- [`internal/forecast/bias.go`](../internal/forecast/bias.go) - Bias correction
