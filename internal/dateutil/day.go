package dateutil

import "time"

const keyLayout = "2006-01-02"

// DateKeyUTC returns a stable YYYY-MM-DD key in UTC.
func DateKeyUTC(t time.Time) string {
	return t.UTC().Format(keyLayout)
}

// ParseDateKey parses a YYYY-MM-DD date key as a UTC time.
func ParseDateKey(dateKey string) (time.Time, error) {
	return time.Parse(keyLayout, dateKey)
}

// LocalDayStart returns midnight for the day of t in the provided location.
func LocalDayStart(t time.Time, loc *time.Location) time.Time {
	local := t.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
}
