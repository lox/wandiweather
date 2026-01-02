# Weather Header Image Generation Plan

> **Created**: January 2, 2026  
> **Status**: ✅ Implemented

## Overview

Generate stylistically consistent header images for the WandiWeather UI using OpenAI's image generation API. Images are dynamically created based on current weather conditions and cached for efficiency.

**Goal**: Add visual appeal to the minimal UI with AI-generated watercolor-style landscapes that reflect current/forecast weather conditions.

**API**: OpenAI Image API with `gpt-image-1` model (using `github.com/openai/openai-go/v3`)

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                   Image Generation Pipeline                      │
├─────────────────────────────────────────────────────────────────┤
│                                                                   │
│  Forecast Ingested ──▶ Extract Condition ──▶ Generate Hash      │
│                              │                     │              │
│                              ▼                     ▼              │
│                        Build Prompt          Check Cache          │
│                              │                     │              │
│                              ▼                     ▼              │
│                     ┌─── Cache Miss? ◀────── Hit: Serve ───┐    │
│                     │                                       │    │
│                     ▼                                       │    │
│              Call OpenAI API                                │    │
│                     │                                       │    │
│                     ▼                                       │    │
│              Store in Cache ─────────────────────────────────    │
│                                                                   │
└─────────────────────────────────────────────────────────────────┘
```

---

## Weather Condition Categories

Extract condition from forecast narrative/iconCode into ~10 categories:

| Category | Triggers | Visual Elements |
|----------|----------|-----------------|
| `clear_warm` | sunny, clear, temp ≥ 25°C | Bright sun, blue sky, warm golden light |
| `clear_cool` | sunny, clear, temp < 25°C | Crisp morning light, soft shadows |
| `partly_cloudy` | partly cloudy, mix of sun | Scattered white clouds, dappled light |
| `mostly_cloudy` | mostly cloudy, overcast | Grey sky, muted tones, diffused light |
| `light_rain` | showers, drizzle, light rain | Gentle rain, wet foliage, soft grey |
| `heavy_rain` | rain, heavy rain | Dramatic rain, dark clouds, wet surfaces |
| `storm` | thunderstorm, severe, thunder | Dark dramatic sky, lightning hints |
| `fog` | fog, mist, haze | Ethereal mist, low visibility, soft edges |
| `hot` | temp ≥ 35°C (any condition) | Heat shimmer, harsh light, dry grass |
| `frost` | frost, temp ≤ 2°C | Frost on grass, cold blue tones |

### Condition Extraction Logic

```go
// internal/forecast/condition.go

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

