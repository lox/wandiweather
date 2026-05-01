package api

import (
	"net/http/httptest"
	"testing"

	"github.com/lox/wandiweather/internal/forecast"
)

func TestServeBannerImage_DisablesBrowserCaching(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()

	(&Server{}).serveBannerImage(w, []byte("png"))

	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if got := w.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", got)
	}
}

func TestParseSmokeOverride(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		want   forecast.SmokeLevel
		wantOK bool
	}{
		{name: "clear", input: "clear", want: forecast.SmokeClear, wantOK: true},
		{name: "haze", input: "haze", want: forecast.SmokeHaze, wantOK: true},
		{name: "smoke", input: "smoke", want: forecast.SmokeVisible, wantOK: true},
		{name: "dense smoke", input: "dense_smoke", want: forecast.SmokeDense, wantOK: true},
		{name: "invalid", input: "wildfire", want: "", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseSmokeOverride(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("parseSmokeOverride() ok = %t, want %t", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Fatalf("parseSmokeOverride() = %v, want %v", got, tt.want)
			}
		})
	}
}
