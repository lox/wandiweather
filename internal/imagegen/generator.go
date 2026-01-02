package imagegen

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/lox/wandiweather/internal/forecast"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// Generator handles weather image generation using OpenAI's API.
type Generator struct {
	client openai.Client
	model  string
}

// NewGenerator creates a new image generator.
// It reads the OPENAI_API_KEY environment variable for authentication.
func NewGenerator() (*Generator, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, errors.New("OPENAI_API_KEY environment variable not set")
	}

	client := openai.NewClient(
		option.WithAPIKey(apiKey),
	)

	return &Generator{
		client: client,
		model:  "gpt-image-1", // Using standard model for better quality
	}, nil
}

// Generate creates an image for the given weather condition (includes time of day).
// The condition should already include time suffix (e.g., "clear_warm_night").
// Returns the image as PNG bytes.
func (g *Generator) Generate(ctx context.Context, condition forecast.WeatherCondition, tod forecast.TimeOfDay, t time.Time) ([]byte, error) {
	moon := forecast.GetMoonPhase(t)
	prompt := forecast.BuildPromptWithTimeAndMoon(condition, tod, moon)
	fullCondition := forecast.ConditionWithTime(condition, tod)

	log.Printf("Generating weather image for: %s (moon: %s)", fullCondition, moon)

	resp, err := g.client.Images.Generate(ctx, openai.ImageGenerateParams{
		Model:        g.model,
		Prompt:       prompt,
		Size:         openai.ImageGenerateParamsSize1536x1024, // Wide landscape for header
		Quality:      openai.ImageGenerateParamsQualityLow,    // Cost-effective
		OutputFormat: openai.ImageGenerateParamsOutputFormatPNG,
	})
	if err != nil {
		return nil, fmt.Errorf("image generation failed: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, errors.New("no image data returned")
	}

	// Decode base64 image data
	imageData := resp.Data[0].B64JSON
	if imageData == "" {
		return nil, errors.New("empty image data returned")
	}

	imageBytes, err := base64.StdEncoding.DecodeString(imageData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image data: %w", err)
	}

	log.Printf("Successfully generated image for: %s (%d bytes)", fullCondition, len(imageBytes))
	return imageBytes, nil
}
