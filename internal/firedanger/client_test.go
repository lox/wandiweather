package firedanger

import (
	"context"
	"testing"
	"time"
)

func TestClient_Fetch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	client := NewNorthEastClient()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	forecasts, err := client.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if len(forecasts) == 0 {
		t.Fatal("Expected at least one forecast day")
	}

	t.Logf("Fetched %d fire danger forecasts for North East district", len(forecasts))
	for _, f := range forecasts {
		tfb := ""
		if f.TotalFireBan {
			tfb = " [TOTAL FIRE BAN]"
		}
		t.Logf("  %s: %s%s", f.Date.Format("Mon 02 Jan"), f.Rating, tfb)
	}
}

func TestParseItemDate(t *testing.T) {
	tests := []struct {
		title string
		want  string
		ok    bool
	}{
		{"Sunday, 25 January 2026", "2026-01-25", true},
		{"Monday, 5 February 2026", "2026-02-05", true},
		{"Fire restrictions by municipality", "", false},
	}

	for _, tt := range tests {
		date, ok := parseItemDate(tt.title)
		if ok != tt.ok {
			t.Errorf("parseItemDate(%q) ok = %v, want %v", tt.title, ok, tt.ok)
			continue
		}
		if ok && date.Format("2006-01-02") != tt.want {
			t.Errorf("parseItemDate(%q) = %s, want %s", tt.title, date.Format("2006-01-02"), tt.want)
		}
	}
}

func TestRating_Severity(t *testing.T) {
	if RatingCatastrophic.Severity() <= RatingExtreme.Severity() {
		t.Error("Catastrophic should be more severe than Extreme")
	}
	if RatingExtreme.Severity() <= RatingHigh.Severity() {
		t.Error("Extreme should be more severe than High")
	}
	if RatingHigh.Severity() <= RatingModerate.Severity() {
		t.Error("High should be more severe than Moderate")
	}
	if RatingModerate.Severity() <= RatingNone.Severity() {
		t.Error("Moderate should be more severe than None")
	}
}
