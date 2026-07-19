# Forecast Component Data Regime

> **Created**: July 13, 2026
> **Status**: Active
> **Last reviewed**: July 19, 2026
> **Goal**: Make provider forecasts, component accuracy, and future display decisions measurable without destabilizing the existing daily forecast path.

## Summary

WandiWeather currently presents a useful blend: Weather Underground supplies rain probability and day/night timing, BOM supplies rain amount ranges and hourly detail, and local observations support temperature correction. The existing daily `forecasts` table remains the compatibility surface for the current UI, but it cannot represent provider-neutral hourly or day/night metrics without adding more nullable columns.

The normalized model separates two concerns:

- `forecast_periods` identifies who issued a forecast and when it is valid.
- `forecast_components` stores the typed metric values attached to that period.

Observed rain periods and component verification make raw provider claims measurable. A resolver and displayed-decision log remain deferred until a real display path consumes components; adding unused decision tables now would create schema without an owner.

## Problem

Provider preferences differ by metric. BOM may be the useful source for rain amount, WU for day/night probability, and corrected daily forecasts for temperature. Treating an entire provider row as the decision unit hides that blend and makes later tuning risky.

The data model needs to support:

- Daily, day, night, and hourly validity windows.
- New forecast metrics without adding provider-specific columns.
- Scalar, range, and small text values with explicit units.
- Verification tied to the exact component that made a claim.
- A later resolver that can select components without changing templates or provider ingest formats.

## Goals

- Normalize provider metrics below a shared period identity.
- Preserve the current `forecasts` table and public UI during migration.
- Store WU day/night rain components and BOM hourly components.
- Verify WU rain occurrence against coverage-aware observed day/night totals.
- Keep migration 27 immutable for databases where it is already applied.
- Make invalid metrics, units, ranges, and values fail visibly.

## Non-Goals

- Do not migrate the current homepage or accuracy page in this data-only branch.
- Do not replace existing daily temperature correction or `forecast_verification` yet.
- Do not introduce an unrestricted metric/value table; supported metrics remain a code-owned registry.
- Do not store narratives, icons, or raw provider payload fields as forecast components.
- Do not add decision logging before a resolver actually emits display decisions.
- Do not score exact hourly arrival until hourly observed-period coverage is defined.

## Current Compatibility Path

- WU daily forecasts continue to populate `forecasts` for existing consumers.
- BOM daily API and legacy BOM rows continue to populate `forecasts`.
- WU day/night fields are transient parser values used only to create normalized components.
- `Store.InsertForecast` writes the compatibility row and its normalized periods/components in one transaction.
- BOM hourly forecasts write directly to normalized periods/components because they have no legacy daily-row consumer.
- Existing API handlers, templates, and accuracy-page queries remain unchanged.

## Target Model

### Forecast Periods

Periods contain temporal and provider identity only:

```sql
CREATE TABLE forecast_periods (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    forecast_id INTEGER REFERENCES forecasts(id),
    source TEXT NOT NULL,
    fetched_at DATETIME NOT NULL,
    valid_date DATE NOT NULL,
    day_of_forecast INTEGER NOT NULL CHECK (day_of_forecast >= 0),
    period TEXT NOT NULL CHECK (period IN ('daily', 'day', 'night', 'hourly')),
    period_start DATETIME NOT NULL,
    period_end DATETIME NOT NULL CHECK (period_end > period_start),
    is_night BOOLEAN NOT NULL DEFAULT FALSE,
    location_id TEXT,
    raw_period_key TEXT NOT NULL
);
```

The unique identity is provider, fetch time, period start, period kind, and location. `forecast_id` is nullable because hourly provider periods do not need a compatibility `forecasts` row.

### Forecast Components

Components contain one supported metric per period:

```sql
CREATE TABLE forecast_components (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    forecast_period_id INTEGER NOT NULL REFERENCES forecast_periods(id) ON DELETE CASCADE,
    metric TEXT NOT NULL,
    value REAL,
    value_min REAL,
    value_max REAL,
    value_text TEXT,
    unit TEXT,
    UNIQUE(forecast_period_id, metric)
);
```

The database enforces numeric-or-text shape, non-inverted ranges, percentage bounds, and non-negative precipitation/wind values. Application validation additionally rejects unknown metrics, wrong units, duplicate metrics, and range use on scalar-only metrics.

