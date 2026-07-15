# BOM Daily API Transition Plan

> **Created**: April 30, 2026
>
> **Status**: Phase 4 complete; daily API live, explicit source names retained
>
> **Goal**: Collect `bom_daily_api` alongside the current BOM feed, compare them against observations and each other, and decide whether the daily API should replace the legacy BOM product.

## Overview

This plan treats the BOM daily API as a product evaluation exercise rather than a straight migration. The legacy FTP/XML feed (`bom`) was kept live while the richer daily API was collected in parallel as `bom_daily_api`; Phase 4 promotes the daily API to the live BOM read path while keeping legacy ingest as a rollback/shadow source.

**Current live source**: `bom_daily_api` in production for current/forecast page BOM reads

**Shadow/fallback source**: `bom` (legacy FTP/XML Wangaratta area product)

**Current state**: Both products are scheduled on the same tick. Current/forecast page reads prefer `bom_daily_api`, with legacy `bom` retained as fallback and shadow ingest. The production 403 issue on the daily API path was mitigated by forcing IPv4 for `bom_daily_api`; the one-week post-cutover window stayed clean. Keep explicit source names rather than renaming `bom_daily_api` to canonical `bom`.

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

### Production Check-In: May 9, 2026

Pulled the latest production database with `task pull-db` and checked the current `/health` endpoint. Production was healthy at check time; all stations were fresh.

Shadow collection is active, but still early:

- First `bom_daily_api` forecast row: `2026-05-02 01:36:19 UTC`.
- Latest `bom_daily_api` forecast row in the pulled database: `2026-05-09 07:00:03 UTC`.
- `bom_daily_api` has 232 forecast rows across 8 fetch days and 15 valid dates.
- Latest fetch for both sources covers `2026-05-09` through `2026-05-16`.

Ingest health since the shadow path started:

- `bom_daily_api`: 30 runs, 30 successful, 0 failures, 0 parse errors.
- `bom` legacy FTP: 30 runs, 29 successful, 1 failure, 0 parse errors.
- The legacy failure was on `2026-05-06 01:00 UTC` with an FTP `xferlog` error. The daily API succeeded during that same scheduler tick.

Field completeness for all collected `bom_daily_api` rows:

- `temp_max`: 232/232
- `temp_min`: 216/232
- `precip_chance`: 232/232
- `precip_range`: 232/232
- `precip_min`: 232/232
- `precip_max`: 109/232
- `narrative_short`: 232/232
- `narrative_extended`: 231/232

Nearest-fetch comparison against legacy BOM, pairing daily API rows to legacy rows for the same valid date within 5 minutes:

- 225 paired forecast rows; 7 daily API rows were unpaired because the legacy FTP ingest failed.
- Max temperature: 210 comparable rows, 0 differences >= 1 C.
- Min temperature: 195 comparable rows, 2 differences >= 1 C.
- Rain chance: 10 paired rows differed by >= 20 percentage points.
- Rain value presence: 0 differences.
- Severe keywords appeared only in the daily API extended narrative for 5 paired rows, all around the `2026-05-03` thunderstorm/heavy-rain event. The legacy short narrative only said `Rain.` for those rows.

Verification against observations is not decision-grade yet:

- Same-valid-date and same-lead paired verification has only 21 rows across `2026-05-03` through `2026-05-08`.
- Max temperature MAE is effectively tied: legacy BOM `2.14 C`, daily API `2.19 C`.
- Min temperature MAE is effectively tied: legacy BOM `2.86 C`, daily API `2.81 C`.

Current decision: keep collecting. The daily API is operationally healthy and appears to preserve materially richer rainfall and severe-weather narrative detail, but the verification sample is too small to justify a cutover. Re-check after the 2-week minimum window completes on May 16, 2026, and prefer a final decision only after the 4-week window if weather events remain sparse.

### Production Check-In: June 21, 2026

Pulled the latest production database with `task pull-db` after confirming `/health` was `ok`; all stations were fresh at check time.

Shadow collection now has enough verification data to make a product-quality assessment:

- First `bom_daily_api` forecast row: `2026-05-02 01:36:19 UTC`.
- Latest successful `bom_daily_api` forecast row: `2026-06-20 01:00:03 UTC`.
- Latest legacy `bom` forecast row: `2026-06-21 07:00:02 UTC`.
- `bom_daily_api` has 1,484 forecast rows across 50 fetch days and 56 valid dates.
- Same-valid-date and same-lead verification has 322 paired rows across 49 valid days, from `2026-05-03` through `2026-06-20`.

