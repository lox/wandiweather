# BOM Daily API Transition Plan

> **Created**: April 30, 2026  
> **Status**: Phase 1 implemented (shadow ingest live), evaluation and transition pending  
> **Goal**: Collect `bom_daily_api` alongside the current BOM feed, compare them against observations and each other, and decide whether the daily API should replace the legacy BOM product.

## Overview

This plan treats the BOM daily API as a product evaluation exercise rather than a straight migration. The current live BOM output remains on the legacy FTP/XML feed (`bom`) while the richer daily API is collected in parallel as `bom_daily_api`.

**Current live source**: `bom` (legacy FTP/XML Wangaratta area product)  
**Shadow source**: `bom_daily_api` (daily API for location `r3811m`)  
**Current state**: Both products ingest on the same schedule; only `bom` feeds the user-facing forecast output.

The core decision is not whether the daily API is newer, but whether it is better for this site once measured against:

1. local forecast verification
2. field completeness
3. operational stability
4. downstream usefulness of the richer payload

---

## Phase 1: Shadow Ingest

### Implemented

- Keep the current live BOM source on `bom`.
- Ingest the daily API in parallel as `bom_daily_api` on the same scheduler tick.
- Store the daily API's richer fields in `forecasts` without changing existing UI reads.
- Preserve separate `ingest_runs`, `raw_payloads`, forecast rows, and verification rows by `source`.

### Richer Fields Stored

The daily API now preserves additional fields that the FTP product does not provide directly:

- `narrative_short`
- `narrative_extended`
- `precip_min`
- `precip_max`
- `precip_units`

Compatibility fields are still populated so the existing forecast pipeline can continue to use:

- `narrative`
- `precip_range`
- `precip_amount`

### Non-Goals for Phase 1

- Do not change the current forecast page output.
- Do not change the accuracy page output.
- Do not rename `bom` yet.
- Do not assume the daily API should become the live source.

---

## Phase 2: Collect and Compare

### Collection Window

Collect shadow data for at least 2 weeks, ideally 4 weeks.

We want enough coverage to include:

- ordinary stable days
- warm/hot days
- rain events
- at least a few days of D+0, D+1, and D+2 verification overlap
- any transient BOM payload or availability issues

### Comparison Questions

For each `valid_date`, compare `bom` and `bom_daily_api` on:

- `temp_max`
- `temp_min`
- `precip_chance`
- `precip_range`
- `precip_min` / `precip_max`
- narrative differences
- missing values
- lead-time behavior
- ingest timing and freshness

### Suggested Metrics

Track:

- count of dates where max temp differs by >= 1 C
- count of dates where min temp differs by >= 1 C
- count of dates where rain chance differs by >= 20 percentage points
- count of dates where one source has rain values and the other does not
- count of dates where severe keywords (`storm`, `thunder`, `heavy rain`) appear only in one source
- ingest failures by source
- parse errors by source

### Suggested SQL Checks

Compare the latest rows for each source by valid date:

```sql
WITH latest AS (
  SELECT source, valid_date, temp_max, temp_min, precip_chance, precip_range,
         precip_min, precip_max, narrative, narrative_short, narrative_extended,
         ROW_NUMBER() OVER (
           PARTITION BY source, SUBSTR(valid_date, 1, 10)
           ORDER BY fetched_at DESC
         ) AS rn
  FROM forecasts
  WHERE source IN ('bom', 'bom_daily_api')
)
SELECT
  SUBSTR(b.valid_date, 1, 10) AS valid_date,
  b.temp_max AS bom_temp_max,
  d.temp_max AS api_temp_max,
  b.temp_min AS bom_temp_min,
  d.temp_min AS api_temp_min,
  b.precip_range AS bom_precip_range,
  d.precip_range AS api_precip_range,
  b.narrative AS bom_narrative,
  d.narrative_short AS api_short,
  d.narrative_extended AS api_extended
FROM latest b
JOIN latest d ON SUBSTR(b.valid_date, 1, 10) = SUBSTR(d.valid_date, 1, 10)
WHERE b.source = 'bom' AND d.source = 'bom_daily_api'
  AND b.rn = 1 AND d.rn = 1
ORDER BY valid_date DESC;
```

Check ingest health by source:

```sql
SELECT source, endpoint, COUNT(*) AS runs,
       SUM(CASE WHEN success THEN 1 ELSE 0 END) AS ok_runs,
       SUM(CASE WHEN NOT success THEN 1 ELSE 0 END) AS failed_runs,
       SUM(COALESCE(parse_errors, 0)) AS parse_errors
FROM ingest_runs
WHERE started_at > datetime('now', '-14 days')
GROUP BY source, endpoint
ORDER BY source, endpoint;
```

---

## Phase 3: Evaluate Against Observations

### Primary Decision Metric

Use forecast verification against the primary station as the main decision input.

The daily API should only replace the legacy BOM source if it is clearly better, or at least not worse while providing clear secondary benefits.

### Decision Criteria

Switch to the daily API only if it is:

1. better on max/min temperature accuracy, or
2. similar on temperature accuracy and clearly better on rainfall/narrative usefulness, and
3. no worse operationally in stability or completeness

### Reasons to Hold on the Legacy Feed

Stay on the current BOM source if:

- `bom_daily_api` is less accurate against local observations
- the daily API drops important fields too often at our fetch times
- the endpoint shape appears unstable
- the richer fields are not materially useful for display or downstream logic

---

## Phase 4: Controlled Cutover

If `bom_daily_api` wins, transition in two steps.

### Step 1: Promote the Daily API to Live Reads

- Switch current/forecast page reads from `bom` to `bom_daily_api`.
- Keep legacy `bom` ingest running in shadow for 1-2 more weeks.
- Re-check verification and ingest health after the cutover.

### Step 2: Normalize Naming

After the new live path is stable:

- rename legacy `bom` to `bom_ftp`
- rename `bom_daily_api` to `bom` only if we want `bom` to remain the canonical live BOM source name

This second step is optional but keeps the long-term model cleaner.

---

## Pre-Cutover Cleanup

Before switching any live read paths, review the remaining BOM-specific assumptions in the app:

- hardcoded `forecasts["bom"]` reads in current and forecast data builders
- BOM-specific precip fallback logic
- accuracy-page logic that assumes the BOM best lead is D+2
- any tooling or scripts that assume forecast sources are only `wu` and `bom`

These should be updated only when the product decision is made, not during the shadow collection period.

---

## Recommended Next Steps

1. Let shadow ingest run in production for 2-4 weeks.
2. Add one lightweight comparison report or admin page.
3. Review verification and ingest health weekly.
4. Make the go/no-go decision only after several meaningful weather events.
5. If the daily API wins, switch read paths first and rename sources later.

This keeps the current site stable while still building enough evidence to decide whether a full transition is warranted.
