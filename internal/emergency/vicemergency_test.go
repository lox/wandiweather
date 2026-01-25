package emergency

import (
	"context"
	"testing"
	"time"
)

func TestClient_Fetch(t *testing.T) {
	// Integration test - requires network
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	client := NewClient(-36.794, 146.977, DefaultRadiusKM)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	alerts, err := client.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	t.Logf("Fetched %d alerts within %.0fkm of Wandiligong", len(alerts), DefaultRadiusKM)
	for _, a := range alerts {
		t.Logf("  [%s] %s - %s (%.1fkm)", a.SeverityName(), a.Category, a.Location, a.Distance)
	}
}

func TestHaversine(t *testing.T) {
	// Wandiligong to Melbourne (approx 210km)
	dist := haversine(-36.794, 146.977, -37.8136, 144.9631)
	if dist < 180 || dist > 250 {
		t.Errorf("Expected ~210km, got %.1fkm", dist)
	}

	// Same point
	dist = haversine(-36.794, 146.977, -36.794, 146.977)
	if dist > 0.001 {
		t.Errorf("Expected ~0km for same point, got %.3fkm", dist)
	}
}

func TestParseSeverity(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		expected int
	}{
		{"Emergency Warning", "", SeverityEmergency},
		{"Watch & Act", "", SeverityWatchAct},
		{"Watch and Act", "", SeverityWatchAct},
		{"Advice", "", SeverityAdvice},
		{"Community Information", "", SeverityCommunity},
		{"Something", "Emergency Warning", SeverityEmergency},
		{"Random", "Random", SeverityUnknown},
	}

	for _, tt := range tests {
		got := parseSeverity(tt.name, tt.title)
		if got != tt.expected {
			t.Errorf("parseSeverity(%q, %q) = %d, want %d", tt.name, tt.title, got, tt.expected)
		}
	}
}

func TestCleanHTML(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<p>Hello</p>", "Hello"},
		{"<strong>Bold</strong> text", "Bold text"},
		{"No tags here", "No tags here"},
		{"<div><p>Nested</p></div>", "Nested"},
	}

	for _, tt := range tests {
		got := cleanHTML(tt.input)
		if got != tt.expected {
			t.Errorf("cleanHTML(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
