# Refactor And Cleanup Review

> **Created**: March 2, 2026
> **Status**: Draft
> **Goal**: Capture concrete bug risks, duplication, and maintainability improvements discovered in a full repository review.

## Overview

This document summarizes a full codebase review focused on:

1. Potential bugs or behavior regressions.
2. Duplication and abstraction opportunities.
3. Maintainability and reasoning complexity.

The review found multiple high-impact correctness/security issues and a set of medium-priority consistency/refactor opportunities.

---

## Validation Baseline

The following checks were run during the review:

1. `task test` (pass)
2. `task lint` (pass)
3. `go test -race ./...` (pass)
4. `go test -cover ./...` (pass)

Coverage highlights (from `go test -cover ./...`):

| Package | Coverage |
|---------|----------|
| `internal/forecast` | 61.5% |
| `internal/firedanger` | 78.7% |
| `internal/emergency` | 60.5% |
| `internal/api` | 21.8% |
| `internal/store` | 15.6% |
| `internal/ingest` | 5.2% |
| `internal/imagegen` | 0.0% |

---

## High-Priority Findings

### 1) Sub-zero temperatures render as missing data

**Impact**: High (user-visible correctness)

The current conditions hero only renders a numeric temp when `ValleyTemp > 0.0`, so valid `0°C` and negative values are displayed as `—`.

**Location**: `internal/api/templates/current.html` (around `if gt .ValleyTemp 0.0`)

---

### 2) Pprof endpoints are exposed on the main HTTP mux

**Impact**: High (security/performance exposure if internet-accessible)

`/debug/pprof/*` routes are mounted unconditionally with no environment gating or auth.

**Location**: `internal/api/server.go` (`/debug/pprof/`, `/heap`, `/goroutine`, `/allocs`)

---

### 3) Emergency alert severity can become stale after updates

**Impact**: High (incorrect alert ordering/urgency classification)

`UpsertAlert` conflict updates `status/name/headline/body/last_seen/updated_at` but does not update `severity` (or distance/location metadata). If an alert escalates, stored severity can remain outdated.

**Location**: `internal/store/alerts.go` (`ON CONFLICT(id) DO UPDATE` set list)

---

## Medium-Priority Findings

### 4) Accuracy chart uses `0.0` for missing source/day values

**Impact**: Medium (analytics misrepresentation)

Missing WU/BOM values are appended as `0.0`, producing synthetic "perfect" bars. Presence flags (`hasWU`, `hasBOM`) are tracked but not used during chart series output.

**Location**: `internal/api/handlers_pages.go` (chart data assembly), `internal/api/templates/accuracy.html` (chart arrays)

---

### 5) Core current-data metrics are hard-coded to station ID `IWANDI23`

**Impact**: Medium (configuration drift risk)

`GetTodayStatsExtended`, `GetTempChangeRate`, and `GetRainHistory` are called with a hard-coded station ID, even though primary station discovery exists.

**Location**: `internal/api/current_data.go`

---

### 6) Weekly forecast template hides BOM-only data

**Impact**: Medium (degraded UX/data display)

Forecast day card temperatures render only when `.WU` exists. BOM-only days display `—` despite BOM values being available.

**Location**: `internal/api/templates/forecast.html`

---

### 7) Daily summary backfill uses dates from first active station only

**Impact**: Medium (incomplete backfill)

Backfill derives date range from `stations[0]` only, so dates present only in other active stations may be skipped.

**Location**: `internal/ingest/daily.go` (`BackfillSummaries`)

---

### 8) Verification forecast selection requires `temp_max IS NOT NULL`

**Impact**: Medium (verification data selection bias)

The selector enforces `temp_max IS NOT NULL` in the inner query. Partial records with useful min/precip/wind values can be excluded.

**Location**: `internal/store/sqlite.go` (`GetVerificationForecasts`)

---

### 9) Timezone handling differs between scheduled and manual daily runs

**Impact**: Medium (date-boundary behavior differences)

Scheduled daily jobs use Melbourne-local date; manual `RunDailyJobs` path uses host local/system timezone (`time.Now()` without `s.loc`).

**Location**: `internal/ingest/scheduler.go`

---

### 10) Partial handlers suppress data/template errors

**Impact**: Medium-Low (observability/diagnostics)

Several handlers ignore store/template errors and continue, making failures harder to detect and debug.

**Location**: `internal/api/handlers_pages.go` (`handleChartPartial`, `handleForecastPartial`)

---

## Duplication And Refactor Opportunities

### A) Duplicate station configuration sources

`defaultStations` and `stationIDs` duplicate station identity data and can drift independently.

**Location**: `cmd/wandiweather/main.go`

**Refactor direction**: derive polling IDs from the seeded station list (or a single station config source).

---

### B) Repeated ingest-run bookkeeping paths (WU vs BOM)

Scheduler forecast ingestion contains near-duplicate run lifecycle logic (start run, set fields, store raw payload, insert rows, complete run).

**Location**: `internal/ingest/scheduler.go` (`ingestForecasts`)

**Refactor direction**: extract a shared ingest pipeline helper to reduce divergence.

---

### C) Date normalization/string matching spread across packages

Repeated `Format("2006-01-02")` and `SUBSTR(...)` patterns are used in `api`, `ingest`, and `store`.

**Location**: broad (`internal/api/*`, `internal/ingest/*`, `internal/store/sqlite.go`)

**Refactor direction**: centralize date-key and local-day boundary helpers.

---

## Proposed Cleanup Plan

### Phase 1: Correctness + Safety (P0)

1. Fix negative/zero temp rendering in `current.html`.
2. Gate or remove pprof routes for production.
3. Update `UpsertAlert` conflict updates to include severity and key metadata.
4. Add regression tests for all three.

### Phase 2: Data Integrity + Consistency (P1)

1. Make accuracy chart represent missing values explicitly (nulls/gaps, not zeros).
2. Remove hard-coded primary station usage in current-data aggregation.
3. Render BOM fallback temperatures in weekly forecast cards.
4. Align manual daily-run timezone behavior with scheduler behavior.
5. Expand verification selection logic to retain useful partial forecast rows.

### Phase 3: Maintainability Refactors (P2)

1. Consolidate station config source-of-truth.
2. Extract common ingest lifecycle helper(s).
3. Introduce shared date boundary/key utilities.
4. Improve handler error propagation/logging consistency.

---

## Testing Priorities

Given low coverage in risk-heavy packages, prioritize tests for:

1. `internal/store`: `GetVerificationForecasts`, `UpsertAlert` conflict update behavior.
2. `internal/api`: template rendering for freezing temps, BOM-only forecast rendering, chart missing-data behavior.
3. `internal/ingest`: timezone-sensitive `RunDailyJobs`/backfill behavior.
4. `internal/imagegen`: cache/get/set/fallback behavior and OG generation failure paths.

---

## Suggested Execution Order

1. Land Phase 1 as a focused bugfix change.
2. Land Phase 2 as a data-consistency change with snapshot/regression tests.
3. Land Phase 3 in smaller refactor-only PRs (no behavior changes per PR).

This order reduces user-facing and operational risk first, then addresses structural maintainability.
