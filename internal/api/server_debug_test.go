package api_test

import (
	"net/http/httptest"
	"testing"

	"github.com/lox/wandiweather/internal/api"
)

func TestPprofRoutesDisabledByDefault(t *testing.T) {
	t.Parallel()

	st, loc := setupTestStore(t)
	srv := api.NewServer(st, "8080", loc)

	req := httptest.NewRequest("GET", "/debug/pprof/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 404 {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestPprofRoutesEnabledWhenConfigured(t *testing.T) {
	t.Parallel()

	st, loc := setupTestStore(t)
	srv := api.NewServer(st, "8080", loc)
	srv.SetDebugRoutesEnabled(true)

	req := httptest.NewRequest("GET", "/debug/pprof/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}
