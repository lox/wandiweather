# Forecast Correction Implementation Plan

> **Created**: January 2, 2026  
> **Status**: Phase 1 & 2 implemented, nowcast disabled pending validation

## Overview

This document details the phased approach to forecast correction for WandiWeather. The strategy uses manual regime classification and nowcasting rules while data is limited, then transitions to linear regression once sufficient data accumulates.

**Current state**: 6 days of verification data (Dec 26-31, summer hot spell)  
**Key finding**: WU has -6°C bias on max temps during hot spells, but overnight lows are accurate (MAE 0.8°C)

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     Correction Pipeline                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                   │
│  Raw Forecast ──▶ Level 0 Bias ──▶ Level 1 Regime ──▶ Nowcast   │
│                   (always on)      (when N≥15)        (day 0)    │
│                                                                   │
│  Future: Level 2 Linear Model replaces Level 0+1 when N≥60      │
│                                                                   │
└─────────────────────────────────────────────────────────────────┘
```

---

## Phase 1: Regime Classification (Implement Now)

### 1.1 Regime Flags

Classify each day into weather regimes for stratified bias correction.

```go
// internal/forecast/regimes.go

type RegimeFlags struct {
    Heatwave       bool  // Hot spell conditions
    InversionNight bool  // Valley colder than upper overnight
    ClearCalm      bool  // Clear skies, low wind (radiative conditions)
}

func ClassifyRegime(
    forecast Forecast,
    summary DailySummary,
    prevDays []DailySummary,
) RegimeFlags {
    return RegimeFlags{
        Heatwave:       classifyHeatwave(forecast, prevDays),
        InversionNight: summary.InversionDetected.Valid && summary.InversionDetected.Bool,
        ClearCalm:      classifyClearCalm(summary),
    }
}

func classifyHeatwave(fc Forecast, prevDays []DailySummary) bool {
    // Forecast max ≥32°C
    if fc.TempMax.Valid && fc.TempMax.Float64 >= 32 {
        return true
    }
    // Recent actuals: any of last 2 days ≥30°C
    for _, d := range prevDays {
        if d.TempMax.Valid && d.TempMax.Float64 >= 30 {
            return true
        }
    }
    // 2-day average ≥28°C
    if len(prevDays) >= 2 {
        avg := (prevDays[0].TempMax.Float64 + prevDays[1].TempMax.Float64) / 2
        if avg >= 28 {
            return true
        }
    }
    return false
}

func classifyClearCalm(summary DailySummary) bool {
    // High solar radiation (clear day proxy) + low evening wind
    // Thresholds TBD based on data analysis
    const solarThreshold = 800.0  // W/m² - needs tuning
    const windThreshold = 10.0    // km/h evening average
    
    // For now, return false until we have solar/wind evening data
    return false
}
```

### 1.2 Schema Changes

Add regime flags to daily summaries for historical tracking:

```sql
-- Migration: Add regime columns to daily_summaries
ALTER TABLE daily_summaries ADD COLUMN regime_heatwave BOOLEAN;
ALTER TABLE daily_summaries ADD COLUMN regime_inversion BOOLEAN;
ALTER TABLE daily_summaries ADD COLUMN regime_clear_calm BOOLEAN;
```

Update `forecast_correction_stats` to support regime-stratified stats (already in schema per PLAN.md).

### 1.3 Daily Job Changes

Modify `RunDailyJobs()` to:
1. Compute regime flags for yesterday
2. Store flags in `daily_summaries`
3. Update `forecast_correction_stats` for both `regime='all'` and specific regimes

---

## Phase 2: Morning Nowcasting (Implement Now)

### 2.1 Concept

By 10-11am, morning observations predict afternoon max better than the overnight forecast. Apply a same-day correction using the morning temperature delta.

```
Δ_morning = T_observed_morning - T_forecast_morning
corrected_max = bias_corrected_max + (α × Δ_morning)
```

Where:
- `T_observed_morning`: Mean observed temp 09:00-11:00 from primary PWS
- `T_forecast_morning`: WU hourly forecast for same window (or interpolated)
- `α = 0.7`: Damping factor (tunable)

### 2.2 Implementation

```go
// internal/forecast/nowcast.go

type NowcastCorrection struct {
    ObservedMorning  float64   // Mean temp 09:00-11:00
    ForecastMorning  float64   // Forecast temp for same window
    Delta            float64   // Observed - Forecast
    Adjustment       float64   // α × Delta (capped)
    CorrectedMax     float64   // Final corrected max
    AppliedAt        time.Time
}

const (
    nowcastAlpha    = 0.7
    maxAdjustment   = 4.0  // Cap: |adjustment| ≤ 4°C
    nowcastStartHour = 10  // Apply after 10:00 local
    nowcastEndHour   = 11  // Use observations up to 11:00
)

