package store

import (
	"testing"
	"time"

	"github.com/lox/wandiweather/internal/ecowitt"
)

func TestUpsertAirQualityReadings_MergesHistoricalAndLiveData(t *testing.T) {
	store := setupTestStore(t)

	observedAt := time.Date(2026, 4, 30, 4, 0, 0, 0, time.UTC)
	history := ecowitt.AirQualityReading{
		ObservedAt:     observedAt,
		PM25:           32.5,
		SourceFieldKey: "pm25_ch1",
	}
	if _, err := store.UpsertAirQualityReadings([]ecowitt.AirQualityReading{history}); err != nil {
		t.Fatalf("UpsertAirQualityReadings(history): %v", err)
	}

	live := ecowitt.AirQualityReading{
		ObservedAt:     observedAt,
		PM25:           32.5,
		RealTimeAQI:    96,
		HasRealTimeAQI: true,
		AQI24H:         88,
		HasAQI24H:      true,
		PM25Avg24H:     28.3,
		HasPM25Avg24H:  true,
		Category:       "Moderate",
		CategoryClass:  "moderate",
		SourceFieldKey: "pm25_ch1",
	}
	if _, err := store.UpsertAirQualityReadings([]ecowitt.AirQualityReading{live}); err != nil {
		t.Fatalf("UpsertAirQualityReadings(live): %v", err)
	}

	latest, err := store.GetLatestAirQualityReading()
	if err != nil {
		t.Fatalf("GetLatestAirQualityReading: %v", err)
	}
	if latest == nil {
		t.Fatal("GetLatestAirQualityReading returned nil")
	}
	if latest.PM25 != 32.5 {
		t.Fatalf("PM25 = %.1f, want 32.5", latest.PM25)
	}
	if !latest.HasRealTimeAQI || latest.RealTimeAQI != 96 {
		t.Fatalf("RealTimeAQI = %d (valid=%t), want 96 true", latest.RealTimeAQI, latest.HasRealTimeAQI)
	}
	if !latest.HasAQI24H || latest.AQI24H != 88 {
		t.Fatalf("AQI24H = %.1f (valid=%t), want 88 true", latest.AQI24H, latest.HasAQI24H)
	}
	if !latest.HasPM25Avg24H || latest.PM25Avg24H != 28.3 {
		t.Fatalf("PM25Avg24H = %.1f (valid=%t), want 28.3 true", latest.PM25Avg24H, latest.HasPM25Avg24H)
	}
	if latest.Category != "Moderate" || latest.CategoryClass != "moderate" {
		t.Fatalf("category = %q/%q, want Moderate/moderate", latest.Category, latest.CategoryClass)
	}

	readings, err := store.GetAirQualityReadings(observedAt.Add(-time.Minute), observedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("GetAirQualityReadings: %v", err)
	}
	if len(readings) != 1 {
		t.Fatalf("len(readings) = %d, want 1", len(readings))
	}
	if readings[0].RealTimeAQI != 96 {
		t.Fatalf("range RealTimeAQI = %d, want 96", readings[0].RealTimeAQI)
	}
}

func TestUpsertAirQualityReadings_CountsOnlyNewRows(t *testing.T) {
	store := setupTestStore(t)

	observedAt := time.Date(2026, 4, 30, 4, 0, 0, 0, time.UTC)
	history := ecowitt.AirQualityReading{
		ObservedAt:     observedAt,
		PM25:           32.5,
		SourceFieldKey: "pm25_ch1",
	}
	inserted, err := store.UpsertAirQualityReadings([]ecowitt.AirQualityReading{history})
	if err != nil {
		t.Fatalf("UpsertAirQualityReadings(history): %v", err)
	}
	if inserted != 1 {
		t.Fatalf("inserted = %d, want 1", inserted)
	}

	live := ecowitt.AirQualityReading{
		ObservedAt:     observedAt,
		PM25:           32.5,
		RealTimeAQI:    96,
		HasRealTimeAQI: true,
		SourceFieldKey: "pm25_ch1",
	}
	inserted, err = store.UpsertAirQualityReadings([]ecowitt.AirQualityReading{live})
	if err != nil {
		t.Fatalf("UpsertAirQualityReadings(live): %v", err)
	}
	if inserted != 0 {
		t.Fatalf("inserted = %d, want 0 for update-only upsert", inserted)
	}

	second := ecowitt.AirQualityReading{
		ObservedAt:     observedAt.Add(5 * time.Minute),
		PM25:           20.1,
		SourceFieldKey: "pm25_ch1",
	}
	inserted, err = store.UpsertAirQualityReadings([]ecowitt.AirQualityReading{history, second})
	if err != nil {
		t.Fatalf("UpsertAirQualityReadings(mixed): %v", err)
	}
	if inserted != 1 {
		t.Fatalf("inserted = %d, want 1 for one existing and one new row", inserted)
	}
}
