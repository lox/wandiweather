package store

import (
	"database/sql"
	"testing"
	"time"

	"github.com/lox/wandiweather/internal/models"
)

func TestGetVerificationForecasts_UsesEarliestUsefulRow(t *testing.T) {
	store := setupTestStore(t)

	validDate := time.Date(2026, time.January, 18, 0, 0, 0, 0, time.UTC)
	cutoff := time.Date(validDate.Year(), validDate.Month(), validDate.Day(), 0, 0, 0, 0, store.loc).UTC()

	wuEarly := models.Forecast{
		Source:        "wu",
		FetchedAt:     cutoff.Add(-6 * time.Hour),
		ValidDate:     validDate,
		DayOfForecast: 1,
		TempMin:       sql.NullFloat64{Float64: 9.5, Valid: true},
		PrecipAmount:  sql.NullFloat64{Float64: 2.1, Valid: true},
	}
	if err := store.InsertForecast(wuEarly); err != nil {
		t.Fatalf("insert wu early: %v", err)
	}

	wuLater := models.Forecast{
		Source:        "wu",
		FetchedAt:     cutoff.Add(-3 * time.Hour),
		ValidDate:     validDate,
		DayOfForecast: 1,
		TempMax:       sql.NullFloat64{Float64: 23.0, Valid: true},
		TempMin:       sql.NullFloat64{Float64: 10.0, Valid: true},
	}
	if err := store.InsertForecast(wuLater); err != nil {
		t.Fatalf("insert wu later: %v", err)
	}

	bomPartial := models.Forecast{
		Source:        "bom",
		FetchedAt:     cutoff.Add(-4 * time.Hour),
		ValidDate:     validDate,
		DayOfForecast: 2,
		TempMin:       sql.NullFloat64{Float64: 8.0, Valid: true},
	}
	if err := store.InsertForecast(bomPartial); err != nil {
		t.Fatalf("insert bom: %v", err)
	}

	forecasts, err := store.GetVerificationForecasts(validDate)
	if err != nil {
		t.Fatalf("GetVerificationForecasts: %v", err)
	}

	if len(forecasts) != 2 {
		t.Fatalf("len(forecasts) = %d, want 2", len(forecasts))
	}

	var wuFound, bomFound bool
	for _, fc := range forecasts {
		switch {
		case fc.Source == "wu" && fc.DayOfForecast == 1:
			wuFound = true
			if !fc.TempMin.Valid {
				t.Fatalf("WU day-1 temp_min should be preserved")
			}
			if fc.TempMax.Valid {
				t.Fatalf("WU day-1 should keep earliest partial row, expected null max temp")
			}
			if fc.FetchedAt != wuEarly.FetchedAt {
				t.Fatalf("WU fetched_at = %s, want earliest useful %s", fc.FetchedAt, wuEarly.FetchedAt)
			}
		case fc.Source == "bom" && fc.DayOfForecast == 2:
			bomFound = true
			if !fc.TempMin.Valid {
				t.Fatalf("BOM day-2 temp_min should be preserved")
			}
		}
	}

	if !wuFound {
		t.Fatal("missing WU day-1 forecast")
	}
	if !bomFound {
		t.Fatal("missing BOM day-2 forecast")
	}
}
