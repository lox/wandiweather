package store

import (
	"database/sql"
	"math"
	"testing"
	"time"

	"github.com/lox/wandiweather/internal/models"
)

func TestForecastSchemaStoresMetricsAsComponents(t *testing.T) {
	store := setupTestStore(t)
	rows, err := store.db.Query("PRAGMA table_info(forecasts)")
	if err != nil {
		t.Fatalf("query forecast columns: %v", err)
	}
	defer rows.Close()

	compatibilityColumns := map[string]bool{
		"precip_chance_day":   true,
		"precip_chance_night": true,
		"precip_amount_day":   true,
		"precip_amount_night": true,
	}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan forecast column: %v", err)
		}
		if compatibilityColumns[name] {
			t.Errorf("forecasts contains transitional column %q", name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate forecast columns: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close forecast columns: %v", err)
	}

	rows, err = store.db.Query("PRAGMA table_info(forecast_periods)")
	if err != nil {
		t.Fatalf("query forecast period columns: %v", err)
	}
	defer rows.Close()

	componentColumns := map[string]bool{
		"precip_chance":     true,
		"precip_amount":     true,
		"precip_amount_min": true,
		"precip_amount_max": true,
		"temp":              true,
		"wind_speed":        true,
		"icon_descriptor":   true,
	}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan forecast period column: %v", err)
		}
		if componentColumns[name] {
			t.Errorf("forecast_periods contains component column %q", name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate forecast period columns: %v", err)
	}

	var componentTableCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'forecast_components'`).Scan(&componentTableCount); err != nil {
		t.Fatalf("query forecast_components table: %v", err)
	}
	if componentTableCount != 1 {
		t.Fatalf("forecast_components table count = %d, want 1", componentTableCount)
	}
}

func TestForecastComponentSchemaMigrationVersion(t *testing.T) {
	store := setupTestStore(t)

	version, err := store.MigrationVersion()
	if err != nil {
		t.Fatalf("MigrationVersion: %v", err)
	}
	if version != 28 {
		t.Errorf("MigrationVersion = %d, want 28", version)
	}
}

func TestMigration28BackfillsForecastComponents(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	loc, err := time.LoadLocation("Australia/Melbourne")
	if err != nil {
		t.Fatalf("load timezone: %v", err)
	}
	store := New(db, loc)
	if err := store.ensureMigrationsTable(); err != nil {
		t.Fatalf("ensure migrations table: %v", err)
	}
	for _, migration := range migrations {
		if migration.Version >= 28 {
			break
		}
		applyMigrationForTest(t, store, migration, migration.Description)
	}

	fetchedAt := time.Date(2026, time.July, 19, 8, 0, 0, 0, time.UTC)
	validDate := time.Date(2026, time.July, 20, 0, 0, 0, 0, time.UTC)
	if _, err := db.Exec(`
		INSERT INTO forecasts (
			source, fetched_at, valid_date, day_of_forecast,
			precip_chance_day, precip_chance_night,
			precip_amount_day, precip_amount_night
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, "wu", fetchedAt, validDate, 1, 20, 70, 0.2, 4.8); err != nil {
		t.Fatalf("insert WU forecast: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO forecasts (
			source, fetched_at, valid_date, day_of_forecast,
			precip_min, precip_max, precip_units
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "bom_daily_api", fetchedAt, validDate, 1, 1.0, 5.0, "mm"); err != nil {
		t.Fatalf("insert BOM forecast: %v", err)
	}

	applyMigrationForTest(t, store, migrations[len(migrations)-1], migrations[len(migrations)-1].Description)

	assertScalarComponent := func(source, period, metric string, want float64) {
		t.Helper()
		var got float64
		if err := db.QueryRow(`
			SELECT component.value
			FROM forecast_components component
			JOIN forecast_periods period ON period.id = component.forecast_period_id
			WHERE period.source = ? AND period.period = ? AND component.metric = ?
		`, source, period, metric).Scan(&got); err != nil {
			t.Fatalf("load %s/%s/%s component: %v", source, period, metric, err)
		}
		if got != want {
			t.Fatalf("%s/%s/%s = %v, want %v", source, period, metric, got, want)
		}
	}
	assertScalarComponent("wu", "day", models.ForecastMetricPrecipChance, 20)
	assertScalarComponent("wu", "night", models.ForecastMetricPrecipAmount, 4.8)

	var amountMin, amountMax float64
	if err := db.QueryRow(`
		SELECT component.value_min, component.value_max
		FROM forecast_components component
		JOIN forecast_periods period ON period.id = component.forecast_period_id
		WHERE period.source = 'bom_daily_api'
		  AND period.period = 'daily'
		  AND component.metric = 'precip_amount'
	`).Scan(&amountMin, &amountMax); err != nil {
		t.Fatalf("load BOM precipitation range: %v", err)
	}
	if amountMin != 1 || amountMax != 5 {
		t.Fatalf("BOM precipitation range = %v-%v, want 1-5", amountMin, amountMax)
	}
}

func TestComputeObservedRainPeriod_HandlesMidnightReset(t *testing.T) {
	store := setupTestStore(t)
	loc, err := time.LoadLocation("Australia/Melbourne")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	validDate := time.Date(2026, time.July, 10, 0, 0, 0, 0, loc)
	stationID := "TEST001"
	totals := []float64{3, 3.2, 3.4, 3.6, 3.8, 4, 0.2, 0.4, 0.6, 0.8, 1, 1.2, 1.2}
	for i, total := range totals {
		observedAt := validDate.Add(time.Duration(18+i) * time.Hour)
		if err := store.InsertObservation(models.Observation{
			StationID:  stationID,
			ObservedAt: observedAt.UTC(),
			PrecipTotal: sql.NullFloat64{
				Float64: total,
				Valid:   true,
			},
		}); err != nil {
			t.Fatalf("insert observation at %s: %v", observedAt, err)
		}
	}

	period, err := store.ComputeObservedRainPeriod(stationID, validDate, "night")
	if err != nil {
		t.Fatalf("ComputeObservedRainPeriod: %v", err)
	}
	if period == nil {
		t.Fatal("ComputeObservedRainPeriod returned nil")
	}
	if !period.PrecipTotal.Valid || math.Abs(period.PrecipTotal.Float64-2.2) > 0.001 {
		t.Fatalf("PrecipTotal = %+v, want 2.2", period.PrecipTotal)
	}
	if period.ObservationCount != 13 {
		t.Fatalf("ObservationCount = %d, want 13", period.ObservationCount)
	}
	if period.CoverageMinutes != 660 {
		t.Fatalf("CoverageMinutes = %d, want 660 because an hourly reset gap is incomplete", period.CoverageMinutes)
	}
}

func TestForecastComponentVerification_IsIdempotent(t *testing.T) {
	store := setupTestStore(t)
	validDate := time.Date(2026, time.July, 10, 0, 0, 0, 0, time.UTC)

	if err := store.InsertForecast(models.Forecast{
		Source:            "wu",
		FetchedAt:         validDate.Add(-24 * time.Hour),
		ValidDate:         validDate,
		DayOfForecast:     1,
		TempMax:           sql.NullFloat64{Float64: 12, Valid: true},
		PrecipChanceNight: sql.NullInt64{Int64: 70, Valid: true},
		PrecipAmountNight: sql.NullFloat64{Float64: 4.8, Valid: true},
	}); err != nil {
		t.Fatalf("InsertForecast: %v", err)
	}
	periods, err := store.GetLatestForecastPeriods("wu", "night", validDate.Add(-12*time.Hour), validDate.AddDate(0, 0, 2), 10)
	if err != nil || len(periods) != 1 {
		t.Fatalf("GetLatestForecastPeriods = %d, %v", len(periods), err)
	}
	chance, ok := periods[0].Component(models.ForecastMetricPrecipChance)
	if !ok {
		t.Fatal("night period has no precipitation chance component")
	}
	observed := models.ObservedPeriod{
		StationID:        "TEST001",
		ValidDate:        validDate,
		Period:           "night",
		PeriodStart:      periods[0].PeriodStart,
		PeriodEnd:        periods[0].PeriodEnd,
		PrecipTotal:      sql.NullFloat64{Float64: 2.2, Valid: true},
		ObservationCount: 72,
		CoverageMinutes:  720,
		IsComplete:       true,
		ComputedAt:       time.Now().UTC(),
	}
	if err := store.UpsertObservedPeriod(observed); err != nil {
		t.Fatalf("UpsertObservedPeriod: %v", err)
	}
	storedObserved, err := store.GetObservedPeriod(observed.StationID, validDate, observed.Period)
	if err != nil || storedObserved == nil {
		t.Fatalf("GetObservedPeriod = %+v, %v", storedObserved, err)
	}

	verification := models.ForecastComponentVerification{
		ForecastComponentID: chance.ID,
		ObservedPeriodID:    storedObserved.ID,
		VerificationKind:    models.ForecastVerificationRainOccurrence,
		ActualValue:         sql.NullFloat64{Float64: 2.2, Valid: true},
		ForecastThreshold:   30,
		ActualThreshold:     0.2,
		VerifierVersion:     models.ForecastVerifierRainOccurrenceV1,
		HitClass:            "hit",
	}
	if err := store.UpsertForecastComponentVerification(verification); err != nil {
		t.Fatalf("first UpsertForecastComponentVerification: %v", err)
	}
	verification.HitClass = "false_alarm"
	if err := store.UpsertForecastComponentVerification(verification); err != nil {
		t.Fatalf("second UpsertForecastComponentVerification: %v", err)
	}

	rows, err := store.GetForecastComponentVerifications(validDate)
	if err != nil {
		t.Fatalf("GetForecastComponentVerifications: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].HitClass != "false_alarm" {
		t.Fatalf("HitClass = %q, want updated false_alarm", rows[0].HitClass)
	}

	verification.VerifierVersion = "rain-occurrence-v2"
	verification.ForecastThreshold = 40
	verification.HitClass = "hit"
	if err := store.UpsertForecastComponentVerification(verification); err != nil {
		t.Fatalf("versioned UpsertForecastComponentVerification: %v", err)
	}
	rows, err = store.GetForecastComponentVerifications(validDate)
	if err != nil {
		t.Fatalf("GetForecastComponentVerifications after version change: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(versioned rows) = %d, want 2", len(rows))
	}

	if err := store.DeleteForecastComponentVerifications(
		storedObserved.ID,
		models.ForecastVerificationRainOccurrence,
		models.ForecastVerifierRainOccurrenceV1,
	); err != nil {
		t.Fatalf("DeleteForecastComponentVerifications: %v", err)
	}
	rows, err = store.GetForecastComponentVerifications(validDate)
	if err != nil {
		t.Fatalf("GetForecastComponentVerifications after delete: %v", err)
	}
	if len(rows) != 1 || rows[0].VerifierVersion != "rain-occurrence-v2" {
		t.Fatalf("versioned rows after V1 delete = %+v, want only V2", rows)
	}
}