The initial metric registry is:

- `precip_chance` (`percent`)
- `precip_amount` (`mm`, scalar or range)
- `temperature`, `feels_like`, `dewpoint` (`celsius`)
- `humidity` (`percent`)
- `wind_speed`, `wind_gust` (`km/h`)
- `wind_direction` (text)

Adding a metric requires a code constant and validation rule, but not a schema migration unless the metric needs a new storage shape.

### Observed Periods

Rain verification uses explicit local-time windows:

```sql
CREATE TABLE observed_periods (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    station_id TEXT NOT NULL,
    valid_date DATE NOT NULL,
    period TEXT NOT NULL CHECK (period IN ('daily', 'day', 'night')),
    period_start DATETIME NOT NULL,
    period_end DATETIME NOT NULL CHECK (period_end > period_start),
    precip_total REAL CHECK (precip_total >= 0),
    observation_count INTEGER NOT NULL CHECK (observation_count >= 0),
    coverage_minutes INTEGER NOT NULL CHECK (coverage_minutes >= 0),
    is_complete BOOLEAN NOT NULL DEFAULT FALSE,
    computed_at DATETIME NOT NULL,
    UNIQUE(station_id, valid_date, period)
);
```

Melbourne-local boundaries are daily 00:00-00:00, day 06:00-18:00, and night 18:00-06:00 the next day.

### Component Verification

Verification references the component rather than copying provider, period, and forecast values:

```sql
CREATE TABLE forecast_component_verification (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    forecast_component_id INTEGER NOT NULL REFERENCES forecast_components(id) ON DELETE CASCADE,
    observed_period_id INTEGER NOT NULL REFERENCES observed_periods(id),
    verification_kind TEXT NOT NULL,
    actual_value REAL NOT NULL,
    forecast_threshold REAL NOT NULL CHECK (forecast_threshold >= 0),
    actual_threshold REAL NOT NULL CHECK (actual_threshold >= 0),
    verifier_version TEXT NOT NULL,
    hit_class TEXT NOT NULL CHECK (hit_class IN ('hit', 'false_alarm', 'miss', 'correct_dry')),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(forecast_component_id, observed_period_id, verification_kind, verifier_version)
);
```

Rain occurrence selects precipitation chance when available, otherwise precipitation amount. A chance of at least 30% or forecast amount of at least 0.2 mm predicts rain; observed rain is more than 0.2 mm.

The verifier version is part of row identity so later threshold experiments create comparable history instead of overwriting earlier classifications.

### Resolver Boundary

The future resolver should consume components and emit the existing view model. Initial rules remain explicit, for example BOM amount plus WU timing. Decision logging should be introduced with that resolver so its schema records the actual selected component IDs and has an active writer and reader.

## Safety Principles

- Migration 28 must upgrade both an empty database and a real migration-27 database transactionally.
- Migration 28 must clear legacy `displayed_forecasts` references whose forecast rows were deleted by earlier cleanup migrations, while preserving valid references.
- Historical backfill must store daily components rather than inventing unavailable day/night splits.
- Period and component insertion must be atomic.
- Runtime SQLite connections must enable foreign-key enforcement so component and verification references cannot become orphaned.
- Duplicate provider periods may be replayed safely, while invalid values must return errors.
- Observed periods fail closed when coverage is incomplete or a gauge reset cannot be reconstructed safely.
- Existing daily verification and public accuracy output remain readable throughout the transition.

## Progress Snapshot

- The local branch is data-only; API handlers, templates, and public accuracy output are unchanged.
- Migration 28 creates periods, components, observed periods, and component verification, then consumes and drops migration 27's four transitional columns.
- Migration 28 repairs pre-existing orphaned display-history references before runtime foreign-key enforcement is enabled.
- Existing forecast rain history is backfilled into `precip_chance` and `precip_amount` components.
- WU ingest writes day/night precipitation components atomically with daily compatibility rows.
- BOM daily rows write daily precipitation components, including amount ranges.
- BOM hourly ingest writes 73 hourly periods with temperature, moisture, rain, and wind components.
- Rain timing verification reads normalized components directly without a compatibility fallback or per-forecast follow-up query.
- Public component accuracy, the resolver, and displayed-decision logging remain deferred.

