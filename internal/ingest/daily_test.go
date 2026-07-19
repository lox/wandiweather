package ingest

import (
	"database/sql"
	"testing"
	"time"

	"github.com/lox/wandiweather/internal/models"
	"github.com/lox/wandiweather/internal/store"
	_ "modernc.org/sqlite"
)

func setupDailyJobsTest(t *testing.T) (*DailyJobs, *store.Store, *time.Location) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	loc, err := time.LoadLocation("Australia/Melbourne")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	s := store.New(db, loc)
	if err := s.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := s.UpsertStation(models.Station{StationID: "PRIMARY", IsPrimary: true, Active: true}); err != nil {
		t.Fatalf("upsert primary station: %v", err)
	}
	return NewDailyJobs(s), s, loc
}

func TestVerifyRainTiming_SkipsIncompleteObservationPeriods(t *testing.T) {
	jobs, s, loc := setupDailyJobsTest(t)
	validDate := time.Date(2026, time.July, 10, 0, 0, 0, 0, loc)

	if err := s.InsertForecast(models.Forecast{
		Source:            "wu",
		FetchedAt:         validDate.Add(-24 * time.Hour),
		ValidDate:         validDate,
		DayOfForecast:     1,
		TempMax:           sql.NullFloat64{Float64: 12, Valid: true},
		PrecipChanceNight: sql.NullInt64{Int64: 80, Valid: true},
		PrecipAmountNight: sql.NullFloat64{Float64: 3, Valid: true},
	}); err != nil {
		t.Fatalf("insert forecast: %v", err)
	}
	for _, hour := range []int{18, 19, 6} {
		observedAt := time.Date(2026, time.July, 10, hour, 0, 0, 0, loc)
		if hour == 6 {
			observedAt = observedAt.AddDate(0, 0, 1)
		}
		if err := s.InsertObservation(models.Observation{
			StationID:   "PRIMARY",
			ObservedAt:  observedAt.UTC(),
			PrecipTotal: sql.NullFloat64{Float64: 1, Valid: true},
		}); err != nil {
			t.Fatalf("insert observation: %v", err)
		}
	}

	if err := jobs.VerifyRainTiming(validDate); err != nil {
		t.Fatalf("VerifyRainTiming: %v", err)
	}
	rows, err := s.GetForecastComponentVerifications(validDate)
	if err != nil {
		t.Fatalf("GetForecastComponentVerifications: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("created %d verification rows for incomplete observations, want 0", len(rows))
	}

	period, err := s.GetObservedPeriod("PRIMARY", validDate, "night")
	if err != nil {
		t.Fatalf("GetObservedPeriod: %v", err)
	}
	if period == nil || period.CoverageMinutes >= 684 {
		t.Fatalf("night coverage = %+v, want persisted but incomplete", period)
	}
}

func TestVerifyRainTiming_DoesNotRequireTemperatureForecast(t *testing.T) {
	jobs, s, loc := setupDailyJobsTest(t)
	validDate := time.Date(2026, time.July, 10, 0, 0, 0, 0, loc)

	if err := s.InsertForecast(models.Forecast{
		Source:          "wu",
		FetchedAt:       validDate.Add(-24 * time.Hour),
		ValidDate:       validDate,
		DayOfForecast:   1,
		PrecipChanceDay: sql.NullInt64{Int64: 80, Valid: true},
		PrecipAmountDay: sql.NullFloat64{Float64: 1, Valid: true},
	}); err != nil {
		t.Fatalf("insert forecast: %v", err)
	}

	periodStart := time.Date(2026, time.July, 10, 6, 0, 0, 0, loc)
	for i := 0; i <= 72; i++ {
		if err := s.InsertObservation(models.Observation{
			StationID:   "PRIMARY",
			ObservedAt:  periodStart.Add(time.Duration(i) * 10 * time.Minute).UTC(),
			PrecipTotal: sql.NullFloat64{Float64: float64(i) / 72, Valid: true},
		}); err != nil {
			t.Fatalf("insert observation %d: %v", i, err)
		}
	}

	if err := jobs.VerifyRainTiming(validDate); err != nil {
		t.Fatalf("VerifyRainTiming: %v", err)
	}
	rows, err := s.GetForecastComponentVerifications(validDate)
	if err != nil {
		t.Fatalf("GetForecastComponentVerifications: %v", err)
	}
	if len(rows) != 1 || rows[0].Period != "day" || rows[0].HitClass != "hit" {
		t.Fatalf("verification rows = %+v, want one daytime hit", rows)
	}
}

func TestVerifyRainTiming_SkipsPeriodWithUnsafeGaugeReset(t *testing.T) {
	jobs, s, loc := setupDailyJobsTest(t)
	validDate := time.Date(2026, time.July, 10, 0, 0, 0, 0, loc)

	if err := s.InsertForecast(models.Forecast{
		Source:            "wu",
		FetchedAt:         validDate.Add(-24 * time.Hour),
		ValidDate:         validDate,
		DayOfForecast:     1,
		PrecipChanceNight: sql.NullInt64{Int64: 80, Valid: true},
	}); err != nil {
		t.Fatalf("insert forecast: %v", err)
	}

	periodStart := time.Date(2026, time.July, 10, 18, 0, 0, 0, loc)
	for i := 0; i <= 72; i++ {
		if i == 36 {
			continue
		}
		total := 2 + float64(i)/100
		if i > 36 {
			total = float64(i-36) / 100
		}
		if err := s.InsertObservation(models.Observation{
			StationID:   "PRIMARY",
			ObservedAt:  periodStart.Add(time.Duration(i) * 10 * time.Minute).UTC(),
			PrecipTotal: sql.NullFloat64{Float64: total, Valid: true},
		}); err != nil {
			t.Fatalf("insert observation %d: %v", i, err)
		}
	}

	if err := jobs.VerifyRainTiming(validDate); err != nil {
		t.Fatalf("VerifyRainTiming: %v", err)
	}
	rows, err := s.GetForecastComponentVerifications(validDate)
	if err != nil {
		t.Fatalf("GetForecastComponentVerifications: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("created %d verification rows across an unsafe reset, want 0", len(rows))
	}
}
