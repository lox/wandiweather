package store

import (
	"time"

	"github.com/lox/wandiweather/internal/emergency"
)

// UpsertAlert inserts or updates an emergency alert.
// Updates last_seen_at on conflict to track when alerts are still active.
func (s *Store) UpsertAlert(alert emergency.Alert, now time.Time) error {
	_, err := s.db.Exec(`
		INSERT INTO emergency_alerts (
			id, category, subcategory, name, status, location, distance_km,
			severity, lat, lon, headline, body, url,
			first_seen_at, last_seen_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status = excluded.status,
			name = excluded.name,
			headline = excluded.headline,
			body = excluded.body,
			last_seen_at = excluded.last_seen_at,
			updated_at = excluded.updated_at
	`,
		alert.ID, alert.Category, alert.SubCategory, alert.Name, alert.Status,
		alert.Location, alert.Distance, alert.Severity, alert.Lat, alert.Lon,
		alert.Headline, alert.Body, alert.URL,
		now, now, alert.Created, alert.Updated,
	)
	return err
}

// GetActiveAlerts returns alerts that were seen within the given duration.
func (s *Store) GetActiveAlerts(maxAge time.Duration) ([]emergency.Alert, error) {
	cutoff := time.Now().Add(-maxAge)

	rows, err := s.db.Query(`
		SELECT id, category, subcategory, name, status, location, distance_km,
		       severity, lat, lon, headline, body, url, created_at, updated_at
		FROM emergency_alerts
		WHERE last_seen_at > ?
		ORDER BY severity ASC, distance_km ASC
	`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []emergency.Alert
	for rows.Next() {
		var a emergency.Alert
		var createdAt, updatedAt *time.Time
		if err := rows.Scan(
			&a.ID, &a.Category, &a.SubCategory, &a.Name, &a.Status,
			&a.Location, &a.Distance, &a.Severity, &a.Lat, &a.Lon,
			&a.Headline, &a.Body, &a.URL, &createdAt, &updatedAt,
		); err != nil {
			return nil, err
		}
		if createdAt != nil {
			a.Created = *createdAt
		}
		if updatedAt != nil {
			a.Updated = *updatedAt
		}
		alerts = append(alerts, a)
	}

	return alerts, rows.Err()
}

// GetUrgentAlerts returns active alerts that are Emergency or Watch & Act level.
func (s *Store) GetUrgentAlerts(maxAge time.Duration) ([]emergency.Alert, error) {
	cutoff := time.Now().Add(-maxAge)

	rows, err := s.db.Query(`
		SELECT id, category, subcategory, name, status, location, distance_km,
		       severity, lat, lon, headline, body, url, created_at, updated_at
		FROM emergency_alerts
		WHERE last_seen_at > ? AND severity <= ?
		ORDER BY severity ASC, distance_km ASC
	`, cutoff, emergency.SeverityWatchAct)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []emergency.Alert
	for rows.Next() {
		var a emergency.Alert
		var createdAt, updatedAt *time.Time
		if err := rows.Scan(
			&a.ID, &a.Category, &a.SubCategory, &a.Name, &a.Status,
			&a.Location, &a.Distance, &a.Severity, &a.Lat, &a.Lon,
			&a.Headline, &a.Body, &a.URL, &createdAt, &updatedAt,
		); err != nil {
			return nil, err
		}
		if createdAt != nil {
			a.Created = *createdAt
		}
		if updatedAt != nil {
			a.Updated = *updatedAt
		}
		alerts = append(alerts, a)
	}

	return alerts, rows.Err()
}
