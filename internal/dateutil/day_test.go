package dateutil

import (
	"testing"
	"time"
)

func TestDateKeyUTC(t *testing.T) {
	t.Parallel()

	when := time.Date(2026, time.January, 4, 23, 15, 0, 0, time.FixedZone("UTC+11", 11*60*60))
	if got, want := DateKeyUTC(when), "2026-01-04"; got != want {
		t.Fatalf("DateKeyUTC() = %q, want %q", got, want)
	}
}

func TestParseDateKey(t *testing.T) {
	t.Parallel()

	parsed, err := ParseDateKey("2026-02-19")
	if err != nil {
		t.Fatalf("ParseDateKey: %v", err)
	}
	if !parsed.Equal(time.Date(2026, time.February, 19, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("ParseDateKey() = %s, want UTC midnight", parsed)
	}
}

func TestLocalDayStart(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("UTC+11", 11*60*60)
	when := time.Date(2026, time.March, 1, 1, 45, 0, 0, time.UTC)

	got := LocalDayStart(when, loc)
	want := time.Date(2026, time.March, 1, 0, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Fatalf("LocalDayStart() = %s, want %s", got, want)
	}
}
