package imagegen

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/lox/wandiweather/internal/forecast"
)

// Cache provides file-based caching for generated weather images.
type Cache struct {
	dir      string
	maxAge   time.Duration
	disabled bool
}

// NewCache creates a new image cache in the specified directory.
// Images are refreshed after maxAge to provide variety.
// If the directory cannot be created, the cache is disabled.
func NewCache(dir string) *Cache {
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("image cache disabled: could not create directory %q: %v", dir, err)
		return &Cache{disabled: true}
	}
	return &Cache{
		dir:    dir,
		maxAge: 7 * 24 * time.Hour, // Refresh weekly for variety
	}
}

// path returns the cache file path for a condition.
func (c *Cache) path(condition forecast.WeatherCondition) string {
	return filepath.Join(c.dir, "weather_"+string(condition)+".png")
}

// Get retrieves a cached image if it exists and is not stale.
// Returns the image bytes and true if found, or nil and false if not cached or stale.
func (c *Cache) Get(condition forecast.WeatherCondition) ([]byte, bool) {
	if c.disabled {
		return nil, false
	}

	path := c.path(condition)
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}

	// Check if stale
	if time.Since(info.ModTime()) > c.maxAge {
		return nil, false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	return data, true
}

// Set stores an image in the cache using atomic write (temp file + rename).
func (c *Cache) Set(condition forecast.WeatherCondition, data []byte) error {
	if c.disabled {
		return nil
	}

	finalPath := c.path(condition)
	tmpPath := finalPath + ".tmp"

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpPath, finalPath)
}

// GetAny returns any non-stale cached image as a fallback.
// Useful when the desired condition is not yet generated.
func (c *Cache) GetAny() ([]byte, bool) {
	if c.disabled {
		return nil, false
	}

	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return nil, false
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".png" {
			continue
		}

		path := filepath.Join(c.dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Skip stale images
		if time.Since(info.ModTime()) > c.maxAge {
			continue
		}

		data, err := os.ReadFile(path)
		if err == nil {
			return data, true
		}
	}

	return nil, false
}

// List returns all cached conditions.
func (c *Cache) List() []forecast.WeatherCondition {
	if c.disabled {
		return nil
	}

	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return nil
	}

	var conditions []forecast.WeatherCondition
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".png" {
			// Extract condition from filename: weather_<condition>.png
			name := entry.Name()
			if len(name) > 12 { // "weather_" + ".png"
				condition := name[8 : len(name)-4]
				conditions = append(conditions, forecast.WeatherCondition(condition))
			}
		}
	}

	return conditions
}
