package emergency

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lox/wandiweather/internal/htmlutil"
	"github.com/lox/wandiweather/internal/httputil"
)

const (
	eventsURL = "https://emergency.vic.gov.au/public/events-geojson.json"

	// Default radius in km to search for alerts around Wandiligong
	// 15km covers the immediate valley (Wandiligong, Bright, Harrietville)
	DefaultRadiusKM = 15.0
)

// Severity levels for sorting alerts
const (
	SeverityEmergency = iota
	SeverityWatchAct
	SeverityAdvice
	SeverityCommunity
	SeverityUnknown
)

// Alert represents a processed emergency alert relevant to our location.
type Alert struct {
	ID          string
	Category    string // e.g., "Fire", "Met", "Flood"
	SubCategory string // e.g., "Bushfire", "Extreme Heat"
	Name        string // e.g., "Emergency Warning", "Watch & Act"
	Status      string // e.g., "Going", "Under Control"
	Location    string
	Distance    float64 // km from Wandiligong
	Severity    int
	Created     time.Time
	Updated     time.Time
	Headline    string
	Body        string
	Text        string
	URL         string
	Lat         float64
	Lon         float64
}

// Client fetches and filters VicEmergency alerts.
type Client struct {
	httpClient *http.Client
	centerLat  float64
	centerLon  float64
	radiusKM   float64

	mu          sync.RWMutex
	cachedAlerts []Alert
	lastFetch   time.Time
}

// NewClient creates a new VicEmergency client centered on a location.
func NewClient(lat, lon, radiusKM float64) *Client {
	return &Client{
		httpClient: httputil.NewClient(),
		centerLat:  lat,
		centerLon:  lon,
		radiusKM:   radiusKM,
	}
}

// Alerts returns cached alerts, fetching fresh data if stale.
func (c *Client) Alerts(ctx context.Context) ([]Alert, error) {
	c.mu.RLock()
	if time.Since(c.lastFetch) < 2*time.Minute && c.cachedAlerts != nil {
		alerts := c.cachedAlerts
		c.mu.RUnlock()
		return alerts, nil
	}
	c.mu.RUnlock()

	return c.Fetch(ctx)
}

// Fetch retrieves fresh alerts from VicEmergency.
func (c *Client) Fetch(ctx context.Context) ([]Alert, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", eventsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "WandiWeather/1.0 (weather site for Wandiligong)")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch events: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var geoJSON GeoJSON
	if err := json.NewDecoder(resp.Body).Decode(&geoJSON); err != nil {
		return nil, fmt.Errorf("decode geojson: %w", err)
	}

	alerts := c.filterAlerts(geoJSON.Features)

	c.mu.Lock()
	c.cachedAlerts = alerts
	c.lastFetch = time.Now()
	c.mu.Unlock()

	return alerts, nil
}

// CachedAlerts returns the last fetched alerts without making a network request.
func (c *Client) CachedAlerts() []Alert {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cachedAlerts
}

// filterAlerts processes GeoJSON features and returns relevant alerts.
func (c *Client) filterAlerts(features []Feature) []Alert {
	var alerts []Alert
	seen := make(map[string]bool)

	for _, f := range features {
		// Skip non-warning/incident features
		feedType := f.Properties.FeedType
		if feedType != "warning" && feedType != "incident" {
			continue
		}

		// Skip if already seen (dedupe by ID)
		id := string(f.Properties.ID)
		if id == "" {
			id = string(f.Properties.SourceID)
		}
		if seen[id] {
			continue
		}

		// Get coordinates
		lat, lon := extractCoordinates(f.Geometry)
		if lat == 0 && lon == 0 {
			continue
		}

		// Calculate distance
		dist := haversine(c.centerLat, c.centerLon, lat, lon)

		// Strict radius filter - only show truly local alerts
		if dist > c.radiusKM {
			continue
		}

		seen[id] = true

		alert := Alert{
			ID:          id,
			Category:    f.Properties.Category1,
			SubCategory: f.Properties.Category2,
			Name:        f.Properties.Name,
			Status:      f.Properties.Status,
			Location:    f.Properties.Location,
			Distance:    dist,
			Severity:    parseSeverity(f.Properties.Name, f.Properties.SourceTitle),
			Headline:    f.Properties.WebHeadline,
			Body:        htmlutil.ToText(f.Properties.WebBody),
			Text:        f.Properties.Text,
			URL:         buildURL(id),
			Lat:         lat,
			Lon:         lon,
		}

		if t, err := time.Parse(time.RFC3339, f.Properties.Created); err == nil {
			alert.Created = t
		}
		if t, err := time.Parse(time.RFC3339, f.Properties.Updated); err == nil {
			alert.Updated = t
		}

		alerts = append(alerts, alert)
	}

	// Sort by severity (most urgent first), then distance
	sort.Slice(alerts, func(i, j int) bool {
		if alerts[i].Severity != alerts[j].Severity {
			return alerts[i].Severity < alerts[j].Severity
		}
		return alerts[i].Distance < alerts[j].Distance
	})

	return alerts
}

// parseSeverity maps alert names to severity levels.
func parseSeverity(name, sourceTitle string) int {
	check := strings.ToLower(name + " " + sourceTitle)
	switch {
	case strings.Contains(check, "emergency"):
		return SeverityEmergency
	case strings.Contains(check, "watch") && strings.Contains(check, "act"):
		return SeverityWatchAct
	case strings.Contains(check, "warning"):
		return SeverityWatchAct
	case strings.Contains(check, "advice"):
		return SeverityAdvice
	case strings.Contains(check, "community"):
		return SeverityCommunity
	default:
		return SeverityUnknown
	}
}

// buildURL constructs the VicEmergency detail URL for an alert.
func buildURL(id string) string {
	if id == "" {
		return "https://emergency.vic.gov.au"
	}
	return fmt.Sprintf("https://emergency.vic.gov.au/respond/#!/warning/%s/moreinfo", id)
}

// haversine calculates the distance in km between two coordinates.
func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371 // Earth radius in km

	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLon/2)*math.Sin(dLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c
}

// extractCoordinates gets the first point coordinates from geometry.
func extractCoordinates(g *Geometry) (lat, lon float64) {
	if g == nil {
		return 0, 0
	}

	switch g.Type {
	case "Point":
		if len(g.Coordinates) >= 2 {
			// GeoJSON is [lon, lat]
			return g.Coordinates[1], g.Coordinates[0]
		}
	case "GeometryCollection":
		for _, geom := range g.Geometries {
			if lat, lon := extractCoordinates(&geom); lat != 0 || lon != 0 {
				return lat, lon
			}
		}
	}
	return 0, 0
}

// SeverityName returns a human-readable severity label.
func (a Alert) SeverityName() string {
	switch a.Severity {
	case SeverityEmergency:
		return "Emergency Warning"
	case SeverityWatchAct:
		return "Watch and Act"
	case SeverityAdvice:
		return "Advice"
	case SeverityCommunity:
		return "Community Information"
	default:
		return "Alert"
	}
}

// SeverityClass returns a CSS class for styling.
func (a Alert) SeverityClass() string {
	switch a.Severity {
	case SeverityEmergency:
		return "alert-emergency"
	case SeverityWatchAct:
		return "alert-watch"
	case SeverityAdvice:
		return "alert-advice"
	default:
		return "alert-info"
	}
}

// IsUrgent returns true for Emergency or Watch & Act alerts.
func (a Alert) IsUrgent() bool {
	return a.Severity <= SeverityWatchAct
}