func ComputeNowcast(
    store *Store,
    stationID string,
    forecastMax float64,
    biasCorrection float64,
) (*NowcastCorrection, error) {
    now := time.Now().In(melbourneTZ)
    
    // Only apply after 10am local
    if now.Hour() < nowcastStartHour {
        return nil, nil
    }
    
    // Get morning observations (09:00-11:00)
    morningStart := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, melbourneTZ)
    morningEnd := time.Date(now.Year(), now.Month(), now.Day(), 11, 0, 0, 0, melbourneTZ)
    
    obs, err := store.GetObservationsInRange(stationID, morningStart, morningEnd)
    if err != nil || len(obs) < 6 { // Need at least 6 readings (30 min coverage)
        return nil, err
    }
    
    // Compute mean observed morning temp
    var sum float64
    var count int
    for _, o := range obs {
        if o.Temp.Valid {
            sum += o.Temp.Float64
            count++
        }
    }
    if count == 0 {
        return nil, nil
    }
    observedMorning := sum / float64(count)
    
    // Get forecast morning temp (simplified: use 10am forecast)
    // TODO: Fetch from stored hourly forecast if available
    forecastMorning := forecastMax * 0.7 // Rough approximation for now
    
    delta := observedMorning - forecastMorning
    adjustment := nowcastAlpha * delta
    
    // Cap adjustment
    if adjustment > maxAdjustment {
        adjustment = maxAdjustment
    } else if adjustment < -maxAdjustment {
        adjustment = -maxAdjustment
    }
    
    correctedMax := forecastMax - biasCorrection + adjustment
    
    return &NowcastCorrection{
        ObservedMorning: observedMorning,
        ForecastMorning: forecastMorning,
        Delta:           delta,
        Adjustment:      adjustment,
        CorrectedMax:    correctedMax,
        AppliedAt:       now,
    }, nil
}
```

### 2.3 API Integration

Add nowcast-corrected forecast to the API response for today:

```go
type TodayForecast struct {
    TempMax          float64
    TempMin          float64
    TempMaxCorrected float64  // After bias + nowcast
    NowcastApplied   bool
    NowcastDelta     float64  // For transparency
    // ... existing fields
}
```

### 2.4 Logging for Future Analysis

Store nowcast components in a new table for model training:

```sql
CREATE TABLE nowcast_log (
    id INTEGER PRIMARY KEY,
    date DATE NOT NULL,
    station_id TEXT NOT NULL,
    observed_morning REAL,
    forecast_morning REAL,
    delta REAL,
    adjustment REAL,
    forecast_max_raw REAL,
    forecast_max_corrected REAL,
    actual_max REAL,  -- Filled in next day
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(date, station_id)
);
```

---

## Phase 3: Level 1 Regime-Aware Bias (When N ≥ 15-20)

### 3.1 Activation Criteria

Only use regime-specific bias when sample size is sufficient:

```go
const minRegimeSamples = 15

func GetBiasCorrection(stats *CorrectionStats, regime string) float64 {
    // Try regime-specific first
    if regimeStats := stats.ForRegime(regime); regimeStats != nil {
        if regimeStats.SampleSize >= minRegimeSamples {
            return regimeStats.MeanBias
        }
    }
    // Fall back to 'all'
    return stats.ForRegime("all").MeanBias
}
```

### 3.2 Priority Order

For max temp correction:
1. `heatwave` regime (if N ≥ 15)
2. `all` regime (fallback)

For min temp correction:
1. `inversion` regime (if N ≥ 15) — for frost prediction
2. `all` regime (fallback)

---

## Phase 4: Level 2 Linear Model (When N ≥ 60-90)

### 4.1 Features to Log Now

Start logging these features immediately so they're available for training:

| Feature | Source | Notes |
|---------|--------|-------|
| `forecast_temp` | WU forecast | The raw forecast value |
| `prev_day_max` | daily_summaries | Yesterday's actual max |
| `prev_night_min` | daily_summaries | Last night's actual min |
| `inversion_flag` | regime classification | Boolean |
| `heatwave_flag` | regime classification | Boolean |
| `clear_calm_flag` | regime classification | Boolean |
| `sin_doy`, `cos_doy` | date | Seasonality encoding |
| `morning_delta` | nowcast | T_obs - T_fc at 10am |
| `temp_at_10am` | observations | Same-day morning temp |

### 4.2 Model Specification

```
error = β₀ + β₁×forecast_temp + β₂×prev_day_max + β₃×prev_night_min
      + β₄×inversion_flag + β₅×heatwave_flag + β₆×clear_calm_flag
      + β₇×sin(doy) + β₈×cos(doy)
