package ingest

import (
	"database/sql"
	"testing"
	"time"

	"github.com/lox/wandiweather/internal/models"
	"github.com/lox/wandiweather/internal/store"

	_ "modernc.org/sqlite"
)

func setupDailyJobsStore(t *testing.T) *store.Store {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	loc, err := time.LoadLocation("Australia/Melbourne")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	st := store.New(db, loc)
	if err := st.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return st
}

func TestBackfillSummaries_UsesDatesFromAllActiveStations(t *testing.T) {
	t.Parallel()

	st := setupDailyJobsStore(t)
	daily := NewDailyJobs(st)

	stationA := models.Station{StationID: "STA", Name: "A", ElevationTier: "valley_floor", IsPrimary: true, Active: true}
	stationB := models.Station{StationID: "STB", Name: "B", ElevationTier: "upper", Active: true}
	if err := st.UpsertStation(stationA); err != nil {
		t.Fatalf("upsert station A: %v", err)
	}
	if err := st.UpsertStation(stationB); err != nil {
		t.Fatalf("upsert station B: %v", err)
	}

	loc, _ := time.LoadLocation("Australia/Melbourne")
	dateA := time.Date(2026, time.January, 10, 12, 0, 0, 0, loc)
	dateB := time.Date(2026, time.January, 11, 12, 0, 0, 0, loc)

	if err := st.InsertObservation(models.Observation{
		StationID:  stationA.StationID,
		ObservedAt: dateA.UTC(),
		Temp:       sql.NullFloat64{Float64: 24.0, Valid: true},
	}); err != nil {
		t.Fatalf("insert A observation: %v", err)
	}
	if err := st.InsertObservation(models.Observation{
		StationID:  stationB.StationID,
		ObservedAt: dateB.UTC(),
		Temp:       sql.NullFloat64{Float64: 12.0, Valid: true},
	}); err != nil {
		t.Fatalf("insert B observation: %v", err)
	}

	if err := daily.BackfillSummaries(); err != nil {
		t.Fatalf("BackfillSummaries: %v", err)
	}

	checkSummary := func(stationID string, wantDate time.Time) {
		t.Helper()

		summaries, err := st.GetDailySummaries(stationID, wantDate.Add(-24*time.Hour), wantDate.Add(24*time.Hour))
		if err != nil {
			t.Fatalf("GetDailySummaries(%s): %v", stationID, err)
		}
		for _, summary := range summaries {
			if summary.Date.Format("2006-01-02") == wantDate.UTC().Format("2006-01-02") {
				return
			}
		}
		t.Fatalf("missing summary for %s on %s", stationID, wantDate.UTC().Format("2006-01-02"))
	}

	checkSummary(stationA.StationID, dateA)
	checkSummary(stationB.StationID, dateB)
}

func TestSchedulerDailyTargetDate_UsesSchedulerLocation(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("UTC+11", 11*60*60)
	s := &Scheduler{loc: loc}

	nowUTC := time.Date(2026, time.January, 5, 13, 30, 0, 0, time.UTC)
	got := s.dailyTargetDate(nowUTC)
	want := nowUTC.In(loc).AddDate(0, 0, -1)

	if !got.Equal(want) {
		t.Fatalf("dailyTargetDate = %s, want %s", got, want)
	}
}
