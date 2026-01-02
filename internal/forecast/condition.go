package forecast

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// WeatherCondition represents a categorized weather state for image generation.
type WeatherCondition string

const (
	ConditionClearWarm    WeatherCondition = "clear_warm"
	ConditionClearCool    WeatherCondition = "clear_cool"
	ConditionPartlyCloudy WeatherCondition = "partly_cloudy"
	ConditionMostlyCloudy WeatherCondition = "mostly_cloudy"
	ConditionLightRain    WeatherCondition = "light_rain"
	ConditionHeavyRain    WeatherCondition = "heavy_rain"
	ConditionStorm        WeatherCondition = "storm"
	ConditionFog          WeatherCondition = "fog"
	ConditionHot          WeatherCondition = "hot"
	ConditionFrost        WeatherCondition = "frost"
)

// TimeOfDay represents the lighting period.
type TimeOfDay string

const (
	TimeDay   TimeOfDay = "day"
	TimeDusk  TimeOfDay = "dusk"
	TimeNight TimeOfDay = "night"
	TimeDawn  TimeOfDay = "dawn"
)

// GetTimeOfDay returns the current time-of-day category for the given location.
func GetTimeOfDay(t time.Time) TimeOfDay {
	hour := t.Hour()
	switch {
	case hour >= 5 && hour < 7:
		return TimeDawn
	case hour >= 7 && hour < 17:
		return TimeDay
	case hour >= 17 && hour < 20:
		return TimeDusk
	default:
		return TimeNight
	}
}

// MoonPhase represents the current lunar phase.
type MoonPhase string

const (
	MoonNew           MoonPhase = "new"
	MoonWaxingCrescent MoonPhase = "waxing_crescent"
	MoonFirstQuarter  MoonPhase = "first_quarter"
	MoonWaxingGibbous MoonPhase = "waxing_gibbous"
	MoonFull          MoonPhase = "full"
	MoonWaningGibbous MoonPhase = "waning_gibbous"
	MoonLastQuarter   MoonPhase = "last_quarter"
	MoonWaningCrescent MoonPhase = "waning_crescent"
)

// LunarCycle is approximately 29.53 days.
const LunarCycle = 29.53

// GetMoonPhase calculates the moon phase for a given date.
// Uses a known new moon as reference (January 6, 2000 18:14 UTC).
func GetMoonPhase(t time.Time) MoonPhase {
	// Reference new moon: January 6, 2000 18:14 UTC
	ref := time.Date(2000, 1, 6, 18, 14, 0, 0, time.UTC)
	
	// Days since reference
	days := t.Sub(ref).Hours() / 24
	
	// Position in current cycle (0 to ~29.53)
	pos := days - float64(int(days/LunarCycle))*LunarCycle
	if pos < 0 {
		pos += LunarCycle
	}
	
	// Divide cycle into 8 phases
	phase := int((pos / LunarCycle) * 8)
	
	switch phase {
	case 0:
		return MoonNew
	case 1:
		return MoonWaxingCrescent
	case 2:
		return MoonFirstQuarter
	case 3:
		return MoonWaxingGibbous
	case 4:
		return MoonFull
	case 5:
		return MoonWaningGibbous
	case 6:
		return MoonLastQuarter
	default:
		return MoonWaningCrescent
	}
}

// MoonIllumination returns approximate illumination percentage (0-100).
func MoonIllumination(t time.Time) int {
	ref := time.Date(2000, 1, 6, 18, 14, 0, 0, time.UTC)
	days := t.Sub(ref).Hours() / 24
	pos := days - float64(int(days/LunarCycle))*LunarCycle
	if pos < 0 {
		pos += LunarCycle
	}
	
	// Illumination follows a cosine curve
	// 0 at new moon, 100 at full moon
	angle := (pos / LunarCycle) * 2 * math.Pi
	illumination := (1 - math.Cos(angle)) / 2 * 100
	return int(illumination)
}

// MoonDescription returns a human-readable description and prompt hint for the moon.
func MoonDescription(phase MoonPhase) (name string, prompt string) {
	switch phase {
	case MoonNew:
		return "New Moon", "No visible moon, very dark sky, stars prominent"
	case MoonWaxingCrescent:
		return "Waxing Crescent", "Thin crescent moon visible"
	case MoonFirstQuarter:
		return "First Quarter", "Half moon visible"
	case MoonWaxingGibbous:
		return "Waxing Gibbous", "Nearly full moon, bright moonlight"
	case MoonFull:
		return "Full Moon", "Bright full moon illuminating the landscape"
	case MoonWaningGibbous:
		return "Waning Gibbous", "Nearly full moon, bright moonlight"
	case MoonLastQuarter:
		return "Last Quarter", "Half moon visible"
	case MoonWaningCrescent:
		return "Waning Crescent", "Thin crescent moon visible"
	default:
		return "Moon", "Moon visible in sky"
	}
}

