package store

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/lox/wandiweather/internal/models"
)

func setupTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	loc, err := time.LoadLocation("Australia/Melbourne")
	if err != nil {
		t.Fatalf("load timezone: %v", err)
	}
	store := New(db, loc)
	if err := store.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return store
}

func TestUpsertAndGetStation(t *testing.T) {
	store := setupTestStore(t)

	station := models.Station{
		StationID:     "TEST001",
		Name:          "Test Station",
		Latitude:      -36.794,
		Longitude:     146.977,
		Elevation:     400.0,
		ElevationTier: "mid_slope",
		IsPrimary:     true,
		Active:        true,
	}

	if err := store.UpsertStation(station); err != nil {
		t.Fatalf("UpsertStation: %v", err)
	}

	stations, err := store.GetActiveStations()
	if err != nil {
		t.Fatalf("GetActiveStations: %v", err)
	}
	if len(stations) != 1 {
		t.Fatalf("len(stations) = %d, want 1", len(stations))
	}
	if stations[0].StationID != "TEST001" {
		t.Errorf("StationID = %q, want TEST001", stations[0].StationID)
	}
	if stations[0].Name != "Test Station" {
		t.Errorf("Name = %q, want 'Test Station'", stations[0].Name)
	}

	primary, err := store.GetPrimaryStation()
	if err != nil {
		t.Fatalf("GetPrimaryStation: %v", err)
	}
	if primary == nil {
		t.Fatal("GetPrimaryStation returned nil")
	}
	if primary.StationID != "TEST001" {
		t.Errorf("Primary StationID = %q, want TEST001", primary.StationID)
	}
}

func TestUpsertStation_Update(t *testing.T) {
	store := setupTestStore(t)

	station := models.Station{
		StationID: "TEST001",
		Name:      "Original Name",
		Active:    true,
	}
	if err := store.UpsertStation(station); err != nil {
		t.Fatalf("UpsertStation: %v", err)
	}

	station.Name = "Updated Name"
	if err := store.UpsertStation(station); err != nil {
		t.Fatalf("UpsertStation update: %v", err)
	}

	stations, err := store.GetActiveStations()
	if err != nil {
		t.Fatalf("GetActiveStations: %v", err)
	}
	if len(stations) != 1 {
		t.Fatalf("len(stations) = %d, want 1", len(stations))
	}
	if stations[0].Name != "Updated Name" {
		t.Errorf("Name = %q, want 'Updated Name'", stations[0].Name)
	}
}

