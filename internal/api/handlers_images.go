package api

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/lox/wandiweather/internal/forecast"
	"github.com/lox/wandiweather/internal/imagegen"
)

// handleWeatherImage serves a weather-appropriate header image.
// It checks cache first, generates on-demand if needed, and returns a placeholder while generating.
// Supports ?weather=condition_time override for testing (e.g., ?weather=storm_night).
func (s *Server) handleWeatherImage(w http.ResponseWriter, r *http.Request) {
	// Get current weather condition and time of day
	loc := s.loc
	now := time.Now().In(loc)
	tod := forecast.GetTimeOfDay(now)
	baseCondition := s.getCurrentCondition()
	hasOverride := false

	// Check for override query param
	if override := r.URL.Query().Get("weather"); override != "" {
		hasOverride = true
		if overrideCond, overrideTod, ok := parseWeatherOverride(override); ok {
			baseCondition = overrideCond
			tod = overrideTod
		} else {
			baseCondition = overrideCond
		}
	}

	condition := forecast.ConditionWithTime(baseCondition, tod)

	// Try cache first
	if data, ok := s.imageCache.Get(condition); ok {
		s.serveBannerImage(w, data)
		return
	}

	// Try any cached image as fallback (but not when testing with override)
	if !hasOverride {
		if data, ok := s.imageCache.GetAny(); ok {
			// Trigger async generation for the correct condition
			go s.generateAndCache(baseCondition, tod, now)
			s.serveBannerImage(w, data)
			return
		}
	}

	// No cache - if we can generate, do it synchronously
	if s.imageGen != nil {
		s.genMu.Lock()
		defer s.genMu.Unlock()

		// Double-check cache after acquiring lock
		if data, ok := s.imageCache.Get(condition); ok {
			s.serveBannerImage(w, data)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()

		log.Printf("Generating first banner image for condition: %s", condition)
		data, err := s.imageGen.Generate(ctx, baseCondition, tod, now)
		if err != nil {
			log.Printf("Banner generation failed: %v", err)
			http.Error(w, "Image generation failed", http.StatusServiceUnavailable)
			return
		}

		if err := s.imageCache.Set(condition, data); err != nil {
			log.Printf("Failed to cache banner: %v", err)
		}

		s.serveBannerImage(w, data)
		return
	}

	// No generator and no cache - return 503
	log.Printf("weather-image: no generator and no cached images available")
	http.Error(w, "Weather image service unavailable", http.StatusServiceUnavailable)
}

func (s *Server) serveBannerImage(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(data)
}

// handleOGImage serves a dynamic Open Graph image for social media sharing.
// It composites the current weather image with temperature and condition text.
func (s *Server) handleOGImage(w http.ResponseWriter, r *http.Request) {
	// Check cache first
	if data, ok := s.ogImageCache.Get(); ok {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Write(data)
		return
	}

	// Get current weather data
	currentData, err := s.getCurrentData()
	if err != nil {
		log.Printf("og-image: failed to get current data: %v", err)
		http.Error(w, "Failed to get weather data", http.StatusInternalServerError)
		return
	}

	// Build OG image data
	ogData := imagegen.OGImageData{}
	if currentData.Primary != nil && currentData.Primary.Temp.Valid {
		ogData.Temperature = currentData.Primary.Temp.Float64
	}

	// Get condition description from today's forecast narrative or derive from condition
	if currentData.TodayForecast != nil && currentData.TodayForecast.Narrative != "" {
		// Extract first sentence or use a simplified version
		narrative := currentData.TodayForecast.Narrative
		if len(narrative) > 30 {
			// Find first period or truncate
			for i, c := range narrative {
				if c == '.' && i > 10 {
					narrative = narrative[:i]
					break
				}
			}
		}
		ogData.Condition = narrative
	} else {
		// Fall back to condition name
		condition := s.getCurrentCondition()
		ogData.Condition = conditionToReadable(condition)
	}

	// Get current weather image
	loc := s.loc
	now := time.Now().In(loc)
	tod := forecast.GetTimeOfDay(now)
	baseCondition := s.getCurrentCondition()
	condition := forecast.ConditionWithTime(baseCondition, tod)

	var ogImage []byte

	if weatherImage, ok := s.imageCache.Get(condition); ok {
		ogImage, err = imagegen.GenerateOGImage(weatherImage, ogData)
	} else if weatherImage, ok := s.imageCache.GetAny(); ok {
		ogImage, err = imagegen.GenerateOGImage(weatherImage, ogData)
	} else {
		// No weather image available - generate fallback
		ogImage, err = imagegen.GenerateFallbackOGImage(ogData)
	}

	if err != nil {
		log.Printf("og-image: failed to generate: %v", err)
		http.Error(w, "Failed to generate OG image", http.StatusInternalServerError)
		return
	}

	// Cache the result
	s.ogImageCache.Set(ogImage)

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Write(ogImage)
}

// conditionToReadable converts a weather condition to a human-readable string.
func conditionToReadable(condition forecast.WeatherCondition) string {
	switch condition {
	case forecast.ConditionClearWarm:
		return "Clear & Warm"
	case forecast.ConditionClearCool:
		return "Clear & Cool"
	case forecast.ConditionPartlyCloudy:
		return "Partly Cloudy"
	case forecast.ConditionMostlyCloudy:
		return "Mostly Cloudy"
	case forecast.ConditionLightRain:
		return "Light Rain"
	case forecast.ConditionHeavyRain:
		return "Heavy Rain"
	case forecast.ConditionStorm:
		return "Stormy"
	case forecast.ConditionFog:
		return "Foggy"
	case forecast.ConditionHot:
		return "Hot"
	case forecast.ConditionFrost:
		return "Frosty"
	default:
		return ""
	}
}

func (s *Server) generateAndCache(baseCondition forecast.WeatherCondition, tod forecast.TimeOfDay, t time.Time) {
	if s.imageGen == nil {
		return
	}

	condition := forecast.ConditionWithTime(baseCondition, tod)

	s.genMu.Lock()
	defer s.genMu.Unlock()

	// Check if already cached
	if _, ok := s.imageCache.Get(condition); ok {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	log.Printf("Background generating banner for condition: %s", condition)
	data, err := s.imageGen.Generate(ctx, baseCondition, tod, t)
	if err != nil {
		log.Printf("Background banner generation failed: %v", err)
		return
	}

	if err := s.imageCache.Set(condition, data); err != nil {
		log.Printf("Failed to cache banner: %v", err)
	}
	log.Printf("Cached banner for condition: %s", condition)
}

// parseWeatherOverride parses a "condition_time" string (e.g., "storm_night")
// into separate condition and time-of-day values. Returns ok=false if not valid.
func parseWeatherOverride(override string) (condition forecast.WeatherCondition, tod forecast.TimeOfDay, ok bool) {
	if override == "" {
		return "", "", false
	}

	// Try to split on known time suffixes
	times := []forecast.TimeOfDay{forecast.TimeDawn, forecast.TimeDay, forecast.TimeDusk, forecast.TimeNight}
	for _, t := range times {
		suffix := "_" + string(t)
		if strings.HasSuffix(override, suffix) {
			cond := strings.TrimSuffix(override, suffix)
			return forecast.WeatherCondition(cond), t, true
		}
	}

	// No time suffix - treat whole string as condition
	return forecast.WeatherCondition(override), "", false
}

// getCurrentCondition extracts the weather condition from today's forecast.
func (s *Server) getCurrentCondition() forecast.WeatherCondition {
	loc := s.loc
	today := time.Now().In(loc)
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)

	forecasts, err := s.store.GetLatestForecasts()
	if err != nil {
		return forecast.ConditionClearCool // Default fallback
	}

	// Check WU forecasts first
	for _, fc := range forecasts["wu"] {
		if fc.ValidDate.Format("2006-01-02") == todayDate.Format("2006-01-02") {
			narrative := ""
			if fc.Narrative.Valid {
				narrative = fc.Narrative.String
			}
			tempMax := 20.0
			tempMin := 10.0
			if fc.TempMax.Valid {
				tempMax = fc.TempMax.Float64
			}
			if fc.TempMin.Valid {
				tempMin = fc.TempMin.Float64
			}
			return forecast.ExtractCondition(narrative, tempMax, tempMin)
		}
	}

	return forecast.ConditionClearCool
}