```

At forecast time: `corrected = forecast - predicted_error`

### 4.3 Training Pipeline

```go
// internal/forecast/linear.go

type LinearModel struct {
    Coefficients []float64  // β₀, β₁, ... β₈
    Features     []string   // Feature names in order
    SampleSize   int
    MAE          float64    // Training MAE
    UpdatedAt    time.Time
}

func TrainLinearModel(data []TrainingRow) (*LinearModel, error) {
    // Implement OLS in pure Go
    // Use normal equations: β = (X'X)^(-1) X'y
    // ...
}
```

### 4.4 Activation Criteria

Switch to Level 2 when:
1. N ≥ 60 days of verification data
2. Data includes multiple regimes (not just one hot spell)
3. Cross-validated MAE beats Level 1 by ≥ 0.5°C
4. Coefficients have sensible signs

---

## Guardrails

### Magnitude Caps

```go
const (
    maxBiasCorrection     = 8.0   // |bias| ≤ 8°C
    maxNowcastAdjustment  = 4.0   // |nowcast adj| ≤ 4°C
    maxTotalCorrection    = 10.0  // |total| ≤ 10°C
)

func capCorrection(correction float64, limit float64) float64 {
    if correction > limit {
        return limit
    }
    if correction < -limit {
        return -limit
    }
    return correction
}
```

### Fallback Logic

```go
func ApplyCorrection(forecast Forecast, stats *CorrectionStats, regime RegimeFlags) float64 {
    // Start with raw forecast
    corrected := forecast.TempMax.Float64
    
    // Level 0/1: Bias correction
    bias := GetBiasCorrection(stats, regimeToString(regime))
    bias = capCorrection(bias, maxBiasCorrection)
    corrected -= bias
    
    // Nowcast (day 0 only, after 10am)
    if forecast.DayOfForecast == 0 {
        if nowcast, _ := ComputeNowcast(...); nowcast != nil {
            adj := capCorrection(nowcast.Adjustment, maxNowcastAdjustment)
            corrected += adj
        }
    }
    
    // Final cap
    totalCorrection := corrected - forecast.TempMax.Float64
    if abs(totalCorrection) > maxTotalCorrection {
        corrected = forecast.TempMax.Float64 - capCorrection(totalCorrection, maxTotalCorrection)
    }
    
    return corrected
}
```

### Monitoring

Track MAE for each correction layer:

```sql
CREATE TABLE correction_performance (
    date DATE PRIMARY KEY,
    mae_raw REAL,           -- Raw WU forecast
    mae_level0 REAL,        -- After bias correction
    mae_level1 REAL,        -- After regime correction
    mae_nowcast REAL,       -- After nowcast (day 0 only)
    mae_level2 REAL         -- After linear model (future)
);
```

Auto-disable a correction if it consistently hurts performance:

```go
func ShouldDisableCorrection(perf []CorrectionPerformance, layer string) bool {
    // If MAE for layer > MAE for previous layer over last 14 days
    // by more than 0.5°C, disable it
    // ...
}
```

---

## Implementation Tasks

### Immediate (This Week) - DONE

- [x] Add regime columns to `daily_summaries` migration
- [x] Implement `ClassifyRegime()` function
- [x] Update daily job to compute and store regime flags
- [x] Create `nowcast_log` table
- [x] Implement basic `ComputeNowcast()` (disabled pending validation)
- [x] Add corrected forecast to API response
- [x] Update dashboard to show corrected vs raw (strikethrough UI)
- [x] Require 7+ samples before applying bias correction
- [x] Add Harrietville station (543m) for inversion detection

### Short Term (2-4 Weeks)

- [ ] Accumulate 7+ days of verification data
- [ ] Accumulate 15-20 days per regime for regime-specific bias
- [ ] Validate nowcast formula and enable if effective
- [ ] Add `clear_calm` classification (needs solar/wind analysis)

### Medium Term (60-90 Days)

- [ ] Implement OLS training for Level 2
- [ ] Cross-validate linear model
- [ ] Compare Level 2 vs Level 1 performance
- [ ] Activate Level 2 if it outperforms

---

## Success Metrics

| Metric | Current | Target (Level 1) | Target (Level 2) |
|--------|---------|------------------|------------------|
| Max temp MAE | 6.0°C | < 3.0°C | < 2.0°C |
| Min temp MAE | 0.8°C | < 1.0°C | < 0.8°C |
| Heatwave max MAE | 6.0°C | < 2.5°C | < 1.5°C |

---

## References

- [PLAN.md](PLAN.md) - Overall project plan
- [RESEARCH.md](RESEARCH.md) - Data source findings
- Oracle consultation (Jan 2, 2026) - Strategy validation
