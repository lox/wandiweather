package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/joho/godotenv"
	_ "modernc.org/sqlite"

	"github.com/lox/wandiweather/internal/api"
	"github.com/lox/wandiweather/internal/ecowitt"
	"github.com/lox/wandiweather/internal/firedanger"
	"github.com/lox/wandiweather/internal/ingest"
	"github.com/lox/wandiweather/internal/models"
	"github.com/lox/wandiweather/internal/store"
)

var cli struct {
	DB                     string `name:"db" default:"data/wandiweather.db" help:"Path to SQLite database."`
	Port                   string `name:"port" default:"8080" env:"PORT" help:"HTTP server port."`
	NoPoll                 bool   `name:"no-poll" help:"Disable polling (server only, for local dev)."`
	Once                   bool   `name:"once" help:"Ingest once and exit (for testing)."`
	Backfill               bool   `name:"backfill" help:"Backfill 7-day observation history."`
	AirQualityBackfillDays int    `name:"air-quality-backfill-days" default:"0" help:"Backfill Ecowitt air quality history for the last N days and exit (max 90)."`
	Daily                  bool   `name:"daily" help:"Run daily jobs (summaries + verification) and exit."`
	BackfillDaily          bool   `name:"backfill-daily" help:"Backfill all daily summaries and verification."`
	PWSApiKey              string `name:"pws-api-key" env:"PWS_API_KEY" required:"" help:"Weather Underground API key."`
	EcowittAPIKey          string `name:"ecowitt-api-key" env:"ECOWITT_API_KEY" help:"Ecowitt cloud API key."`
	EcowittAppKey          string `name:"ecowitt-app-key" env:"ECOWITT_APP_KEY" help:"Ecowitt cloud application key."`
	EcowittMAC             string `name:"ecowitt-mac" env:"ECOWITT_MAC" help:"Ecowitt device MAC address."`
}

var defaultStations = []models.Station{
	{StationID: "IWANDI23", Name: "Wandiligong (Primary)", Latitude: -36.794, Longitude: 146.977, Elevation: 386, ElevationTier: "valley_floor", IsPrimary: true, Active: true},
	{StationID: "IWANDI25", Name: "Wandiligong (Shade)", Latitude: -36.794, Longitude: 146.977, Elevation: 386, ElevationTier: "valley_floor", IsPrimary: false, Active: true},
	{StationID: "IBRIGH180", Name: "Bright", Latitude: -36.729, Longitude: 146.968, Elevation: 313, ElevationTier: "valley_floor", IsPrimary: false, Active: true},
	{StationID: "IVICTORI162", Name: "Wandiligong", Latitude: -36.757, Longitude: 146.986, Elevation: 392, ElevationTier: "valley_floor", IsPrimary: false, Active: false},
	{StationID: "IHARRI6", Name: "Harrietville", Latitude: -36.900, Longitude: 147.057, Elevation: 520, ElevationTier: "upper", IsPrimary: false, Active: true},
	{StationID: "IHARRI19", Name: "Harrietville", Latitude: -36.9, Longitude: 147.053, Elevation: 543, ElevationTier: "upper", IsPrimary: false, Active: false},
}

var stationIDs = []string{
	"IWANDI23",  // Primary station (valley floor)
	"IWANDI25",  // Shade reference (valley floor)
	"IBRIGH180", // Bright (valley floor)
	"IHARRI6",   // Harrietville (upper, for inversion detection)
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

	ecowittAppKey := cli.EcowittAppKey
	if ecowittAppKey == "" {
		ecowittAppKey = os.Getenv("ECOWITT_APPLICATION_KEY")
	}

	var ecowittClient *ecowitt.Client
	switch {
	case cli.EcowittAPIKey == "" && ecowittAppKey == "" && cli.EcowittMAC == "":
		// Air quality is optional.
	case cli.EcowittAPIKey == "" || ecowittAppKey == "" || cli.EcowittMAC == "":
		log.Printf("Ecowitt air quality disabled: set ECOWITT_API_KEY, ECOWITT_APP_KEY (or ECOWITT_APPLICATION_KEY), and ECOWITT_MAC")
	default:
		ecowittClient, err = ecowitt.NewClient(ecowittAppKey, cli.EcowittAPIKey, cli.EcowittMAC)
		if err != nil {
			log.Printf("Ecowitt air quality disabled: %v", err)
		}
	}

	server := api.NewServer(st, cli.Port, loc, ecowittClient)
	scheduler.SetAirQualityClient(ecowittClient)

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

	if cli.AirQualityBackfillDays > 0 {
		log.Printf("backfilling %d days of Ecowitt air quality history", cli.AirQualityBackfillDays)
		if err := scheduler.BackfillAirQualityHistory(cli.AirQualityBackfillDays); err != nil {
			log.Fatalf("air quality backfill: %v", err)
		}
		log.Println("done")
		return
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
