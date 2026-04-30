package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lox/wandiweather/internal/api"
)

func TestWeatherImagePreviewsPage(t *testing.T) {
	t.Parallel()

	s, loc := setupTestStore(t)
	srv := api.NewServer(s, "8080", loc)

	req := httptest.NewRequest(http.MethodGet, "/weather-image-previews", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "<title>Weather Image Previews - WandiWeather</title>") {
		t.Fatal("expected weather image previews title")
	}
	if !strings.Contains(body, "/weather-image?weather=clear_warm_day") {
		t.Fatal("expected clear warm daytime preview image")
	}
	if !strings.Contains(body, "/weather-image?weather=storm_night&amp;smoke=dense_smoke") {
		t.Fatal("expected smoke-aware storm night preview image")
	}
}