## Delivery Strategy

### Slice 1: Component Storage and Provider Ingest (implemented locally)

- Add period identity and component value tables.
- Backfill existing daily rain values without inventing precision.
- Dual-write WU day/night and BOM daily components.
- Ingest BOM hourly components.
- Keep existing `GetLatestForecasts` behavior unchanged.

### Slice 2: Observed Rain and Component Verification (implemented locally)

- Compute coverage-aware daily/day/night rain totals for the primary station.
- Handle cumulative-gauge resets across midnight.
- Verify the selected WU precipitation component idempotently.
- Suppress scores for incomplete or unsafe observation windows.

### Slice 3: Resolver and Decision Logging (deferred)

- Add a component resolver that emits the current view-model contract.
- Start with explicit provider preference rules.
- Add decision tables only with the resolver, keyed to every selected component.
- Preserve existing UI output while comparing old and new resolution paths.

### Slice 4: Accuracy Page Split (data-gated)

- Separate provider-component accuracy from displayed-decision accuracy.
- Keep legacy daily verification visible during transition.
- Show sample counts and incomplete-window warnings.
- Do not publish timing claims until at least ten complete verified periods exist.

## Verification

Repository checks:

```bash
go test ./...
go vet ./...
go test -race ./internal/store ./internal/ingest
git diff --check
```

Migration checks:

- Run all migrations against an empty in-memory database.
- Upgrade a copy of `data/wandiweather-live.db` from migration 27 to 28.
- Confirm `pragma_foreign_key_check` reports no violations after the upgrade.
- Confirm the four transitional `forecasts` columns are gone.
- Confirm `forecast_periods` has no metric columns.
- Confirm component counts and shapes after historical backfill.
- Run one live ingest against the copied database and inspect hourly metric counts.

Representative queries:

```sql
SELECT period.source, period.period, component.metric, COUNT(*)
FROM forecast_periods period
JOIN forecast_components component ON component.forecast_period_id = period.id
GROUP BY period.source, period.period, component.metric;

SELECT period.source, period.period, component.metric,
       verification.hit_class, COUNT(*)
FROM forecast_component_verification verification
JOIN forecast_components component ON component.id = verification.forecast_component_id
JOIN forecast_periods period ON period.id = component.forecast_period_id
GROUP BY period.source, period.period, component.metric, verification.hit_class;
```

## Key Learnings From Pressure-Testing

- A single component table that repeats period metadata would make hourly data wasteful and allow inconsistent windows. Period identity plus child components keeps both concerns explicit.
- A wide period table is initially convenient but recreates schema churn and nullable-column growth as providers add metrics.
- An unrestricted EAV table is too permissive. A code-owned metric registry and database shape constraints catch typos and invalid units while retaining extensibility.
- Decision logging without a resolver is speculative schema. It is deferred until both the write path and accuracy consumer are defined.
- Component verification should reference immutable forecast facts rather than duplicate source, date, and forecast values that can drift.
- Raw payloads remain useful evidence but cannot be the sole backfill source because their history is incomplete and deduplicated.

## Resolved Decisions

- Keep `forecast_periods` as the temporal/provider envelope and `forecast_components` as the metric layer.
- Store scalar, range, and small text components; keep narratives, icons, and raw metadata outside the abstraction.
- Keep supported metric names and units in code.
- Preserve migration 27 and consume its transitional columns in migration 28.
- Use Melbourne-local day/night windows, with night crossing midnight.
- Require 95% observed coverage, ignore gaps over 90 minutes, and fail closed when a reset follows a gap over 15 minutes.
- Prefer precipitation chance as the rain-occurrence signal and fall back to amount only when chance is unavailable.
- Use a 30% chance threshold and 0.2 mm amount/observation threshold for initial rain-occurrence verification.
- Keep legacy daily forecast and accuracy paths unchanged until component history is sufficient.

## Deferred Questions

- How to represent resolver decisions that combine several components into one display value.
- How hourly observed rain coverage and arrival-time error should be scored.
- Whether component verification should eventually use a small station-network aggregate rather than only the primary station.
- When enough component history exists to retire legacy daily verification fields.
