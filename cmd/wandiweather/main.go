package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "modernc.org/sqlite"

	"github.com/lox/wandiweather/internal/api"
	"github.com/lox/wandiweather/internal/ingest"
	"github.com/lox/wandiweather/internal/models"
	"github.com/lox/wandiweather/internal/store"
)

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

func main() {
	dbPath := flag.String("db", "data/wandiweather.db", "path to SQLite database")
	port := flag.String("port", "8080", "HTTP server port")
	noPoll := flag.Bool("no-poll", false, "disable polling (server only, for local dev)")
	once := flag.Bool("once", false, "ingest once and exit (for testing)")
	backfill := flag.Bool("backfill", false, "backfill 7-day observation history")
	dailyJobs := flag.Bool("daily", false, "run daily jobs (summaries + verification) and exit")
	backfillDaily := flag.Bool("backfill-daily", false, "backfill all daily summaries and verification")
	flag.Parse()

	apiKey := os.Getenv("PWS_API_KEY")
	if apiKey == "" {
		log.Fatal("PWS_API_KEY environment variable required")
	}

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA busy_timeout=5000")

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

	pws := ingest.NewPWS(apiKey)
	forecast := ingest.NewForecastClient(apiKey, wandiligongLat, wandiligongLon)
	scheduler := ingest.NewScheduler(st, pws, forecast, stationIDs, loc)
	server := api.NewServer(st, *port, loc)

	// Configure image generation for weather banners, sharing mutex with server
	if gen := server.ImageGenerator(); gen != nil {
		scheduler.SetImageGenerator(gen, server.ImageCache(), server.ImageGenMutex())
	}

	if *backfill {
		log.Println("backfilling 7-day observation history")
		if err := scheduler.BackfillHistory7Day(); err != nil {
			log.Fatalf("backfill: %v", err)
		}
	}

	if *backfillDaily {
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

	if *dailyJobs {
		log.Println("running daily jobs")
		if err := scheduler.RunDailyJobs(); err != nil {
			log.Fatalf("daily jobs: %v", err)
		}
		log.Println("done")
		return
	}

	if *once {
		log.Println("running single ingestion")
		if err := scheduler.IngestOnce(); err != nil {
			log.Fatalf("ingest: %v", err)
		}
		log.Println("done")
		return
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if !*noPoll {
		go scheduler.Run(ctx)
	} else {
		log.Println("polling disabled (--no-poll)")
	}

	log.Printf("starting server on :%s", *port)
	if err := server.Run(ctx); err != nil {
		log.Fatalf("server: %v", err)
	}
}
