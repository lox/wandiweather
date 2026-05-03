package main

import "testing"

func TestModeRequiresPWSKey(t *testing.T) {
	tests := []struct {
		name                   string
		noPoll                 bool
		once                   bool
		backfill               bool
		daily                  bool
		backfillDaily          bool
		airQualityBackfillDays int
		want                   bool
	}{
		{name: "default server mode requires key", want: true},
		{name: "once requires key", once: true, want: true},
		{name: "history backfill requires key", backfill: true, want: true},
		{name: "history backfill still requires key when combined with air quality backfill", backfill: true, airQualityBackfillDays: 7, want: true},
		{name: "server only does not require key", noPoll: true, want: false},
		{name: "once still requires key when combined with no-poll", noPoll: true, once: true, want: true},
		{name: "daily jobs do not require key", daily: true, want: false},
		{name: "daily jobs outrank once", daily: true, once: true, want: false},
		{name: "daily backfill does not require key", backfillDaily: true, want: false},
		{name: "daily backfill outranks once", backfillDaily: true, once: true, want: false},
		{name: "air quality backfill does not require key", airQualityBackfillDays: 7, want: false},
		{name: "air quality backfill outranks once", airQualityBackfillDays: 7, once: true, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := modeRequiresPWSKey(tt.noPoll, tt.once, tt.backfill, tt.airQualityBackfillDays, tt.daily, tt.backfillDaily)
			if got != tt.want {
				t.Fatalf("modeRequiresPWSKey() = %t, want %t", got, tt.want)
			}
		})
	}
}
