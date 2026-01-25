package store

import (
	"time"

	"github.com/lox/wandiweather/internal/firedanger"
)

// UpsertFireDanger inserts or updates a fire danger rating.
func (s *Store) UpsertFireDanger(f firedanger.DayForecast, fetchedAt time.Time) error {
	_, err := s.db.Exec(`
		INSERT INTO fire_danger_ratings (date, district, rating, total_fire_ban, fetched_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(date, district) DO UPDATE SET
			rating = excluded.rating,
			total_fire_ban = excluded.total_fire_ban,
			fetched_at = excluded.fetched_at
	`, f.Date.Format("2006-01-02"), f.District, string(f.Rating), f.TotalFireBan, fetchedAt)
	return err
}

// GetFireDanger returns the fire danger rating for a specific date and district.
func (s *Store) GetFireDanger(date time.Time, district string) (*firedanger.DayForecast, error) {
	row := s.db.QueryRow(`
		SELECT date, district, rating, total_fire_ban
		FROM fire_danger_ratings
		WHERE date = ? AND district = ?
	`, date.Format("2006-01-02"), district)

	var f firedanger.DayForecast
	var dateStr string
	var rating string
	if err := row.Scan(&dateStr, &f.District, &rating, &f.TotalFireBan); err != nil {
		return nil, err
	}

	f.Rating = firedanger.Rating(rating)
	if t, err := time.Parse("2006-01-02", dateStr); err == nil {
		f.Date = t
	}

	return &f, nil
}

// GetTodayFireDanger returns today's fire danger rating for the default district.
func (s *Store) GetTodayFireDanger(loc *time.Location) (*firedanger.DayForecast, error) {
	today := time.Now().In(loc)
	return s.GetFireDanger(today, "North East")
}

// GetFireDangerForecast returns the multi-day fire danger forecast.
func (s *Store) GetFireDangerForecast(district string, days int) ([]firedanger.DayForecast, error) {
	rows, err := s.db.Query(`
		SELECT date, district, rating, total_fire_ban
		FROM fire_danger_ratings
		WHERE district = ? AND date >= date('now')
		ORDER BY date ASC
		LIMIT ?
	`, district, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var forecasts []firedanger.DayForecast
	for rows.Next() {
		var f firedanger.DayForecast
		var dateStr string
		var rating string
		if err := rows.Scan(&dateStr, &f.District, &rating, &f.TotalFireBan); err != nil {
			return nil, err
		}
		f.Rating = firedanger.Rating(rating)
		if t, err := time.Parse("2006-01-02", dateStr); err == nil {
			f.Date = t
		}
		forecasts = append(forecasts, f)
	}

	return forecasts, rows.Err()
}
