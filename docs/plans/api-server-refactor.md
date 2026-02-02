# API Server Refactoring Plan

> **Created**: February 2, 2026  
> **Status**: Phase 2 complete ✅  
> **Goal**: Split `internal/api/server.go` into smaller, testable components

## Overview

The `internal/api/server.go` file has grown to ~2000 lines containing HTTP handlers, business logic, data aggregation, and template rendering. This plan outlines a phased approach to split it into logical components that are easier to maintain and test.

**Current state**: Monolithic `server.go` with mixed concerns  
**Target state**: Layered architecture with clear separation of HTTP, business logic, and presentation

---

## Problem Summary

| Issue | Impact | Priority |
|-------|--------|----------|
| Single 2000+ line file | Hard to navigate, review, maintain | High |
| Business logic mixed with HTTP handlers | Hard to unit test without HTTP | High |
| Template funcs embedded in server | Tight coupling | Medium |
| View models mixed with domain models | Unclear boundaries | Medium |
| No interfaces for dependencies | Hard to mock in tests | Medium |

---

## Phased Approach

### Phase 1: Split files within `api` package ✅ Ready
**Effort**: S–M (< half day)  
**Risk**: Low (no import changes)

Split `server.go` into multiple files within the same package:

| File | Contents |
|------|----------|
| `server.go` | Server struct, NewServer, Run, route registration |
| `handlers_pages.go` | `/`, `/partials/*` page handlers |
| `handlers_api.go` | `/api/*` JSON endpoint handlers |
| `handlers_images.go` | `/weather-image`, `/og-image` handlers |
| `viewmodels.go` | CurrentData, ForecastData, ForecastDay, TodayForecast, etc. |
| `templates.go` | Template funcs (deref, abs, neg, upper) + parsing |
| `current_data.go` | getCurrentData() and related helpers |
| `forecast_data.go` | getForecastData() and related helpers |

**Benefits**:
- Immediate navigability improvement
- Easier code review (smaller diffs per file)
- No breaking changes

---

### Phase 2: Extract forecast math to `internal/forecast` ✅ Complete
**Effort**: S (< 2 hours)  
**Risk**: Low

Move pure forecast calculation logic to `internal/forecast`:

```
internal/forecast/
├── correction.go      # BiasCorrector (existing)
├── nowcast.go         # Nowcaster (existing)
├── todaytemps.go      # ComputeTodayTemps, LookupBias, LookupBiasWithFallback
└── todaytemps_test.go # Table-driven tests (20 cases)
```

**Key changes**:
- `computeTodayTemps()` → `forecast.ComputeTodayTemps()`
- `getCorrectionBiasWithFallback()` → `forecast.LookupBiasWithFallback()`
- All functions take inputs as parameters (pure functions, no DB calls)
- Easy to unit test with table tests
- `api` package calls `forecast` directly (no wrapper files)

**Example test structure**:
```go
func TestComputeTodayTemps(t *testing.T) {
    tests := []struct {
        name     string
        input    TodayTempInput
        wantMax  float64
        wantMin  float64
    }{
        {
            name: "prefers BOM over WU for max",
            input: TodayTempInput{
                BOMForecast: &models.Forecast{TempMax: sql.NullFloat64{Float64: 28, Valid: true}},
                WUForecast:  &models.Forecast{TempMax: sql.NullFloat64{Float64: 30, Valid: true}},
            },
            wantMax: 28,
        },
        {
            name: "falls back to WU when BOM exceeds current by 3+",
            input: TodayTempInput{
                BOMForecast:    &models.Forecast{TempMax: sql.NullFloat64{Float64: 25, Valid: true}},
                WUForecast:     &models.Forecast{TempMax: sql.NullFloat64{Float64: 30, Valid: true}},
                CurrentTemp:    29,
                HasCurrentTemp: true,
            },
            wantMax: 30,
        },
        // ... more cases
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := ComputeTodayTemps(tt.input)
            if result.TempMax != tt.wantMax {
                t.Errorf("TempMax = %v, want %v", result.TempMax, tt.wantMax)
            }
        })
    }
}
```

---

### Phase 3: Create view models package (optional)
**Effort**: M (half day)  
**Risk**: Medium (requires import updates)

Create `internal/web/viewmodel` for presentation-facing types:

