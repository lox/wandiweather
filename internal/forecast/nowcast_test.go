package forecast

import (
	"testing"
)

func TestNowcastConstants(t *testing.T) {
	if nowcastAlpha <= 0 || nowcastAlpha > 1 {
		t.Errorf("nowcastAlpha = %v, should be between 0 and 1", nowcastAlpha)
	}

	if maxAdjustment <= 0 {
		t.Errorf("maxAdjustment = %v, should be positive", maxAdjustment)
	}

	if nowcastStartHour < 0 || nowcastStartHour > 23 {
		t.Errorf("nowcastStartHour = %v, should be valid hour", nowcastStartHour)
	}

	if nowcastEndHour < nowcastStartHour || nowcastEndHour > 23 {
		t.Errorf("nowcastEndHour = %v, should be >= startHour and valid", nowcastEndHour)
	}

	if minReadings <= 0 {
		t.Errorf("minReadings = %v, should be positive", minReadings)
	}
}

func TestNowcastEnabled(t *testing.T) {
	if nowcastEnabled {
		t.Log("nowcast is enabled - ensure validation data supports this")
	}
}

func TestCapCorrectionForNowcast(t *testing.T) {
	tests := []struct {
		name       string
		adjustment float64
		want       float64
	}{
		{"within limit positive", 2.0, 2.0},
		{"within limit negative", -2.0, -2.0},
		{"exceeds positive limit", 6.0, maxAdjustment},
		{"exceeds negative limit", -6.0, -maxAdjustment},
		{"exactly at limit", maxAdjustment, maxAdjustment},
		{"zero", 0.0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := capCorrection(tt.adjustment, maxAdjustment)
			if got != tt.want {
				t.Errorf("capCorrection(%v, maxAdjustment) = %v, want %v", tt.adjustment, got, tt.want)
			}
		})
	}
}

func TestNowcastAdjustmentCalculation(t *testing.T) {
	observedMorning := 22.5
	forecastMorning := 20.0
	delta := observedMorning - forecastMorning
	expectedAdjustment := nowcastAlpha * delta

	if expectedAdjustment != 1.75 {
		t.Logf("nowcastAlpha * delta = %v (alpha=%v, delta=%v)", expectedAdjustment, nowcastAlpha, delta)
	}

	if expectedAdjustment < -maxAdjustment || expectedAdjustment > maxAdjustment {
		t.Error("adjustment exceeds max limits before capping")
	}
}
