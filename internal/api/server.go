package api

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"net/http/pprof"
	"sync"
	"time"

	"github.com/lox/wandiweather/internal/emergency"
	"github.com/lox/wandiweather/internal/imagegen"
	"github.com/lox/wandiweather/internal/store"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	_ "github.com/lox/wandiweather/internal/metrics" // Register metrics
)

// Server is the HTTP server for the weather API.
type Server struct {
	store           *store.Store
	port            string
	loc             *time.Location
	tmpl            *template.Template
	imageCache      *imagegen.Cache
	imageGen        *imagegen.Generator
	genMu           sync.Mutex // Prevents concurrent generation of same image
	emergencyClient *emergency.Client
	ogImageCache    *imagegen.OGImageCache
}

// NewServer creates a new Server instance.
func NewServer(store *store.Store, port string, loc *time.Location) *Server {
	tmpl := newTemplates()

	// Initialize image generator (optional - may not have API key)
	var imageGen *imagegen.Generator
	if gen, err := imagegen.NewGenerator(); err != nil {
		log.Printf("Image generation disabled: %v", err)
	} else {
		imageGen = gen
	}

	// Initialize VicEmergency client for Wandiligong area
	emergencyClient := emergency.NewClient(-36.794, 146.977, emergency.DefaultRadiusKM)

	return &Server{
		store:           store,
		port:            port,
		loc:             loc,
		tmpl:            tmpl,
		imageCache:      imagegen.NewCache("data/images"),
		imageGen:        imageGen,
		emergencyClient: emergencyClient,
		ogImageCache:    imagegen.NewOGImageCache(5 * time.Minute),
	}
}

// ImageGenerator returns the image generator for use by the scheduler.
func (s *Server) ImageGenerator() *imagegen.Generator {
	return s.imageGen
}

// ImageCache returns the image cache for use by the scheduler.
func (s *Server) ImageCache() *imagegen.Cache {
	return s.imageCache
}

// ImageGenMutex returns a pointer to the image generation mutex for coordinating
// between the HTTP handler and scheduler to prevent duplicate API calls.
func (s *Server) ImageGenMutex() *sync.Mutex {
	return &s.genMu
}

// EmergencyClient returns the VicEmergency client for use by the scheduler.
func (s *Server) EmergencyClient() *emergency.Client {
	return s.emergencyClient
}

// Handler returns the HTTP handler with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Page handlers
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/accuracy", s.handleAccuracy)
	mux.HandleFunc("/data", s.handleData)
	mux.HandleFunc("/health", s.handleHealth)

	// Partials (HTMX)
	mux.HandleFunc("/partials/current", s.handleCurrentPartial)
	mux.HandleFunc("/partials/chart", s.handleChartPartial)
	mux.HandleFunc("/partials/forecast", s.handleForecastPartial)

	// API endpoints
	mux.HandleFunc("/api/current", s.handleAPICurrent)
	mux.HandleFunc("/api/history", s.handleAPIHistory)
	mux.HandleFunc("/api/stations", s.handleAPIStations)
	mux.HandleFunc("/api/forecast", s.handleAPIForecast)

	// Image endpoints
	mux.HandleFunc("/weather-image", s.handleWeatherImage)
	mux.HandleFunc("/weather-image/", s.handleWeatherImage)
	mux.HandleFunc("/og-image", s.handleOGImage)

	// Metrics and debugging
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/heap", pprof.Handler("heap").ServeHTTP)
	mux.HandleFunc("/debug/pprof/goroutine", pprof.Handler("goroutine").ServeHTTP)
	mux.HandleFunc("/debug/pprof/allocs", pprof.Handler("allocs").ServeHTTP)

	return mux
}

// Run starts the HTTP server and blocks until the context is cancelled.
func (s *Server) Run(ctx context.Context) error {
	server := &http.Server{
		Addr:    ":" + s.port,
		Handler: s.Handler(),
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}