Temperature accuracy is effectively identical:

- Overall paired max-temperature MAE: legacy BOM `1.81 C`, daily API `1.81 C`.
- Overall paired min-temperature MAE: legacy BOM `2.27 C`, daily API `2.27 C`.
- Lead-by-lead differences are negligible; daily API is within `0.04 C` of legacy BOM on every lead-time MAE bucket.
- Nearest-fetch source comparison had 1,477 paired rows. Max temps differed by at least `1 C` in only 22 rows; min temps differed by at least `1 C` in only 17 rows.

Rainfall payload quality is better in the daily API, but verified amount accuracy is not materially different when both sources have an amount:

- `bom_daily_api` has precipitation amount/range data on 1,484/1,484 rows.
- Legacy `bom` has precipitation amount/range data on 810/1,508 rows over the same collection period.
- In paired verification, daily API had precipitation amounts for 322/322 rows; legacy BOM had amounts for 209/322 rows.
- Where both sources had a precipitation amount, rain/no-rain classification was identical: 137 hits, 72 false alarms, 0 misses, 0 correct-dry rows.
- Where both sources had a precipitation amount, all-row precipitation MAE was legacy BOM `4.69 mm` and daily API `4.72 mm`.
- Across all paired rows where daily API had an amount, daily API produced 112 correct-dry rows that legacy BOM could not score because legacy had no precipitation amount.
- Rain chance differs more often than temperature: 59 paired rows had daily API and legacy BOM chance-of-rain values differ by at least 20 percentage points.
- Severe-weather keywords appeared in only one source for 46 paired rows, usually because daily API carries extended narrative text that legacy does not.

Operational stability is now the blocker:

- Since shadow ingest started, legacy `bom` has 201/202 successful ingest runs and 0 parse errors.
- Since shadow ingest started, `bom_daily_api` has 197/202 successful ingest runs and 0 parse errors.
- All five `bom_daily_api` failures are recent HTTP 403 responses from the BOM daily API endpoint: `2026-06-20 07:00 UTC`, `2026-06-20 13:00 UTC`, `2026-06-20 19:00 UTC`, `2026-06-21 01:00 UTC`, and `2026-06-21 07:00 UTC`.
- Legacy `bom` succeeded on those same scheduler ticks.
- A direct HTTPS request to `https://api.weather.bom.gov.au/v1/locations/r3811m/forecasts/daily` succeeded from the local development machine during this check, so the failure appears to be production egress or request-shape related rather than a schema/parser break.
- Follow-up probes from the Fly machine showed browser-style headers still returned 403 on the default path. Direct HTTPS to the Fly-resolved IPv4 Akamai address returned 200, while direct HTTPS to the Fly-resolved IPv6 Akamai address returned 403. This points to the BOM/Akamai IPv6 path from Fly being blocked.
- Mitigation selected: force IPv4 only for the `bom_daily_api` HTTP client. Keep this scoped to the daily API path; do not change other HTTP clients.
- The IPv4-only client was deployed on June 21, 2026 as image `deployment-01KVMWA6KF84QRT2ZN92E3V0CN`. The startup forecast ingest immediately succeeded and inserted 8 `bom_daily_api` forecast days.
- June 22, 2026 follow-up: the first four post-deploy BOM forecast ingests all succeeded for both `bom` and `bom_daily_api` with 0 parse errors. Runs at `2026-06-21 10:43 UTC`, `2026-06-21 13:00 UTC`, `2026-06-21 19:00 UTC`, and `2026-06-22 01:00 UTC` stored 30 `bom_daily_api` forecast rows total. Latest `bom_daily_api` fetch now covers `2026-06-22` through `2026-06-28`.

### Production Check-In: June 30, 2026

Pulled the latest production database with `task pull-db` after confirming `/health` was `ok`; all stations were fresh at check time.

The IPv4 mitigation has held:

- Post-deploy `bom_daily_api` ingest runs since `2026-06-21 10:43 UTC`: 37/37 successful, 0 failures, 0 parse errors.
- Last 7 days since `2026-06-23 00:00 UTC`: 30/30 `bom_daily_api` runs successful, 0 failures, 0 parse errors.
- Latest `bom_daily_api` fetch: `2026-06-30 07:00:03 UTC`, storing 9 rows and covering `2026-06-30` through `2026-07-08`.
- Latest legacy `bom` fetch: `2026-06-30 07:00:01 UTC`, storing 8 rows and covering `2026-06-30` through `2026-07-07`.

