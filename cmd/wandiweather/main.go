package main

import (
	"context"
	"database/sql"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/joho/godotenv"
	_ "modernc.org/sqlite"

	"github.com/lox/wandiweather/internal/api"
	"github.com/lox/wandiweather/internal/firedanger"
	"github.com/lox/wandiweather/internal/ingest"
	"github.com/lox/wandiweather/internal/models"
	"github.com/lox/wandiweather/internal/store"
)

var cli struct {
	DB           string `name:"db" default:"data/wandiweather.db" help:"Path to SQLite database."`
	Port         string `name:"port" default:"8080" env:"PORT" help:"HTTP server port."`
	NoPoll       bool   `name:"no-poll" help:"Disable polling (server only, for local dev)."`
	Once         bool   `name:"once" help:"Ingest once and exit (for testing)."`
	Backfill     bool   `name:"backfill" help:"Backfill 7-day observation history."`
	Daily        bool   `name:"daily" help:"Run daily jobs (summaries + verification) and exit."`
	BackfillDaily bool  `name:"backfill-daily" help:"Backfill all daily summaries and verification."`
	PWSApiKey    string `name:"pws-api-key" env:"PWS_API_KEY" required:"" help:"Weather Underground API key."`
}

var defaultStations = []models.Station{
	{StationID: "IWANDI23", Name: "Wandiligong (Primary)", Latitude: -36.794, Longitude: 146.977, Elevation: 386, ElevationTier: "valley_floor", IsPrimary: true, Active: true},
	{StationID: "IWANDI25", Name: "Wandiligong (Shade)", Latitude: -36.794, Longitude: 146.977, Elevation: 386, ElevationTier: "valley_floor", IsPrimary: false, Active: true},
	{StationID: "IBRIGH180", Name: "Bright", Latitude: -36.729, Longitude: 146.968, Elevation: 313, ElevationTier: "valley_floor", IsPrimary: false, Active: true},
	{StationID: "IVICTORI162", Name: "Wandiligong", Latitude: -36.757, Longitude: 146.986, Elevation: 392, ElevationTier: "valley_floor", IsPrimary: false, Active: false},
	{StationID: "IHARRI19", Name: "Harrietville", Latitude: -36.9, Longitude: 147.053, Elevation: 543, ElevationTier: "upper", IsPrimary: false, Active: true},
}

var stationIDs = []string{
	"IWANDI23",  // Primary station (valley floor)
	"IWANDI25",  // Shade reference (valley floor)
	"IBRIGH180", // Bright (valley floor)
	"IHARRI19",  // Harrietville (upper, for inversion detection)
}

const (
	wandiligongLat = -36.794
	wandiligongLon = 146.977
)

func init() {
	_ = godotenv.Load() // Load .env if present, ignore error if missing
}

func main() {
	kong.Parse(&cli,
		kong.Name("wandiweather"),
		kong.Description("Weather station data ingestion and display server."),
	)

	db, err := sql.Open("sqlite", cli.DB)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		log.Printf("warning: failed to set journal_mode=WAL: %v", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		log.Printf("warning: failed to set busy_timeout: %v", err)
	}

	// Load timezone once at startup
	loc, err := time.LoadLocation("Australia/Melbourne")
	if err != nil {
		log.Printf("Warning: could not load Australia/Melbourne timezone, using UTC: %v", err)
		loc = time.UTC
	}

	st := store.New(db, loc)
	if err := st.Migrate(); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	log.Println("database migrated")

	for _, station := range defaultStations {
		if err := st.UpsertStation(station); err != nil {
			log.Fatalf("upsert station %s: %v", station.StationID, err)
		}
	}
	log.Println("stations seeded")

	pws := ingest.NewPWS(cli.PWSApiKey)
	forecast := ingest.NewForecastClient(cli.PWSApiKey, wandiligongLat, wandiligongLon)
	scheduler := ingest.NewScheduler(st, pws, forecast, stationIDs, loc)
	server := api.NewServer(st, cli.Port, loc)

	// Configure image generation for weather banners, sharing mutex with server
	if gen := server.ImageGenerator(); gen != nil {
		scheduler.SetImageGenerator(gen, server.ImageCache(), server.ImageGenMutex())
	}

	// Share emergency client between server and scheduler
	scheduler.SetEmergencyClient(server.EmergencyClient())

	// Set up fire danger client for North East district
	scheduler.SetFireDangerClient(firedanger.NewNorthEastClient())

	if cli.Backfill {
		log.Println("backfilling 7-day observation history")
		if err := scheduler.BackfillHistory7Day(); err != nil {
			log.Fatalf("backfill: %v", err)
		}
	}

	if cli.BackfillDaily {
		log.Println("backfilling daily summaries and verification")
		if err := scheduler.BackfillDailySummaries(); err != nil {
			log.Fatalf("backfill summaries: %v", err)
		}
		if err := scheduler.BackfillVerification(); err != nil {
			log.Fatalf("backfill verification: %v", err)
		}
		log.Println("done")
		return
	}

	if cli.Daily {
		log.Println("running daily jobs")
		if err := scheduler.RunDailyJobs(); err != nil {
			log.Fatalf("daily jobs: %v", err)
		}
		log.Println("done")
		return
	}

	if cli.Once {
		log.Println("running single ingestion")
		if err := scheduler.IngestOnce(); err != nil {
			log.Fatalf("ingest: %v", err)
		}
		log.Println("done")
		return
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if !cli.NoPoll {
		go scheduler.Run(ctx)
	} else {
		log.Println("polling disabled (--no-poll)")
	}

	log.Printf("starting server on :%s", cli.Port)
	if err := server.Run(ctx); err != nil {
		log.Fatalf("server: %v", err)
	}
}