// ExtractCondition determines the weather condition category from forecast data.
// It considers the narrative text and temperature values to categorize the weather.
func ExtractCondition(narrative string, tempMax, tempMin float64) WeatherCondition {
	lower := strings.ToLower(narrative)

	// Temperature extremes take priority
	if tempMax >= 35 {
		return ConditionHot
	}
	if tempMin <= 2 {
		return ConditionFrost
	}

	// Storm conditions (highest priority weather)
	if strings.Contains(lower, "thunder") || strings.Contains(lower, "storm") {
		return ConditionStorm
	}

	// Rain conditions
	if strings.Contains(lower, "heavy rain") {
		return ConditionHeavyRain
	}
	if strings.Contains(lower, "rain") || strings.Contains(lower, "shower") ||
		strings.Contains(lower, "drizzle") {
		return ConditionLightRain
	}

	// Fog/mist
	if strings.Contains(lower, "fog") || strings.Contains(lower, "mist") ||
		strings.Contains(lower, "haze") {
		return ConditionFog
	}

	// Cloud conditions
	if strings.Contains(lower, "mostly cloudy") || strings.Contains(lower, "overcast") {
		return ConditionMostlyCloudy
	}
	if strings.Contains(lower, "partly cloudy") || strings.Contains(lower, "mix of") {
		return ConditionPartlyCloudy
	}

	// Default to clear based on temperature
	if tempMax >= 25 {
		return ConditionClearWarm
	}
	return ConditionClearCool
}

// ConditionWithTime combines a weather condition with time of day for cache keys.
func ConditionWithTime(condition WeatherCondition, tod TimeOfDay) WeatherCondition {
	return WeatherCondition(fmt.Sprintf("%s_%s", condition, tod))
}

// baseStylePrompt defines the consistent visual style for all generated images.
const baseStylePrompt = `Serene watercolor landscape painting of Wandiligong valley in the Australian Alps.
Rolling green hills with eucalyptus trees, distant mountains in soft purple haze.
Style: impressionistic watercolor, soft gradients, muted earth tones, peaceful and minimal.
Wide panoramic composition suitable for a website header banner.
No text, no people, no buildings, no animals.`

// conditionPrompts maps each weather condition to specific visual elements (time-neutral).
var conditionPrompts = map[WeatherCondition]string{
	ConditionClearWarm:    "Warm temperature, clear sky, no clouds, vibrant green grass and trees.",
	ConditionClearCool:    "Cool temperature, clear sky, no clouds, crisp air feeling.",
	ConditionPartlyCloudy: "Scattered clouds drifting across sky, patches of clear sky visible.",
	ConditionMostlyCloudy: "Overcast, heavy cloud cover, soft diffused light, muted colors.",
	ConditionLightRain:    "Light rain falling, wet glistening foliage, grey sky, fresh feeling.",
	ConditionHeavyRain:    "Heavy rain, dark grey clouds, dramatic atmosphere, wet surfaces.",
	ConditionStorm:        "Dramatic stormy sky, dark threatening clouds, wind in trees.",
	ConditionFog:          "Mist floating through valley, ethereal atmosphere, soft edges, mysterious.",
	ConditionHot:          "Very hot, dry golden grass, heat shimmer effect.",
	ConditionFrost:        "Cold, frost on grass, cold blue tones, bare trees, crisp air.",
}

// timePrompts adds lighting context for each time of day.
var timePrompts = map[TimeOfDay]string{
	TimeDawn:  "Early dawn, soft pink and orange glow on horizon, cool blue shadows, quiet stillness before sunrise.",
	TimeDay:   "Midday, bright daylight, full sun high in sky, clear visibility, warm natural lighting.",
	TimeDusk:  "Sunset, golden hour, warm orange and pink sky, sun setting behind mountains, long shadows, peaceful evening.",
	TimeNight: "NIGHTTIME SCENE. Dark night sky, no sunlight. Moon visible. Stars scattered across deep blue-black sky. Landscape lit only by soft silvery moonlight. Dark silhouettes of trees and hills. Nocturnal, peaceful, quiet night atmosphere.",
}

// BuildPrompt creates the full image generation prompt for a weather condition.
func BuildPrompt(condition WeatherCondition) string {
	conditionDesc, ok := conditionPrompts[condition]
	if !ok {
		conditionDesc = conditionPrompts[ConditionClearCool]
	}
	return fmt.Sprintf("%s\n\nWeather: %s", baseStylePrompt, conditionDesc)
}

// BuildPromptWithTime creates the full image generation prompt including time of day.
func BuildPromptWithTime(condition WeatherCondition, tod TimeOfDay) string {
	conditionDesc, ok := conditionPrompts[condition]
	if !ok {
		conditionDesc = conditionPrompts[ConditionClearCool]
	}
	timeDesc := timePrompts[tod]
	
	// Put time of day FIRST and emphasize it strongly
	return fmt.Sprintf("%s\n\n%s\n\nWeather conditions: %s", timeDesc, baseStylePrompt, conditionDesc)
}

// BuildPromptWithTimeAndMoon creates prompt including moon phase for night scenes.
func BuildPromptWithTimeAndMoon(condition WeatherCondition, tod TimeOfDay, moon MoonPhase) string {
	conditionDesc, ok := conditionPrompts[condition]
	if !ok {
		conditionDesc = conditionPrompts[ConditionClearCool]
	}
	
	timeDesc := timePrompts[tod]
	
	// For night, add moon phase info
	if tod == TimeNight {
		_, moonPrompt := MoonDescription(moon)
		timeDesc = fmt.Sprintf("NIGHTTIME SCENE. %s. Dark night sky, no sunlight. Stars scattered across deep blue-black sky. Landscape lit by moonlight. Dark silhouettes of trees and hills. Nocturnal, peaceful atmosphere.", moonPrompt)
	}
	
	return fmt.Sprintf("%s\n\n%s\n\nWeather conditions: %s", timeDesc, baseStylePrompt, conditionDesc)
}