func ExtractCondition(narrative string, tempMax, tempMin float64) WeatherCondition {
    lower := strings.ToLower(narrative)
    
    // Temperature overrides
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
    if strings.Contains(lower, "fog") || strings.Contains(lower, "mist") {
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
```

---

## Style Prompt Template

Consistent style across all generated images:

```go
const baseStylePrompt = `Serene watercolor landscape painting of Wandiligong valley in the Australian Alps.
Rolling green hills with eucalyptus trees, distant mountains in soft purple haze.
Style: impressionistic watercolor, soft gradients, muted earth tones, peaceful and minimal.
Wide panoramic composition suitable for a website header banner.
No text, no people, no buildings, no animals.`

var conditionPrompts = map[WeatherCondition]string{
    ConditionClearWarm:    "Warm sunny day with bright golden light, clear blue sky, vibrant greens.",
    ConditionClearCool:    "Cool crisp morning with soft dawn light, pale blue sky, gentle shadows.",
    ConditionPartlyCloudy: "Scattered white clouds drifting across blue sky, dappled sunlight on hills.",
    ConditionMostlyCloudy: "Overcast grey sky, soft diffused light, muted colors, calm atmosphere.",
    ConditionLightRain:    "Gentle rain falling, wet glistening foliage, soft grey sky, fresh feeling.",
    ConditionHeavyRain:    "Heavy rain, dark grey clouds, dramatic atmosphere, wet surfaces reflecting.",
    ConditionStorm:        "Dramatic stormy sky, dark threatening clouds, moody lighting, wind in trees.",
    ConditionFog:          "Morning mist floating through valley, ethereal atmosphere, soft edges, mysterious.",
    ConditionHot:          "Intense summer heat, harsh bright light, dry golden grass, heat shimmer.",
    ConditionFrost:        "Cold winter morning, frost on grass, cold blue tones, bare trees, crisp air.",
}

func BuildPrompt(condition WeatherCondition) string {
    return fmt.Sprintf("%s\n\nWeather: %s", baseStylePrompt, conditionPrompts[condition])
}
```

---

## Image Generation Service

### OpenAI Client

```go
// internal/imagegen/generator.go

package imagegen

import (
    "context"
    "encoding/base64"
    "fmt"
    "os"
    
    "github.com/lox/wandiweather/internal/forecast"
)

type Generator struct {
    apiKey string
    model  string
}

func NewGenerator() *Generator {
    return &Generator{
        apiKey: os.Getenv("OPENAI_API_KEY"),
        model:  "gpt-image-1-mini", // Cost-effective option
    }
}

type ImageRequest struct {
    Model   string `json:"model"`
    Prompt  string `json:"prompt"`
    N       int    `json:"n"`
    Size    string `json:"size"`
    Quality string `json:"quality"`
}

type ImageResponse struct {
    Data []struct {
        B64JSON string `json:"b64_json"`
    } `json:"data"`
}

func (g *Generator) Generate(ctx context.Context, condition forecast.WeatherCondition) ([]byte, error) {
    if g.apiKey == "" {
        return nil, fmt.Errorf("OPENAI_API_KEY not set")
    }
    
    prompt := forecast.BuildPrompt(condition)
    
    req := ImageRequest{
        Model:   g.model,
        Prompt:  prompt,
        N:       1,
        Size:    "1536x1024", // Wide for header
        Quality: "low",       // Cost-effective, ~$0.016/image
    }
    
    // HTTP request to OpenAI API
    // POST https://api.openai.com/v1/images/generations
    // Returns base64-encoded image
    
    // ... implementation details ...
    
    return imageBytes, nil
}
```

---

## Caching Strategy

### File-based Cache

Store generated images as files for simplicity and persistence across restarts:

```
data/
  images/
    weather_clear_warm.png
    weather_clear_cool.png
    weather_storm.png
    ...
```

```go
// internal/imagegen/cache.go

package imagegen

import (
    "fmt"
    "os"
    "path/filepath"
    "time"
    
    "github.com/lox/wandiweather/internal/forecast"
)

type Cache struct {
    dir    string
    maxAge time.Duration
}

func NewCache(dir string) *Cache {
    os.MkdirAll(dir, 0755)
    return &Cache{
        dir:    dir,
        maxAge: 7 * 24 * time.Hour, // Refresh weekly for variety
    }
}

func (c *Cache) path(condition forecast.WeatherCondition) string {
    return filepath.Join(c.dir, fmt.Sprintf("weather_%s.png", condition))
}

func (c *Cache) Get(condition forecast.WeatherCondition) ([]byte, bool) {
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

func (c *Cache) Set(condition forecast.WeatherCondition, data []byte) error {
    return os.WriteFile(c.path(condition), data, 0644)
}
```

---

## Integration Points

### 1. Forecast Ingestion Hook

Trigger image generation when daily forecast is ingested:

```go
// internal/ingest/forecast.go (modification)

func (i *Ingester) IngestForecasts(ctx context.Context) error {
    // ... existing forecast ingestion ...
    
    // After storing forecast, trigger image generation for today
    if todayForecast != nil {
        go i.ensureWeatherImage(ctx, todayForecast)
    }
    
    return nil
}

func (i *Ingester) ensureWeatherImage(ctx context.Context, fc *models.Forecast) {
    condition := forecast.ExtractCondition(
        fc.Narrative.String,
        fc.TempMax.Float64,
        fc.TempMin.Float64,
    )
    
    // Check cache first
    if _, ok := i.imageCache.Get(condition); ok {
        return // Already cached
    }
    
    // Generate new image
    data, err := i.imageGen.Generate(ctx, condition)
    if err != nil {
        log.Printf("image generation failed: %v", err)
        return
    }
    
    if err := i.imageCache.Set(condition, data); err != nil {
        log.Printf("image cache write failed: %v", err)
    }
}
```

### 2. HTTP Handler

Serve the current weather image:

```go
// internal/api/server.go (addition)

func (s *Server) handleWeatherImage(w http.ResponseWriter, r *http.Request) {
    // Get today's condition
    condition := s.getCurrentCondition()
    
    // Try cache
    data, ok := s.imageCache.Get(condition)
    if !ok {
        // Fallback to default or any cached image
        data = s.getDefaultImage()
    }
    
    w.Header().Set("Content-Type", "image/png")
    w.Header().Set("Cache-Control", "public, max-age=3600")
    w.Write(data)
}
```

### 3. Template Update

Add header image to the UI:

```html
<!-- internal/api/templates/index.html -->
<body>
    <div class="container">
        <div class="weather-header">
            <img src="/weather-image" alt="Current weather" class="header-image">
        </div>
        <div id="current" hx-get="/partials/current" ...>
```

```css
.weather-header {
    margin: -1rem -1rem 1rem -1rem;
    border-radius: 12px;
    overflow: hidden;
}
.header-image {
    width: 100%;
    height: 200px;
    object-fit: cover;
}
```

---

## Cost Analysis

| Model | Quality | Size | Cost/Image |
|-------|---------|------|------------|
| gpt-image-1-mini | low | 1024x1024 | ~$0.011 |
| gpt-image-1-mini | low | 1536x1024 | ~$0.016 |
| gpt-image-1-mini | medium | 1536x1024 | ~$0.063 |
| gpt-image-1 | low | 1536x1024 | ~$0.016 |

**Estimated monthly cost:**
- 10 condition categories × 1 regeneration/week = ~40 images/month
- At $0.016/image = **~$0.64/month**

---

## Environment Variables

```bash
OPENAI_API_KEY=sk-...  # Required for image generation
```

Add to fly.toml secrets for production.

---

## Implementation Checklist

### Phase 1: Core Infrastructure ✅
- [x] Create `internal/imagegen/` package
- [x] Implement condition extraction in `internal/forecast/condition.go`
- [x] Implement OpenAI API client (using `github.com/openai/openai-go/v3`)
- [x] Implement file-based cache

### Phase 2: Integration ✅
- [x] Add image generation hook to forecast ingestion (scheduler calls `ensureWeatherImage`)
- [x] Add hourly check for time-of-day transitions (`checkWeatherImage`)
- [x] Add `/weather-image` HTTP endpoint
- [x] Update templates with header image (background on hero section)

### Phase 3: Polish ✅
- [x] Add fallback/default image (`GetAny()` returns any cached image)
- [x] Add image preloading for smooth UX (async JS loading after HTMX swap)
- [x] Time-of-day variants (dawn/day/dusk/night) with distinct prompts
- [x] Moon phase calculation and integration into night prompts
- [ ] Add manual regeneration endpoint for testing

---

## Implementation Notes

### Time-of-Day Handling
- **Dawn**: 5:00-7:00 (soft pink/orange glow)
- **Day**: 7:00-17:00 (bright daylight)  
- **Dusk**: 17:00-20:00 (golden hour, sunset)
- **Night**: 20:00-5:00 (dark sky, moonlight)

Cache keys include time of day: `weather_clear_warm_night.png`

Hourly scheduler check ensures images are pre-generated before transitions.

### Moon Phase Tracking
Calculates phase from reference new moon (Jan 6, 2000). 8 phases with illumination percentage. Moon description added to night scene prompts.

### Prompt Engineering
- Time of day prompt comes FIRST and is emphatic (especially for night)
- Weather condition prompts are time-neutral (no "sunny day" references)
- Night needs strong language: "NIGHTTIME SCENE. Dark night sky, no sunlight..."

---

## Future Enhancements

1. **Seasonal variation**: Adjust base prompt for seasons (autumn colors, winter bare trees)
2. **Animation**: Use HTMX to fade between images as conditions change
3. **User preference**: Allow users to toggle images on/off for data savings
4. **Manual regeneration**: Admin endpoint to force regenerate specific conditions
