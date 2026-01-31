package imagegen

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	"image/color"

	"image/png"
	"sync"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

//go:embed fonts/*.ttf
var fontsFS embed.FS

var (
	fontLight   font.Face
	fontRegular font.Face
	fontOnce    sync.Once
	fontErr     error
)

func loadFonts() {
	fontOnce.Do(func() {
		// Load Inter Regular for body text
		regularData, err := fontsFS.ReadFile("fonts/Inter-Regular.ttf")
		if err != nil {
			fontErr = fmt.Errorf("load Inter-Regular: %w", err)
			return
		}

		regularFont, err := opentype.Parse(regularData)
		if err != nil {
			fontErr = fmt.Errorf("parse Inter-Regular: %w", err)
			return
		}

		fontRegular, err = opentype.NewFace(regularFont, &opentype.FaceOptions{
			Size:    36,
			DPI:     72,
			Hinting: font.HintingFull,
		})
		if err != nil {
			fontErr = fmt.Errorf("create regular face: %w", err)
			return
		}

		// Load Inter Light for large temperature display
		lightData, err := fontsFS.ReadFile("fonts/Inter-Light.ttf")
		if err != nil {
			fontErr = fmt.Errorf("load Inter-Light: %w", err)
			return
		}

		lightFont, err := opentype.Parse(lightData)
		if err != nil {
			fontErr = fmt.Errorf("parse Inter-Light: %w", err)
			return
		}

		fontLight, err = opentype.NewFace(lightFont, &opentype.FaceOptions{
			Size:    120,
			DPI:     72,
			Hinting: font.HintingFull,
		})
		if err != nil {
			fontErr = fmt.Errorf("create light face: %w", err)
			return
		}
	})
}

// OGImageData contains the dynamic data for the OG image.
type OGImageData struct {
	Temperature float64 // Current temperature in Celsius
	Condition   string  // e.g., "Partly Cloudy", "Clear", "Rain"
}

// OGImageCache caches the generated OG image for a short period.
type OGImageCache struct {
	mu        sync.RWMutex
	data      []byte
	expiresAt time.Time
	cacheTTL  time.Duration
}

// NewOGImageCache creates a new OG image cache with the specified TTL.
func NewOGImageCache(ttl time.Duration) *OGImageCache {
	return &OGImageCache{
		cacheTTL: ttl,
	}
}

// Get returns the cached OG image if still valid.
func (c *OGImageCache) Get() ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.data == nil || time.Now().After(c.expiresAt) {
		return nil, false
	}
	return c.data, true
}

// Set stores a new OG image in the cache.
func (c *OGImageCache) Set(data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data = data
	c.expiresAt = time.Now().Add(c.cacheTTL)
}

// OGWidth and OGHeight are the standard Open Graph image dimensions.
const (
	OGWidth  = 1200
	OGHeight = 630
)

