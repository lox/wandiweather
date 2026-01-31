package firedanger

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/lox/wandiweather/internal/htmlutil"
	"github.com/lox/wandiweather/internal/httputil"
)

const (
	// North East district RSS feed URL (covers Alpine region including Wandiligong)
	NorthEastFeedURL = "https://www.cfa.vic.gov.au/cfa/rssfeed/northeast-firedistrict_rss.xml"
)

// Rating represents fire danger rating levels
type Rating string

const (
	RatingNone        Rating = "NO RATING"
	RatingModerate    Rating = "MODERATE"
	RatingHigh        Rating = "HIGH"
	RatingExtreme     Rating = "EXTREME"
	RatingCatastrophic Rating = "CATASTROPHIC"
)

// RatingSeverity returns a numeric severity for sorting (higher = more dangerous)
func (r Rating) Severity() int {
	switch r {
	case RatingCatastrophic:
		return 4
	case RatingExtreme:
		return 3
	case RatingHigh:
		return 2
	case RatingModerate:
		return 1
	default:
		return 0
	}
}

// CSSClass returns the CSS class for styling
func (r Rating) CSSClass() string {
	switch r {
	case RatingCatastrophic:
		return "fdr-catastrophic"
	case RatingExtreme:
		return "fdr-extreme"
	case RatingHigh:
		return "fdr-high"
	case RatingModerate:
		return "fdr-moderate"
	default:
		return "fdr-none"
	}
}

// DayForecast represents fire danger info for a single day
type DayForecast struct {
	Date         time.Time
	Rating       Rating
	TotalFireBan bool
	District     string
}

// Client fetches fire danger ratings from CFA RSS feed
type Client struct {
	httpClient *http.Client
	feedURL    string
	district   string
}

// NewClient creates a new CFA fire danger client
func NewClient(feedURL, district string) *Client {
	return &Client{
		httpClient: httputil.NewClient(),
		feedURL:    feedURL,
		district:   district,
	}
}

// NewNorthEastClient creates a client for the North East fire district
func NewNorthEastClient() *Client {
	return NewClient(NorthEastFeedURL, "North East")
}

// Fetch retrieves the current fire danger forecast
func (c *Client) Fetch(ctx context.Context) ([]DayForecast, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.feedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "WandiWeather/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch feed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var rss rssDocument
	if err := xml.NewDecoder(resp.Body).Decode(&rss); err != nil {
		return nil, fmt.Errorf("decode rss: %w", err)
	}

	return c.parseItems(rss.Channel.Items), nil
}

// RSS XML structures
type rssDocument struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title   string    `xml:"title"`
	PubDate string    `xml:"pubDate"`
	Items   []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Description string `xml:"description"`
}

// parseItems extracts fire danger forecasts from RSS items
func (c *Client) parseItems(items []rssItem) []DayForecast {
	var forecasts []DayForecast

	// Regex patterns
	ratingPattern := regexp.MustCompile(`North East:\s*(NO RATING|MODERATE|HIGH|EXTREME|CATASTROPHIC)`)
	tfbPattern := regexp.MustCompile(`declared a day of Total Fire Ban`)

	for _, item := range items {
		// Skip non-date items (like "Fire restrictions by municipality")
		date, ok := parseItemDate(item.Title)
		if !ok {
			continue
		}

		forecast := DayForecast{
			Date:     date,
			District: c.district,
		}

		// Extract rating from description
		desc := htmlutil.ToText(item.Description)
		if match := ratingPattern.FindStringSubmatch(desc); len(match) > 1 {
			forecast.Rating = Rating(match[1])
		}

		// Check for Total Fire Ban
		forecast.TotalFireBan = tfbPattern.MatchString(desc)

		forecasts = append(forecasts, forecast)
	}

	return forecasts
}

// parseItemDate tries to parse a date from item title like "Sunday, 25 January 2026"
func parseItemDate(title string) (time.Time, bool) {
	// Try common formats
	formats := []string{
		"Monday, 2 January 2006",
		"Monday, 02 January 2006",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, title); err == nil {
			return t, true
		}
	}

	return time.Time{}, false
}