func TestGetActiveStations_FilterInactive(t *testing.T) {
	store := setupTestStore(t)

	if err := store.UpsertStation(models.Station{StationID: "ACTIVE", Active: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertStation(models.Station{StationID: "INACTIVE", Active: false}); err != nil {
		t.Fatal(err)
	}

	stations, err := store.GetActiveStations()
	if err != nil {
		t.Fatalf("GetActiveStations: %v", err)
	}
	if len(stations) != 1 {
		t.Fatalf("len(stations) = %d, want 1", len(stations))
	}
	if stations[0].StationID != "ACTIVE" {
		t.Errorf("StationID = %q, want ACTIVE", stations[0].StationID)
	}
}

func TestInsertAndGetObservation(t *testing.T) {
	store := setupTestStore(t)

	if err := store.UpsertStation(models.Station{StationID: "TEST001", Active: true}); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	obs := models.Observation{
		StationID:  "TEST001",
		ObservedAt: now,
		Temp:       sql.NullFloat64{Float64: 25.5, Valid: true},
		Humidity:   sql.NullInt64{Int64: 65, Valid: true},
		Pressure:   sql.NullFloat64{Float64: 1015.2, Valid: true},
		QCStatus:   1,
		ObsType:    models.ObsTypeInstant,
	}

	if err := store.InsertObservation(obs); err != nil {
		t.Fatalf("InsertObservation: %v", err)
	}

	latest, err := store.GetLatestObservation("TEST001")
	if err != nil {
		t.Fatalf("GetLatestObservation: %v", err)
	}
	if latest == nil {
		t.Fatal("GetLatestObservation returned nil")
	}
	if !latest.Temp.Valid || latest.Temp.Float64 != 25.5 {
		t.Errorf("Temp = %v, want 25.5", latest.Temp)
	}
	if !latest.Humidity.Valid || latest.Humidity.Int64 != 65 {
		t.Errorf("Humidity = %v, want 65", latest.Humidity)
	}
}

func TestInsertObservation_NoDuplicate(t *testing.T) {
	store := setupTestStore(t)

	if err := store.UpsertStation(models.Station{StationID: "TEST001", Active: true}); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	obs1 := models.Observation{
		StationID:  "TEST001",
		ObservedAt: now,
		Temp:       sql.NullFloat64{Float64: 20.0, Valid: true},
		ObsType:    models.ObsTypeInstant,
	}
	obs2 := models.Observation{
		StationID:  "TEST001",
		ObservedAt: now,
		Temp:       sql.NullFloat64{Float64: 25.0, Valid: true},
		ObsType:    models.ObsTypeInstant,
	}

	if err := store.InsertObservation(obs1); err != nil {
		t.Fatalf("InsertObservation first: %v", err)
	}
	if err := store.InsertObservation(obs2); err != nil {
		t.Fatalf("InsertObservation second: %v", err)
	}

	latest, err := store.GetLatestObservation("TEST001")
	if err != nil {
		t.Fatalf("GetLatestObservation: %v", err)
	}
	if latest.Temp.Float64 != 20.0 {
		t.Errorf("Temp = %v, want 20.0 (first insert wins with ON CONFLICT DO NOTHING)", latest.Temp.Float64)
	}
}

func TestGetObservations_DateRange(t *testing.T) {
	store := setupTestStore(t)

	if err := store.UpsertStation(models.Station{StationID: "TEST001", Active: true}); err != nil {
		t.Fatal(err)
	}

	baseTime := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		obs := models.Observation{
			StationID:  "TEST001",
			ObservedAt: baseTime.Add(time.Duration(i) * time.Hour),
			Temp:       sql.NullFloat64{Float64: float64(20 + i), Valid: true},
			ObsType:    models.ObsTypeInstant,
		}
		if err := store.InsertObservation(obs); err != nil {
			t.Fatal(err)
		}
	}

	start := baseTime.Add(1 * time.Hour)
	end := baseTime.Add(3 * time.Hour)
	observations, err := store.GetObservations("TEST001", start, end)
	if err != nil {
		t.Fatalf("GetObservations: %v", err)
	}
	if len(observations) != 3 {
		t.Fatalf("len(observations) = %d, want 3", len(observations))
	}
}

func TestGetCleanObservations(t *testing.T) {
	store := setupTestStore(t)

	if err := store.UpsertStation(models.Station{StationID: "TEST001", Active: true}); err != nil {
		t.Fatal(err)
	}

	baseTime := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	goodObs := models.Observation{
		StationID:  "TEST001",
		ObservedAt: baseTime,
		Temp:       sql.NullFloat64{Float64: 25.0, Valid: true},
		QCStatus:   1,
		ObsType:    models.ObsTypeInstant,
	}
	if err := store.InsertObservation(goodObs); err != nil {
		t.Fatal(err)
	}

	badQCObs := models.Observation{
		StationID:  "TEST001",
		ObservedAt: baseTime.Add(1 * time.Hour),
		Temp:       sql.NullFloat64{Float64: 26.0, Valid: true},
		QCStatus:   5,
		ObsType:    models.ObsTypeInstant,
	}
	if err := store.InsertObservation(badQCObs); err != nil {
		t.Fatal(err)
	}

	flaggedObs := models.Observation{
		StationID:    "TEST001",
		ObservedAt:   baseTime.Add(2 * time.Hour),
		Temp:         sql.NullFloat64{Float64: 60.0, Valid: true},
		QCStatus:     0,
		ObsType:      models.ObsTypeInstant,
		QualityFlags: sql.NullString{String: `["temp_out_of_range"]`, Valid: true},
	}
	if err := store.InsertObservation(flaggedObs); err != nil {
		t.Fatal(err)
	}

	unknownTypeObs := models.Observation{
		StationID:  "TEST001",
		ObservedAt: baseTime.Add(3 * time.Hour),
		Temp:       sql.NullFloat64{Float64: 24.0, Valid: true},
		QCStatus:   0,
		ObsType:    models.ObsTypeUnknown,
	}
	if err := store.InsertObservation(unknownTypeObs); err != nil {
		t.Fatal(err)
	}

	start := baseTime.Add(-1 * time.Hour)
	end := baseTime.Add(5 * time.Hour)
	cleanObs, err := store.GetCleanObservations("TEST001", start, end)
	if err != nil {
		t.Fatalf("GetCleanObservations: %v", err)
	}

	if len(cleanObs) != 1 {
		t.Fatalf("len(cleanObs) = %d, want 1 (only good observation)", len(cleanObs))
	}
	if cleanObs[0].Temp.Float64 != 25.0 {
		t.Errorf("cleanObs[0].Temp = %v, want 25.0", cleanObs[0].Temp.Float64)
	}
}

func TestInsertAndGetForecast(t *testing.T) {
	store := setupTestStore(t)

	fetchedAt := time.Now().UTC().Truncate(time.Second)
	validDate := time.Now().UTC().Add(24 * time.Hour).Truncate(24 * time.Hour)

	wuForecast := models.Forecast{
		Source:        "wu",
		FetchedAt:     fetchedAt,
		ValidDate:     validDate,
		DayOfForecast: 1,
		TempMax:       sql.NullFloat64{Float64: 28.0, Valid: true},
		TempMin:       sql.NullFloat64{Float64: 15.0, Valid: true},
		Narrative:     sql.NullString{String: "Partly cloudy", Valid: true},
		LocationID:    sql.NullString{String: "-36.794,146.977", Valid: true},
	}
	if err := store.InsertForecast(wuForecast); err != nil {
		t.Fatalf("InsertForecast WU: %v", err)
	}

	bomForecast := models.Forecast{
		Source:        "bom",
		FetchedAt:     fetchedAt,
		ValidDate:     validDate,
		DayOfForecast: 1,
		TempMax:       sql.NullFloat64{Float64: 27.0, Valid: true},
		TempMin:       sql.NullFloat64{Float64: 14.0, Valid: true},
		PrecipRange:   sql.NullString{String: "0 to 1 mm", Valid: true},
		LocationID:    sql.NullString{String: "VIC_PT075", Valid: true},
	}
	if err := store.InsertForecast(bomForecast); err != nil {
		t.Fatalf("InsertForecast BOM: %v", err)
	}

	forecasts, err := store.GetLatestForecasts()
	if err != nil {
		t.Fatalf("GetLatestForecasts: %v", err)
	}

	if len(forecasts["wu"]) == 0 {
		t.Fatal("No WU forecasts returned")
	}
	if len(forecasts["bom"]) == 0 {
		t.Fatal("No BOM forecasts returned")
	}
	if forecasts["wu"][0].TempMax.Float64 != 28.0 {
		t.Errorf("WU TempMax = %v, want 28.0", forecasts["wu"][0].TempMax.Float64)
	}
	if forecasts["bom"][0].TempMax.Float64 != 27.0 {
		t.Errorf("BOM TempMax = %v, want 27.0", forecasts["bom"][0].TempMax.Float64)
	}
}

func TestIngestRun_StartAndComplete(t *testing.T) {
	store := setupTestStore(t)

	stationID := "TEST001"
	run, err := store.StartIngestRun("wu", "pws/observations/current", &stationID, nil)
	if err != nil {
		t.Fatalf("StartIngestRun: %v", err)
	}
	if run.ID == 0 {
		t.Error("run.ID should be set")
	}
	if run.Source != "wu" {
		t.Errorf("run.Source = %q, want 'wu'", run.Source)
	}

	run.HTTPStatus = sql.NullInt64{Int64: 200, Valid: true}
	run.ResponseSizeBytes = sql.NullInt64{Int64: 1024, Valid: true}
	run.RecordsParsed = sql.NullInt64{Int64: 1, Valid: true}
	run.RecordsStored = sql.NullInt64{Int64: 1, Valid: true}
	run.Success = true

	if err := store.CompleteIngestRun(run); err != nil {
		t.Fatalf("CompleteIngestRun: %v", err)
	}

	health, err := store.GetIngestHealth(1)
	if err != nil {
		t.Fatalf("GetIngestHealth: %v", err)
	}
	if len(health) == 0 {
		t.Fatal("No health summaries returned")
	}

	found := false
	for _, h := range health {
		if h.Source == "wu" && h.Endpoint == "pws/observations/current" {
			found = true
			if h.SuccessRuns != 1 {
				t.Errorf("SuccessRuns = %d, want 1", h.SuccessRuns)
			}
		}
	}
	if !found {
		t.Error("Expected health summary for wu/pws/observations/current")
	}
}

func TestIngestRun_GetRecentErrors(t *testing.T) {
	store := setupTestStore(t)

	run, err := store.StartIngestRun("wu", "forecast/daily/5day", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	run.HTTPStatus = sql.NullInt64{Int64: 500, Valid: true}
	run.Success = false
	run.ErrorMessage = sql.NullString{String: "server error", Valid: true}
	if err := store.CompleteIngestRun(run); err != nil {
		t.Fatal(err)
	}

	errors, err := store.GetRecentIngestErrors(10)
	if err != nil {
		t.Fatalf("GetRecentIngestErrors: %v", err)
	}
	if len(errors) != 1 {
		t.Fatalf("len(errors) = %d, want 1", len(errors))
	}
	if errors[0].ErrorMessage.String != "server error" {
		t.Errorf("ErrorMessage = %q, want 'server error'", errors[0].ErrorMessage.String)
	}
}

func TestMigrationVersion(t *testing.T) {
	store := setupTestStore(t)

	version, err := store.MigrationVersion()
	if err != nil {
		t.Fatalf("MigrationVersion: %v", err)
	}
	if version < 1 {
		t.Errorf("MigrationVersion = %d, want >= 1", version)
	}
}

func TestGetLatestObservation_NoData(t *testing.T) {
	store := setupTestStore(t)

	if err := store.UpsertStation(models.Station{StationID: "EMPTY", Active: true}); err != nil {
		t.Fatal(err)
	}

	latest, err := store.GetLatestObservation("EMPTY")
	if err != nil {
		t.Fatalf("GetLatestObservation: %v", err)
	}
	if latest != nil {
		t.Error("Expected nil for station with no observations")
	}
}

func TestGetPrimaryStation_None(t *testing.T) {
	store := setupTestStore(t)

	if err := store.UpsertStation(models.Station{StationID: "NOTPRIMARY", IsPrimary: false, Active: true}); err != nil {
		t.Fatal(err)
	}

	primary, err := store.GetPrimaryStation()
	if err != nil {
		t.Fatalf("GetPrimaryStation: %v", err)
	}
	if primary != nil {
		t.Error("Expected nil when no primary station exists")
	}
}

func TestGetLatestObservation_ReturnsLatest(t *testing.T) {
	store := setupTestStore(t)

	if err := store.UpsertStation(models.Station{StationID: "TEST001", Active: true}); err != nil {
		t.Fatal(err)
	}

	baseTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

	olderObs := models.Observation{
		StationID:  "TEST001",
		ObservedAt: baseTime,
		Temp:       sql.NullFloat64{Float64: 20.0, Valid: true},
		ObsType:    models.ObsTypeInstant,
	}
	if err := store.InsertObservation(olderObs); err != nil {
		t.Fatal(err)
	}

	newerObs := models.Observation{
		StationID:  "TEST001",
		ObservedAt: baseTime.Add(1 * time.Hour),
		Temp:       sql.NullFloat64{Float64: 25.0, Valid: true},
		ObsType:    models.ObsTypeInstant,
	}
	if err := store.InsertObservation(newerObs); err != nil {
		t.Fatal(err)
	}

	latest, err := store.GetLatestObservation("TEST001")
	if err != nil {
		t.Fatalf("GetLatestObservation: %v", err)
	}
	if latest == nil {
		t.Fatal("GetLatestObservation returned nil")
	}
	if latest.Temp.Float64 != 25.0 {
		t.Errorf("Latest observation Temp = %v, want 25.0 (newer observation)", latest.Temp.Float64)
	}
}

func TestGetObservations_InclusiveDateRange(t *testing.T) {
	store := setupTestStore(t)

	if err := store.UpsertStation(models.Station{StationID: "TEST001", Active: true}); err != nil {
		t.Fatal(err)
	}

	times := []time.Time{
		time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 15, 11, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC),
	}

	for i, t := range times {
		obs := models.Observation{
			StationID:  "TEST001",
			ObservedAt: t,
			Temp:       sql.NullFloat64{Float64: float64(20 + i), Valid: true},
			ObsType:    models.ObsTypeInstant,
		}
		if err := store.InsertObservation(obs); err != nil {
			panic(err)
		}
	}

	observations, err := store.GetObservations("TEST001", times[0], times[2])
	if err != nil {
		t.Fatalf("GetObservations: %v", err)
	}
	if len(observations) != 3 {
		t.Fatalf("len(observations) = %d, want 3 (inclusive range)", len(observations))
	}
	if !observations[0].ObservedAt.Equal(times[0]) {
		t.Errorf("First observation time = %v, want %v", observations[0].ObservedAt, times[0])
	}
	if !observations[2].ObservedAt.Equal(times[2]) {
		t.Errorf("Last observation time = %v, want %v", observations[2].ObservedAt, times[2])
	}
}

func TestIngestHealth_Aggregation(t *testing.T) {
	store := setupTestStore(t)

	stationID := "TEST001"

	successRun, err := store.StartIngestRun("wu", "pws/observations/current", &stationID, nil)
	if err != nil {
		t.Fatal(err)
	}
	successRun.HTTPStatus = sql.NullInt64{Int64: 200, Valid: true}
	successRun.RecordsStored = sql.NullInt64{Int64: 1, Valid: true}
	successRun.Success = true
	if err := store.CompleteIngestRun(successRun); err != nil {
		t.Fatal(err)
	}

	failedRun, err := store.StartIngestRun("wu", "pws/observations/current", &stationID, nil)
	if err != nil {
		t.Fatal(err)
	}
	failedRun.HTTPStatus = sql.NullInt64{Int64: 500, Valid: true}
	failedRun.Success = false
	failedRun.ErrorMessage = sql.NullString{String: "server error", Valid: true}
	if err := store.CompleteIngestRun(failedRun); err != nil {
		t.Fatal(err)
	}

	health, err := store.GetIngestHealth(1)
	if err != nil {
		t.Fatalf("GetIngestHealth: %v", err)
	}

	var found bool
	for _, h := range health {
		if h.Source == "wu" && h.Endpoint == "pws/observations/current" {
			found = true
			if h.TotalRuns != 2 {
				t.Errorf("TotalRuns = %d, want 2", h.TotalRuns)
			}
			if h.SuccessRuns != 1 {
				t.Errorf("SuccessRuns = %d, want 1", h.SuccessRuns)
			}
			if h.FailedRuns != 1 {
				t.Errorf("FailedRuns = %d, want 1", h.FailedRuns)
			}
		}
	}
	if !found {
		t.Error("Expected health summary for wu/pws/observations/current")
	}
}