Updated product comparison:

- `bom_daily_api` has 1,780 forecast rows across 60 fetch days and 68 valid dates.
- Same-valid-date and same-lead verification now has 384 paired rows across 58 valid days, from `2026-05-03` through `2026-06-29`.
- Temperature remains effectively tied: max-temperature MAE is legacy BOM `1.88 C`, daily API `1.88 C`; min-temperature MAE is legacy BOM `2.39 C`, daily API `2.39 C`.
- Rainfall field completeness remains materially better in the daily API: daily API precipitation amount/range data exists for 1,762 rows, while legacy BOM has it for 957 rows over the same collection period.
- In paired verification, daily API had precipitation amounts for 384/384 rows; legacy BOM had amounts for 234/384 rows.
- Where both sources had precipitation amounts, rain/no-rain classification remained nearly identical: legacy BOM had 151 hits, 83 false alarms, 0 misses, and 0 correct-dry rows; daily API had 151 hits, 82 false alarms, 0 misses, and 1 correct-dry row.
- Where both sources had precipitation amounts, all-row precipitation MAE was legacy BOM `4.48 mm` and daily API `4.50 mm`.
- Across all paired rows where daily API had an amount, daily API recorded 150 correct-dry rows that legacy BOM could not score because legacy had no precipitation amount.

Current decision: proceed to Phase 4 Step 1 when ready. The daily API is not worse on temperature accuracy, is operationally stable after the IPv4 fix, and is clearly better for rainfall/narrative completeness. Promote live reads to `bom_daily_api`, keep legacy `bom` ingest running in shadow for 1-2 weeks, then re-check before any source renaming.

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

Implementation started on June 30, 2026:

- Added a live BOM source selector that prefers `bom_daily_api` per valid date and fills missing dates from legacy `bom`.
- Switched the current and multi-day forecast data builders to use the live BOM selector.
- Made precipitation range fallback source-aware: try the live BOM source first, then legacy `bom`, then WU amount.
- Updated BOM temperature correction paths to use the actual forecast row source for correction and nowcast lookup, while preserving the user-facing `BOM` label.
- Added API, forecast-helper, and store tests covering daily API live reads, legacy fallback, source-aware precipitation fallback, and source-specific bias correction.
- Left the accuracy page and source renaming unchanged for Step 2/post-cutover review.

Production cutover on July 4, 2026:

- Deployed image `deployment-01KWNNRVYGS48KR26H0AVAF4WZ` to Fly.io.
- Fly machine checks passed and `/health` returned `ok`; all active stations were fresh immediately after deploy.
- `/api/forecast` returned `BOM.Source = bom_daily_api` for all displayed forecast days (`2026-07-04` through `2026-07-08` at smoke-check time).
- Startup ingest succeeded and stored 7 legacy `bom` forecast days and 8 `bom_daily_api` forecast days.
- Fresh production DB pull showed last-24h ingest health: `bom` 5/5 successful, `bom_daily_api` 5/5 successful, WU forecast 5/5 successful, WU observations 1152/1152 successful, all with 0 failures and 0 parse errors.
- Latest forecast coverage after deploy: legacy `bom` covered `2026-07-04` through `2026-07-10`; `bom_daily_api` covered `2026-07-04` through `2026-07-11`; WU covered `2026-07-04` through `2026-07-09`.
- Monitoring window is now active. Keep legacy `bom` shadow ingest running through at least July 11, 2026 before deciding whether to normalize source names.
- A one-shot heartbeat automation, `bom-cutover-24h-check`, is active for the 24-hour production recheck. Its handoff includes scheduling the one-week follow-up after a clean check, because this thread can only have one active heartbeat automation at a time.

Same-day follow-up at `2026-07-04 14:28 AEST`:

- `/health` remained `ok`; all active stations were fresh.
- `/api/forecast` still returned `BOM.Source = bom_daily_api` for all displayed forecast days.
- `/api/current` rendered the daily API-backed BOM forecast path successfully.
- Recent logs showed the post-deploy startup, successful WU forecast ingest, successful legacy `bom` ingest, successful `bom_daily_api` ingest, and normal observation polling. The only notable log entries were benign SSH reply EOFs from inspection sessions.
- A fresh DB pull at `2026-07-04 14:30 AEST` still showed clean last-24h ingest health: legacy `bom` 5/5 successful, `bom_daily_api` 5/5 successful, WU forecast 5/5 successful, WU observations 1152/1152 successful, and Ecowitt observations 288/288 successful, all with 0 failures and 0 parse errors.
- Latest pulled forecast coverage remained healthy: legacy `bom` covered `2026-07-04` through `2026-07-10`, `bom_daily_api` covered `2026-07-04` through `2026-07-11`, and WU covered `2026-07-04` through `2026-07-09`.

