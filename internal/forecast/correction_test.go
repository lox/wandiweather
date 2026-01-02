package forecast

import (
	"testing"
)

func TestCapCorrection(t *testing.T) {
	tests := []struct {
		name       string
		correction float64
		limit      float64
		want       float64
	}{
		{"within positive limit", 5.0, 8.0, 5.0},
		{"within negative limit", -5.0, 8.0, -5.0},
		{"at positive limit", 8.0, 8.0, 8.0},
		{"at negative limit", -8.0, 8.0, -8.0},
		{"exceeds positive limit", 12.0, 8.0, 8.0},
		{"exceeds negative limit", -12.0, 8.0, -8.0},
		{"zero correction", 0.0, 8.0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := capCorrection(tt.correction, tt.limit)
			if got != tt.want {
				t.Errorf("capCorrection(%v, %v) = %v, want %v", tt.correction, tt.limit, got, tt.want)
			}
		})
	}
}

func TestTotalCorrectionClamping(t *testing.T) {
	tests := []struct {
		name             string
		rawMax           float64
		biasMax          float64
		nowcastAdj       float64
		wantCorrectedMax float64
	}{
		{
			name:             "no clamping needed - small positive correction",
			rawMax:           25.0,
			biasMax:          0.0,
			nowcastAdj:       3.0,
			wantCorrectedMax: 28.0,
		},
		{
			name:             "no clamping needed - small negative correction",
			rawMax:           25.0,
			biasMax:          0.0,
			nowcastAdj:       -3.0,
			wantCorrectedMax: 22.0,
		},
		{
			name:             "clamp large positive correction to +10",
			rawMax:           20.0,
			biasMax:          -8.0,
			nowcastAdj:       4.0,
			wantCorrectedMax: 30.0,
		},
		{
			name:             "clamp large negative correction to -10",
			rawMax:           30.0,
			biasMax:          8.0,
			nowcastAdj:       -4.0,
			wantCorrectedMax: 20.0,
		},
		{
			name:             "exactly at +10 limit - no clamp",
			rawMax:           20.0,
			biasMax:          -6.0,
			nowcastAdj:       4.0,
			wantCorrectedMax: 30.0,
		},
		{
			name:             "exactly at -10 limit - no clamp",
			rawMax:           30.0,
			biasMax:          6.0,
			nowcastAdj:       -4.0,
			wantCorrectedMax: 20.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adjustment := capCorrection(tt.nowcastAdj, maxAdjustment)
			correctedMax := tt.rawMax - tt.biasMax + adjustment

			totalCorrection := correctedMax - tt.rawMax
			if totalCorrection > maxTotalCorrection {
				correctedMax = tt.rawMax + maxTotalCorrection
			} else if totalCorrection < -maxTotalCorrection {
				correctedMax = tt.rawMax - maxTotalCorrection
			}

			if correctedMax != tt.wantCorrectedMax {
				t.Errorf("correctedMax = %v, want %v", correctedMax, tt.wantCorrectedMax)
			}
		})
	}
}

func TestNowcastAdjustmentCapped(t *testing.T) {
	tests := []struct {
		name       string
		adjustment float64
		want       float64
	}{
		{"within limit", 3.0, 3.0},
		{"exceeds positive limit", 6.0, maxAdjustment},
		{"exceeds negative limit", -6.0, -maxAdjustment},
		{"exactly at limit", maxAdjustment, maxAdjustment},
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
