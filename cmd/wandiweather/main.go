package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	_ "modernc.org/sqlite"

	"github.com/lox/wandiweather/internal/api"
	"github.com/lox/wandiweather/internal/ingest"
	"github.com/lox/wandiweather/internal/models"
	"github.com/lox/wandiweather/internal/store"
)

var defaultStations = []models.Station{
	// Valley floor (96-161m)
	{StationID: "IWANDI23", Name: "Wandiligong (Primary)", Latitude: -36.794, Longitude: 146.977, Elevation: 117, ElevationTier: "valley_floor", IsPrimary: true, Active: true},
	{StationID: "IWANDI24", Name: "Wandiligong", Latitude: -36.786, Longitude: 146.992, Elevation: 161, ElevationTier: "valley_floor", IsPrimary: false, Active: true},
	{StationID: "IBRIGH169", Name: "Bright", Latitude: -36.731, Longitude: 146.985, Elevation: 101, ElevationTier: "valley_floor", IsPrimary: false, Active: true},
	{StationID: "IBRIGH180", Name: "Bright", Latitude: -36.729, Longitude: 146.968, Elevation: 96, ElevationTier: "valley_floor", IsPrimary: false, Active: true},
	// Mid-slope (316-367m)
	{StationID: "IWANDI8", Name: "Wandiligong", Latitude: -36.779, Longitude: 146.977, Elevation: 364, ElevationTier: "mid_slope", IsPrimary: false, Active: false}, // Rain gauge broken
	{StationID: "IWANDI10", Name: "Wandiligong", Latitude: -36.767, Longitude: 146.981, Elevation: 355, ElevationTier: "mid_slope", IsPrimary: false, Active: true},
	{StationID: "IWANDI22", Name: "Wandiligong", Latitude: -36.767, Longitude: 146.982, Elevation: 367, ElevationTier: "mid_slope", IsPrimary: false, Active: true},
	{StationID: "IBRIGH55", Name: "Bright", Latitude: -36.742, Longitude: 146.973, Elevation: 336, ElevationTier: "mid_slope", IsPrimary: false, Active: true},
	{StationID: "IBRIGH127", Name: "Bright", Latitude: -36.732, Longitude: 146.973, Elevation: 316, ElevationTier: "mid_slope", IsPrimary: false, Active: true},
	// Upper (400m)
	{StationID: "IVICTORI162", Name: "Wandiligong Upper", Latitude: -36.757, Longitude: 146.986, Elevation: 400, ElevationTier: "upper", IsPrimary: false, Active: true},
}

var stationIDs = []string{
	"IWANDI23", "IWANDI24", "IBRIGH169", "IBRIGH180",
	"IWANDI10", "IWANDI22", "IBRIGH55", "IBRIGH127",
	"IVICTORI162",
}

const (
	wandiligongLat = -36.794
	wandiligongLon = 146.977
)

func main() {
	dbPath := flag.String("db", "data/wandiweather.db", "path to SQLite database")
	port := flag.String("port", "8080", "HTTP server port")
	once := flag.Bool("once", false, "ingest once and exit (for testing)")
	backfill := flag.Bool("backfill", false, "backfill 1-day history on startup")
	backfill7d := flag.Bool("backfill7d", false, "backfill 7-day hourly history")
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

	st := store.New(db)
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
	bom := ingest.NewBOMClient("")
	scheduler := ingest.NewScheduler(st, pws, forecast, bom, stationIDs)

	if *backfill7d {
		log.Println("backfilling 7-day history")
		if err := scheduler.BackfillHistory7Day(); err != nil {
			log.Fatalf("backfill7d: %v", err)
		}
	}

	if *backfill {
		log.Println("backfilling 1-day history")
		if err := scheduler.BackfillHistory(); err != nil {
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

	go scheduler.Run(ctx)

	server := api.NewServer(st, *port)
	log.Printf("starting server on :%s", *port)
	if err := server.Run(ctx); err != nil {
		log.Fatalf("server: %v", err)
	}
}