One-week follow-up at `2026-07-13 23:19 AEST`:

- `/health` remained `ok`; all active stations were fresh.
- `/api/forecast` still returned `BOM.Source = bom_daily_api` for all displayed forecast days, with live BOM rainfall ranges populated for the current rain event.
- `/api/current` rendered the daily API-backed BOM forecast path successfully, including `PrecipDisplay = 3–5mm` for `2026-07-13`.
- Fly was still running image `deployment-01KWNNRVYGS48KR26H0AVAF4WZ`.
- Recent production logs showed normal observation polling and a successful `bom_daily_api` forecast ingest at `2026-07-13 23:00 AEST`; the error filter found no recent errors, failures, panics, stale warnings, 403s, or parse errors.
- Fresh production DB pull showed clean ingest health since `2026-07-04`: legacy `bom` 40/40 successful, `bom_daily_api` 40/40 successful, WU forecast 40/40 successful, WU observations 11008/11008 successful, and Ecowitt observations 2752/2752 successful, all with 0 failures and 0 parse errors.
- Latest forecast coverage remained healthy: legacy `bom` covered `2026-07-13` through `2026-07-20`, `bom_daily_api` covered `2026-07-13` through `2026-07-21`, and WU covered `2026-07-13` through `2026-07-18`.
- Paired verification for valid dates `2026-07-04` through `2026-07-12` had 63 same-valid-date/same-lead rows. Temperature accuracy was identical: max-temperature MAE was legacy `bom` `2.27 C` and `bom_daily_api` `2.27 C`; min-temperature MAE was legacy `bom` `1.73 C` and `bom_daily_api` `1.73 C`.
- Rainfall completeness remained materially better in the daily API: in the same 63 paired verification rows, `bom_daily_api` had precipitation amounts for 63/63 rows while legacy `bom` had them for 19/63 rows.
- Where both products had precipitation amounts, rainfall amount MAE and rain/no-rain classification were identical: both had `3.32 mm` MAE, 14 hits, 5 false alarms, 0 misses, and 0 correct-dry rows.
- Across all daily API rows with precipitation amounts, `bom_daily_api` had `1.00 mm` precipitation MAE, 14 hits, 5 false alarms, 0 misses, and 44 correct-dry rows that legacy `bom` could not score because it had no amount.
- Latest paired source comparison after cutover found 17 paired valid dates: max/min temperatures differed by at least `1 C` on 0 dates, rain chance differed by at least 20 points on 2 dates, and `bom_daily_api` had a precipitation range where legacy had none on 15 dates.

### Step 2: Normalize Naming

Decision after the one-week post-cutover review: do not normalize source names now.

Keep `bom_daily_api` explicit as the live BOM product and keep legacy `bom` explicit as the FTP/XML fallback/shadow source. Renaming `bom_daily_api` to canonical `bom` would make historical verification and rollback analysis less obvious, while the current UI already preserves the user-facing `BOM` label.

This leaves a possible future cleanup only if we later decide to retire the legacy FTP/XML feed entirely.

---

## Cutover Assumption Audit

Phase 4 Step 1 addressed these BOM-specific assumptions:

- hardcoded `forecasts["bom"]` reads in current and forecast data builders now use the live BOM selector
- BOM-specific precip fallback logic now checks the live source first and keeps legacy `bom` as fallback

These assumptions remain intentionally unchanged because the source names are staying explicit:

- accuracy-page logic that assumes the BOM best lead is D+2
- any tooling or scripts that assume forecast sources are only `wu` and `bom`

---

## Recommended Next Steps

1. Keep `bom_daily_api` as the live BOM read source.
2. Keep explicit source names: `bom_daily_api` for the daily API product and `bom` for the legacy FTP/XML product.
3. Leave legacy `bom` ingest running as fallback/shadow unless a separate cleanup explicitly retires it.
4. Treat any future source-name migration as a separate data migration, not part of this cutover.

The transition is complete: the daily API is live, verified stable, at least as accurate for temperature, and materially better for rainfall completeness.