// GenerateOGImage creates an OG image by compositing the weather image with text overlay.
func GenerateOGImage(weatherImage []byte, data OGImageData) ([]byte, error) {
	loadFonts()
	if fontErr != nil {
		return nil, fmt.Errorf("load fonts: %w", fontErr)
	}

	// Decode the source weather image
	src, _, err := image.Decode(bytes.NewReader(weatherImage))
	if err != nil {
		return nil, fmt.Errorf("decode weather image: %w", err)
	}

	// Create the destination image at OG dimensions
	dst := image.NewRGBA(image.Rect(0, 0, OGWidth, OGHeight))

	// Calculate crop/scale to fill OG dimensions (center crop)
	srcBounds := src.Bounds()
	srcW, srcH := srcBounds.Dx(), srcBounds.Dy()

	// Calculate the scaling to cover the destination
	scaleX := float64(OGWidth) / float64(srcW)
	scaleY := float64(OGHeight) / float64(srcH)
	scale := scaleX
	if scaleY > scaleX {
		scale = scaleY
	}

	// Calculate the scaled dimensions and offset for center crop
	scaledW := int(float64(srcW) * scale)
	scaledH := int(float64(srcH) * scale)
	offsetX := (scaledW - OGWidth) / 2
	offsetY := (scaledH - OGHeight) / 2

	// Simple nearest-neighbor resize and crop
	for y := 0; y < OGHeight; y++ {
		for x := 0; x < OGWidth; x++ {
			srcX := int(float64(x+offsetX) / scale)
			srcY := int(float64(y+offsetY) / scale)
			if srcX >= 0 && srcX < srcW && srcY >= 0 && srcY < srcH {
				dst.Set(x, y, src.At(srcBounds.Min.X+srcX, srcBounds.Min.Y+srcY))
			}
		}
	}

	// Draw a gradient overlay at the bottom for text readability
	drawGradientOverlay(dst)

	// Draw text overlay
	drawTextOverlay(dst, data)

	// Encode to PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, fmt.Errorf("encode OG image: %w", err)
	}

	return buf.Bytes(), nil
}

// drawGradientOverlay draws a dark gradient at the bottom of the image.
func drawGradientOverlay(img *image.RGBA) {
	bounds := img.Bounds()
	gradientHeight := 300

	for y := bounds.Max.Y - gradientHeight; y < bounds.Max.Y; y++ {
		progress := float64(y-(bounds.Max.Y-gradientHeight)) / float64(gradientHeight)
		// Ease-in curve for smoother gradient
		progress = progress * progress
		alpha := progress * 0.85 // Max 85% opacity

		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			orig := img.RGBAAt(x, y)
			// Blend with black
			orig.R = uint8(float64(orig.R) * (1 - alpha))
			orig.G = uint8(float64(orig.G) * (1 - alpha))
			orig.B = uint8(float64(orig.B) * (1 - alpha))
			img.SetRGBA(x, y, orig)
		}
	}
}

// drawTextOverlay draws the weather information text on the image.
func drawTextOverlay(img *image.RGBA, data OGImageData) {
	white := color.RGBA{255, 255, 255, 255}
	lightGray := color.RGBA{200, 200, 200, 255}

	// Draw large temperature (light weight, like the site)
	tempStr := fmt.Sprintf("%.0f°", data.Temperature)
	drawText(img, tempStr, 60, OGHeight-180, white, fontLight)

	// Draw condition below temperature
	if data.Condition != "" {
		drawText(img, data.Condition, 60, OGHeight-80, lightGray, fontRegular)
	}

	// Draw "wandiweather.com" at bottom
	drawText(img, "wandiweather.com", 60, OGHeight-30, lightGray, fontRegular)
}

// drawText draws text at the given position using the specified font face.
func drawText(img *image.RGBA, text string, x, y int, col color.Color, face font.Face) {
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  fixed.Point26_6{X: fixed.I(x), Y: fixed.I(y)},
	}
	d.DrawString(text)
}

// GenerateFallbackOGImage creates a simple fallback OG image when no weather image is available.
func GenerateFallbackOGImage(data OGImageData) ([]byte, error) {
	loadFonts()
	if fontErr != nil {
		return nil, fmt.Errorf("load fonts: %w", fontErr)
	}

	// Create image with dark blue gradient background
	img := image.NewRGBA(image.Rect(0, 0, OGWidth, OGHeight))

	// Draw gradient background
	for y := 0; y < OGHeight; y++ {
		progress := float64(y) / float64(OGHeight)
		r := uint8(20 + progress*10)
		g := uint8(20 + progress*15)
		b := uint8(40 + progress*20)
		for x := 0; x < OGWidth; x++ {
			img.SetRGBA(x, y, color.RGBA{r, g, b, 255})
		}
	}

	// Draw text overlay
	drawTextOverlay(img, data)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode fallback OG image: %w", err)
	}

	return buf.Bytes(), nil
}
