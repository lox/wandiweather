package store

import (
	"testing"
	"time"

	"github.com/lox/wandiweather/internal/emergency"
)

func TestUpsertAlert_ConflictRefreshesSeverityAndMetadata(t *testing.T) {
	store := setupTestStore(t)

	now := time.Now().Add(-10 * time.Minute).Truncate(time.Second)
	original := emergency.Alert{
		ID:          "alert-1",
		Category:    "Fire",
		SubCategory: "Bushfire",
		Name:        "Advice",
		Status:      "Going",
		Location:    "Upper Valley",
		Distance:    12.2,
		Severity:    emergency.SeverityAdvice,
		Lat:         -36.70,
		Lon:         146.95,
		Headline:    "Initial headline",
		Body:        "Initial body",
		URL:         "https://example.com/initial",
		Created:     now.Add(-2 * time.Hour),
		Updated:     now.Add(-2 * time.Hour),
	}

	if err := store.UpsertAlert(original, now); err != nil {
		t.Fatalf("insert alert: %v", err)
	}

	updated := original
	updated.Category = "Flood"
	updated.SubCategory = "Riverine"
	updated.Name = "Emergency Warning"
	updated.Status = "Emergency"
	updated.Location = "Lower Valley"
	updated.Distance = 4.8
	updated.Severity = emergency.SeverityEmergency
	updated.Lat = -36.81
	updated.Lon = 147.03
	updated.Headline = "Updated headline"
	updated.Body = "Updated body"
	updated.URL = "https://example.com/updated"
	updated.Updated = now.Add(10 * time.Minute)

	if err := store.UpsertAlert(updated, now.Add(5*time.Minute)); err != nil {
		t.Fatalf("update alert: %v", err)
	}

	alerts, err := store.GetActiveAlerts(24 * time.Hour)
	if err != nil {
		t.Fatalf("GetActiveAlerts: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("len(alerts) = %d, want 1", len(alerts))
	}

	got := alerts[0]
	if got.Severity != updated.Severity {
		t.Fatalf("Severity = %d, want %d", got.Severity, updated.Severity)
	}
	if got.Location != updated.Location {
		t.Fatalf("Location = %q, want %q", got.Location, updated.Location)
	}
	if got.Distance != updated.Distance {
		t.Fatalf("Distance = %.1f, want %.1f", got.Distance, updated.Distance)
	}
	if got.Category != updated.Category {
		t.Fatalf("Category = %q, want %q", got.Category, updated.Category)
	}
	if got.SubCategory != updated.SubCategory {
		t.Fatalf("SubCategory = %q, want %q", got.SubCategory, updated.SubCategory)
	}
	if got.URL != updated.URL {
		t.Fatalf("URL = %q, want %q", got.URL, updated.URL)
	}
}