```
internal/web/viewmodel/
├── current.go      # CurrentData, TodayStats, StationReading, InversionStatus
├── forecast.go     # ForecastData, ForecastDay, TodayForecast, ForecastExplanation
├── chart.go        # ChartData
└── index.go        # IndexData (combines current + forecast)
```

**Benefits**:
- Clear separation from persistence models (`internal/models`)
- Types optimized for templates/JSON, not storage
- Easier to evolve API responses independently

---

### Phase 4: Create service layer (optional, higher effort)
**Effort**: L (1–2 days)  
**Risk**: Medium-High

Create application services with interfaces for dependencies:

```
internal/app/
├── current/
│   ├── service.go       # Service struct with Get() method
│   ├── service_test.go  # Tests with mock store
│   └── interfaces.go    # Store, Clock interfaces (local to package)
└── forecast/
    ├── service.go       # Service struct with Get5Day(), GetToday()
    ├── service_test.go
    └── interfaces.go
```

**Example service interface**:
```go
// internal/app/current/interfaces.go
type Store interface {
    GetActiveStations() ([]models.Station, error)
    GetLatestObservation(stationID string) (*models.Observation, error)
    GetTodayStatsExtended(stationID string, day time.Time) (*models.TodayStatsExtended, error)
}

type Clock interface {
    Now() time.Time
}
```

**Benefits**:
- Unit tests don't need real DB
- Clear boundaries between aggregation logic and HTTP
- Handlers become thin wiring layer

---

### Phase 5: Extract template renderer (optional)
**Effort**: S (< 2 hours)  
**Risk**: Low

Create `internal/web/render` for template concerns:

```
internal/web/render/
├── renderer.go     # Renderer interface + implementation
├── funcs.go        # Template funcs (deref, abs, neg, upper)
└── templates/      # Move templates here (or keep in api/templates)
```

**Interface**:
```go
type Renderer interface {
    Execute(w io.Writer, name string, data any) error
}
```

---

## Recommended Execution Order

1. **Phase 1** (immediate): Split files in `api` package
2. **Phase 2** (next): Extract `computeTodayTemps` to `internal/forecast`
3. **Phases 3-5** (as needed): Only when complexity grows or multiple frontends needed

---

## Risks and Guardrails

| Risk | Mitigation |
|------|------------|
| Circular dependencies when moving types | viewmodel must not import api; services import viewmodel; api imports both |
| Breaking JSON API responses | Add golden tests for JSON endpoints before refactoring |
| Template rendering changes | Add snapshot tests for key templates |
| Time-dependent logic becomes flaky | Introduce Clock interface in Phase 4 |

---

## Success Criteria

### Phase 1 Complete When:
- [x] `server.go` is under 300 lines (135 lines ✅)
- [x] Each new file has a clear single responsibility
- [x] `task check` passes
- [x] No functional changes (same behavior)

### Phase 2 Complete When:
- [x] `computeTodayTemps` lives in `internal/forecast` → `forecast.ComputeTodayTemps`
- [x] `getCorrectionBiasWithFallback` lives in `internal/forecast` → `forecast.LookupBiasWithFallback`
- [x] Table-driven tests cover key scenarios (20 test cases)
- [x] ~80% test coverage on `ComputeTodayTemps`, 88% on `LookupBiasWithFallback`
- [x] Bug fixes: symmetric BOM/WU diff check, proper observed temp validity flags

---

## File Size Results (internal/api)

| File | Target Lines | Actual Lines |
|------|--------------|--------------|
| server.go | < 300 | 135 ✅ |
| handlers_pages.go | ~400 | 429 ✅ |
| handlers_api.go | ~200 | 57 ✅ |
| handlers_images.go | ~150 | 283 |
| current_data.go | ~400 | 351 ✅ |
| forecast_data.go | ~300 | 288 ✅ |
| viewmodels.go | ~200 | 216 ✅ |
| templates.go | — | 33 |

## File Size Results (internal/forecast, new)

| File | Lines |
|------|-------|
| todaytemps.go | 289 |
| todaytemps_test.go | 340 |

---

## References

- [Go Project Layout](https://github.com/golang-standards/project-layout)
- [Effective Go](https://go.dev/doc/effective_go)
- Oracle consultation (Feb 2, 2026)
