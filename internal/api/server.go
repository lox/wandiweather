package api

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lox/wandiweather/internal/emergency"
	"github.com/lox/wandiweather/internal/firedanger"
	"github.com/lox/wandiweather/internal/forecast"
	"github.com/lox/wandiweather/internal/imagegen"
	"github.com/lox/wandiweather/internal/models"
	"github.com/lox/wandiweather/internal/store"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	_ "github.com/lox/wandiweather/internal/metrics" // Register metrics
)

//go:embed templates/*
var templateFS embed.FS

type Server struct {
	store           *store.Store
	port            string
	loc             *time.Location
	tmpl            *template.Template
	imageCache      *imagegen.Cache
	imageGen        *imagegen.Generator
	genMu           sync.Mutex // Prevents concurrent generation of same image
	emergencyClient *emergency.Client
}

func NewServer(store *store.Store, port string, loc *time.Location) *Server {
	funcs := template.FuncMap{
		"deref": func(f *float64) float64 {
			if f == nil {
				return 0
			}
			return *f
		},
		"abs": func(f float64) float64 {
			if f < 0 {
				return -f
			}
			return f
		},
		"neg": func(f float64) float64 {
			return -f
		},
		"upper": strings.ToUpper,
	}
	tmpl := template.Must(template.New("").Funcs(funcs).ParseFS(templateFS, "templates/*.html"))

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

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/accuracy", s.handleAccuracy)
	mux.HandleFunc("/health", s.handleHealth)
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/weather-image", s.handleWeatherImage)
	mux.HandleFunc("/weather-image/", s.handleWeatherImage)
	mux.HandleFunc("/partials/current", s.handleCurrentPartial)
	mux.HandleFunc("/partials/chart", s.handleChartPartial)
	mux.HandleFunc("/partials/forecast", s.handleForecastPartial)
	mux.HandleFunc("/api/current", s.handleAPICurrent)
	mux.HandleFunc("/api/history", s.handleAPIHistory)
	mux.HandleFunc("/api/stations", s.handleAPIStations)
	mux.HandleFunc("/api/forecast", s.handleAPIForecast)
	return mux
}

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

type CurrentData struct {
	Primary        *models.Observation
	ValleyTemp     float64
	TempChangeRate *float64
	FeelsLike      *float64
	Stations       map[string]*models.Observation
	StationMeta    map[string]models.Station
	AllStations    []StationReading
	ValleyFloor    []StationReading
	MidSlope       []StationReading
	Upper          []StationReading
	Inversion      *InversionStatus
	TodayForecast  *TodayForecast
	TodayStats     *TodayStats
	LastUpdated    time.Time
	Moon           *MoonData
	Alerts         []emergency.Alert
	UrgentAlerts   []emergency.Alert
	FireDanger     *firedanger.DayForecast
}

// MoonData contains moon phase information for display.
type MoonData struct {
	Phase        string // e.g., "Waxing Gibbous"
	Illumination int    // 0-100 percentage
	Emoji        string // 🌑🌒🌓🌔🌕🌖🌗🌘
}

// moonEmoji returns the appropriate moon phase emoji.
func moonEmoji(phase forecast.MoonPhase) string {
	switch phase {
	case forecast.MoonNew:
		return "🌑"
	case forecast.MoonWaxingCrescent:
		return "🌒"
	case forecast.MoonFirstQuarter:
		return "🌓"
	case forecast.MoonWaxingGibbous:
		return "🌔"
	case forecast.MoonFull:
		return "🌕"
	case forecast.MoonWaningGibbous:
		return "🌖"
	case forecast.MoonLastQuarter:
		return "🌗"
	case forecast.MoonWaningCrescent:
		return "🌘"
	default:
		return "🌙"
	}
}

// IndexData wraps CurrentData with additional page-level data.
type IndexData struct {
	*CurrentData
	Palette         forecast.Palette
	WeatherOverride string // Optional override for testing (e.g., "storm_night")
}

type TodayForecast struct {
	TempMax           float64
	TempMin           float64
	TempMaxRaw        float64
	NowcastApplied    bool
	NowcastAdjustment float64
	PrecipChance      int64
	PrecipAmount      float64
	Narrative         string
	HasPrecip         bool
	// Explanation tracks how the forecast was calculated
	Explanation       ForecastExplanation
}

type ForecastExplanation struct {
	MaxSource         string  // "bom" or "wu"
	MaxRaw            float64 // raw forecast value
	MaxBiasApplied    float64 // bias correction applied
	MaxBiasDayUsed    int     // which day's bias was used (-1 if none)
	MaxBiasSamples    int     // how many samples the bias is based on
	MaxBiasFallback   bool    // true if fallback day was used
	MaxNowcast        float64 // nowcast adjustment (if any)
	MaxFinal          float64 // final displayed value
	MinSource         string
	MinRaw            float64
	MinBiasApplied    float64
	MinBiasDayUsed    int     // which day's bias was used (-1 if none)
	MinBiasSamples    int     // how many samples the bias is based on
	MinBiasFallback   bool    // true if fallback day was used
	MinFinal          float64
}

type TodayStats struct {
	MinTemp     float64
	MaxTemp     float64
	MinTempTime string
	MaxTempTime string
	RainTotal   float64
	HasRain     bool
	MaxWind     float64
	MaxGust     float64
	HasWind     bool
}

type StationReading struct {
	Station models.Station
	Obs     *models.Observation
}

type InversionStatus struct {
	Active      bool
	Strength    float64
	ValleyAvg   float64
	MidAvg      float64
	UpperAvg    float64
}

func (s *Server) getCurrentData() (*CurrentData, error) {
	stations, err := s.store.GetActiveStations()
	if err != nil {
		return nil, err
	}

	data := &CurrentData{
		Stations:    make(map[string]*models.Observation),
		StationMeta: make(map[string]models.Station),
	}

	var valleyTemps, midTemps, upperTemps []float64

	for _, st := range stations {
		data.StationMeta[st.StationID] = st
		obs, err := s.store.GetLatestObservation(st.StationID)
		if err != nil {
			log.Printf("get latest %s: %v", st.StationID, err)
			continue
		}
		if obs == nil {
			continue
		}
		data.Stations[st.StationID] = obs

		if st.IsPrimary {
			data.Primary = obs
			data.LastUpdated = obs.ObservedAt.In(s.loc)
		}

		reading := StationReading{Station: st, Obs: obs}
		data.AllStations = append(data.AllStations, reading)
		switch st.ElevationTier {
		case "valley_floor":
			data.ValleyFloor = append(data.ValleyFloor, reading)
			if obs.Temp.Valid {
				valleyTemps = append(valleyTemps, obs.Temp.Float64)
			}
		case "mid_slope":
			data.MidSlope = append(data.MidSlope, reading)
			if obs.Temp.Valid {
				midTemps = append(midTemps, obs.Temp.Float64)
			}
		case "upper":
			data.Upper = append(data.Upper, reading)
			if obs.Temp.Valid {
				upperTemps = append(upperTemps, obs.Temp.Float64)
			}
		case "local":
			data.ValleyFloor = append(data.ValleyFloor, reading)
			if obs.Temp.Valid {
				valleyTemps = append(valleyTemps, obs.Temp.Float64)
			}
		}
	}

	if len(valleyTemps) > 0 {
		data.ValleyTemp = median(valleyTemps)
		
		if len(upperTemps) > 0 {
			valleyAvg := avg(valleyTemps)
			midAvg := avg(midTemps)
			upperAvg := avg(upperTemps)
			expectedDiff := (400.0 - 117.0) / 1000.0 * 6.5
			actualDiff := upperAvg - valleyAvg

			data.Inversion = &InversionStatus{
				Active:    actualDiff > expectedDiff+2,
				Strength:  actualDiff - expectedDiff,
				ValleyAvg: valleyAvg,
				MidAvg:    midAvg,
				UpperAvg:  upperAvg,
			}
		}
	}

	loc := s.loc
	now := time.Now().In(loc)
	todayDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	// Add moon phase data
	phase := forecast.GetMoonPhase(now)
	phaseName, _ := forecast.MoonDescription(phase)
	data.Moon = &MoonData{
		Phase:        phaseName,
		Illumination: forecast.MoonIllumination(now),
		Emoji:        moonEmoji(phase),
	}

	todayStats, err := s.store.GetTodayStatsExtended("IWANDI23", now)
	if err == nil {
		ts := &TodayStats{}
		if todayStats.MinTemp.Valid {
			ts.MinTemp = todayStats.MinTemp.Float64
		}
		if todayStats.MaxTemp.Valid {
			ts.MaxTemp = todayStats.MaxTemp.Float64
		}
		if todayStats.MinTempTime.Valid {
			ts.MinTempTime = todayStats.MinTempTime.Time.In(loc).Format("3:04 PM")
		}
		if todayStats.MaxTempTime.Valid {
			ts.MaxTempTime = todayStats.MaxTempTime.Time.In(loc).Format("3:04 PM")
		}
		if todayStats.RainTotal.Valid && todayStats.RainTotal.Float64 > 0 {
			ts.RainTotal = todayStats.RainTotal.Float64
			ts.HasRain = true
		}
		if todayStats.MaxWind.Valid || todayStats.MaxGust.Valid {
			ts.MaxWind = todayStats.MaxWind.Float64
			ts.MaxGust = todayStats.MaxGust.Float64
			ts.HasWind = true
		}
		data.TodayStats = ts
	}

	if rate, err := s.store.GetTempChangeRate("IWANDI23"); err == nil && rate.Valid {
		data.TempChangeRate = &rate.Float64
	}

	if data.Primary != nil {
		if data.Primary.Temp.Valid {
			temp := data.Primary.Temp.Float64
			if temp >= 27 && data.Primary.HeatIndex.Valid {
				data.FeelsLike = &data.Primary.HeatIndex.Float64
			} else if temp <= 10 && data.Primary.WindChill.Valid {
				data.FeelsLike = &data.Primary.WindChill.Float64
			}
		}
	}

	forecasts, err := s.store.GetLatestForecasts()
	if err == nil {
		correctionStats, _ := s.store.GetAllCorrectionStats()
		nowcaster := forecast.NewNowcaster(s.store, s.loc)
		biasCorrector := forecast.NewBiasCorrector(s.store)

		var primaryStationID string
		for _, st := range stations {
			if st.IsPrimary {
				primaryStationID = st.StationID
				break
			}
		}

		todayStr := todayDate.Format("2006-01-02")

		// Find today's forecasts from both sources
		// Prefer forecasts that have valid temp data (skip day-0 entries with NULL temps)
		var wuForecast, bomForecast *models.Forecast
		for _, fc := range forecasts["wu"] {
			if fc.ValidDate.Format("2006-01-02") == todayStr && (fc.TempMax.Valid || fc.TempMin.Valid) {
				f := fc
				wuForecast = &f
				break
			}
		}
		for _, fc := range forecasts["bom"] {
			if fc.ValidDate.Format("2006-01-02") == todayStr && (fc.TempMax.Valid || fc.TempMin.Valid) {
				f := fc
				bomForecast = &f
				break
			}
		}

		if wuForecast != nil || bomForecast != nil {
			tf := &TodayForecast{}
			exp := &tf.Explanation

			// MAX TEMP: prefer BOM (better accuracy)
			if bomForecast != nil && bomForecast.TempMax.Valid {
				exp.MaxSource = "bom"
				exp.MaxRaw = bomForecast.TempMax.Float64
				tf.TempMax = bomForecast.TempMax.Float64

				biasResult := getCorrectionBiasWithFallback(correctionStats, "bom", "tmax", bomForecast.DayOfForecast)
				if biasResult.DayUsed >= 0 {
					exp.MaxBiasApplied = biasResult.Bias
					exp.MaxBiasDayUsed = biasResult.DayUsed
					exp.MaxBiasSamples = biasResult.Samples
					exp.MaxBiasFallback = biasResult.IsFallback
					tf.TempMax = bomForecast.TempMax.Float64 - biasResult.Bias
				} else {
					exp.MaxBiasDayUsed = -1
				}
				tf.TempMaxRaw = math.Round(tf.TempMax)

				// Nowcast using BOM as base
				if bomForecast.DayOfForecast == 0 && primaryStationID != "" {
					biasMax := biasCorrector.GetCorrection("bom", "tmax", 0)
					nowcast, err := nowcaster.ComputeNowcast(primaryStationID, bomForecast.TempMax.Float64, biasMax)
					if err == nil && nowcast != nil {
						exp.MaxNowcast = nowcast.Adjustment
						tf.TempMax = nowcast.CorrectedMax
						tf.NowcastApplied = true
						tf.NowcastAdjustment = nowcast.Adjustment
						if err := nowcaster.LogNowcast(primaryStationID, bomForecast.TempMax.Float64, nowcast); err != nil {
							log.Printf("api: log nowcast: %v", err)
						}
					}
				}
				tf.TempMax = math.Round(tf.TempMax)
				exp.MaxFinal = tf.TempMax
			} else if wuForecast != nil && wuForecast.TempMax.Valid {
				// Fallback to WU if BOM unavailable
				exp.MaxSource = "wu"
				exp.MaxRaw = wuForecast.TempMax.Float64
				tf.TempMax = wuForecast.TempMax.Float64

				biasResult := getCorrectionBiasWithFallback(correctionStats, "wu", "tmax", wuForecast.DayOfForecast)
				if biasResult.DayUsed >= 0 {
					exp.MaxBiasApplied = biasResult.Bias
					exp.MaxBiasDayUsed = biasResult.DayUsed
					exp.MaxBiasSamples = biasResult.Samples
					exp.MaxBiasFallback = biasResult.IsFallback
					tf.TempMax = wuForecast.TempMax.Float64 - biasResult.Bias
				} else {
					exp.MaxBiasDayUsed = -1
				}
				tf.TempMaxRaw = math.Round(tf.TempMax)
				tf.TempMax = math.Round(tf.TempMax)
				exp.MaxFinal = tf.TempMax
			}

			// MIN TEMP: prefer WU (better accuracy)
			if wuForecast != nil && wuForecast.TempMin.Valid {
				exp.MinSource = "wu"
				exp.MinRaw = wuForecast.TempMin.Float64
				tf.TempMin = wuForecast.TempMin.Float64

				biasResult := getCorrectionBiasWithFallback(correctionStats, "wu", "tmin", wuForecast.DayOfForecast)
				if biasResult.DayUsed >= 0 {
					exp.MinBiasApplied = biasResult.Bias
					exp.MinBiasDayUsed = biasResult.DayUsed
					exp.MinBiasSamples = biasResult.Samples
					exp.MinBiasFallback = biasResult.IsFallback
					tf.TempMin = wuForecast.TempMin.Float64 - biasResult.Bias
				} else {
					exp.MinBiasDayUsed = -1
				}
				tf.TempMin = math.Round(tf.TempMin)
				exp.MinFinal = tf.TempMin
			} else if bomForecast != nil && bomForecast.TempMin.Valid {
				// Fallback to BOM if WU unavailable
				exp.MinSource = "bom"
				exp.MinRaw = bomForecast.TempMin.Float64
				tf.TempMin = bomForecast.TempMin.Float64

				biasResult := getCorrectionBiasWithFallback(correctionStats, "bom", "tmin", bomForecast.DayOfForecast)
				if biasResult.DayUsed >= 0 {
					exp.MinBiasApplied = biasResult.Bias
					exp.MinBiasDayUsed = biasResult.DayUsed
					exp.MinBiasSamples = biasResult.Samples
					exp.MinBiasFallback = biasResult.IsFallback
					tf.TempMin = bomForecast.TempMin.Float64 - biasResult.Bias
				} else {
					exp.MinBiasDayUsed = -1
				}
				tf.TempMin = math.Round(tf.TempMin)
				exp.MinFinal = tf.TempMin
			}

			// Precip from WU (has more detail)
			if wuForecast != nil {
				if wuForecast.PrecipChance.Valid {
					tf.PrecipChance = wuForecast.PrecipChance.Int64
					tf.HasPrecip = wuForecast.PrecipChance.Int64 > 10
				}
				if wuForecast.PrecipAmount.Valid {
					tf.PrecipAmount = wuForecast.PrecipAmount.Float64
				}
			}

			// Build narrative
			day := &ForecastDay{WU: wuForecast, BOM: bomForecast}
			if bomForecast != nil && bomForecast.TempMax.Valid {
				day.BOMCorrectedMax = &tf.TempMax
			}
			if wuForecast != nil && wuForecast.TempMin.Valid {
				day.WUCorrectedMin = &tf.TempMin
			}
			tf.Narrative = buildGeneratedNarrative(day)

			data.TodayForecast = tf

			// Log displayed forecast for accuracy tracking
			// Only log when both WU and BOM forecasts are present to ensure unique index works
			// (SQLite treats NULL as distinct, so ON CONFLICT wouldn't dedupe otherwise)
			if wuForecast != nil && bomForecast != nil {
				dayOfForecast := bomForecast.DayOfForecast
				df := models.DisplayedForecast{
					DisplayedAt:   time.Now().UTC(),
					ValidDate:     todayDate,
					DayOfForecast: dayOfForecast,
				}
				df.WUForecastID = sql.NullInt64{Int64: wuForecast.ID, Valid: true}
				df.BOMForecastID = sql.NullInt64{Int64: bomForecast.ID, Valid: true}
				df.RawTempMax = sql.NullFloat64{Float64: exp.MaxRaw, Valid: exp.MaxSource != ""}
				df.RawTempMin = sql.NullFloat64{Float64: exp.MinRaw, Valid: exp.MinSource != ""}
				df.CorrectedTempMax = sql.NullFloat64{Float64: exp.MaxFinal, Valid: exp.MaxSource != ""}
				df.CorrectedTempMin = sql.NullFloat64{Float64: exp.MinFinal, Valid: exp.MinSource != ""}
				df.BiasAppliedMax = sql.NullFloat64{Float64: exp.MaxBiasApplied, Valid: exp.MaxBiasDayUsed >= 0}
				df.BiasAppliedMin = sql.NullFloat64{Float64: exp.MinBiasApplied, Valid: exp.MinBiasDayUsed >= 0}
				df.BiasDayUsedMax = sql.NullInt64{Int64: int64(exp.MaxBiasDayUsed), Valid: exp.MaxBiasDayUsed >= 0}
				df.BiasDayUsedMin = sql.NullInt64{Int64: int64(exp.MinBiasDayUsed), Valid: exp.MinBiasDayUsed >= 0}
				df.BiasSamplesMax = sql.NullInt64{Int64: int64(exp.MaxBiasSamples), Valid: exp.MaxBiasDayUsed >= 0}
				df.BiasSamplesMin = sql.NullInt64{Int64: int64(exp.MinBiasSamples), Valid: exp.MinBiasDayUsed >= 0}
				df.BiasFallbackMax = sql.NullBool{Bool: exp.MaxBiasFallback, Valid: exp.MaxBiasDayUsed >= 0}
				df.BiasFallbackMin = sql.NullBool{Bool: exp.MinBiasFallback, Valid: exp.MinBiasDayUsed >= 0}
				df.SourceMax = sql.NullString{String: exp.MaxSource, Valid: exp.MaxSource != ""}
				df.SourceMin = sql.NullString{String: exp.MinSource, Valid: exp.MinSource != ""}

				if err := s.store.UpsertDisplayedForecast(df); err != nil {
					log.Printf("api: log displayed forecast: %v", err)
				}
			}
		}
	}

	// Get emergency alerts from database (populated by scheduler)
	// Alerts older than 30 mins are considered stale
	if alerts, err := s.store.GetActiveAlerts(30 * time.Minute); err != nil {
		log.Printf("get active alerts: %v", err)
	} else {
		data.Alerts = alerts
		for _, a := range alerts {
			if a.IsUrgent() {
				data.UrgentAlerts = append(data.UrgentAlerts, a)
			}
		}
	}

	// Get today's fire danger rating
	if fdr, err := s.store.GetTodayFireDanger(s.loc); err == nil {
		data.FireDanger = fdr
	}

	return data, nil
}

func avg(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func median(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := s.getCurrentData()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get current weather condition and time of day for palette
	now := time.Now().In(s.loc)
	tod := forecast.GetTimeOfDay(now)
	condition := s.getCurrentCondition()

	// Check for override query param: ?weather=storm_night
	if override := r.URL.Query().Get("weather"); override != "" {
		if overrideCond, overrideTod, ok := parseWeatherOverride(override); ok {
			condition = overrideCond
			tod = overrideTod
		} else {
			// Just condition, keep current time of day
			condition = overrideCond
		}
	}

	palette := forecast.GetPalette(condition, tod)

	indexData := IndexData{
		CurrentData:     data,
		Palette:         palette,
		WeatherOverride: r.URL.Query().Get("weather"),
	}

	s.tmpl.ExecuteTemplate(w, "index.html", indexData)
}

func (s *Server) handleCurrentPartial(w http.ResponseWriter, r *http.Request) {
	data, err := s.getCurrentData()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.tmpl.ExecuteTemplate(w, "current.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

type ChartData struct {
	Labels []string    `json:"labels"`
	Series []ChartSeries `json:"series"`
}

type ChartSeries struct {
	Name  string    `json:"name"`
	Data  []float64 `json:"data"`
	Color string    `json:"color"`
}

func (s *Server) handleChartPartial(w http.ResponseWriter, r *http.Request) {
	end := time.Now()
	start := end.Add(-24 * time.Hour)

	stations, _ := s.store.GetActiveStations()
	colors := []string{"#4fc3f7", "#81c784", "#ffb74d", "#f48fb1"}

	chartData := ChartData{
		Labels: make([]string, 0),
		Series: make([]ChartSeries, 0),
	}

	for i, st := range stations {
		obs, _ := s.store.GetObservations(st.StationID, start, end)
		series := ChartSeries{
			Name:  fmt.Sprintf("%s (%.0fm)", st.Name, st.Elevation),
			Data:  make([]float64, 0),
			Color: colors[i%len(colors)],
		}

		for _, o := range obs {
			if o.Temp.Valid {
				if i == 0 {
					chartData.Labels = append(chartData.Labels, o.ObservedAt.In(s.loc).Format("3:04 PM"))
				}
				series.Data = append(series.Data, o.Temp.Float64)
			}
		}
		chartData.Series = append(chartData.Series, series)
	}

	s.tmpl.ExecuteTemplate(w, "chart.html", chartData)
}

func (s *Server) handleAPICurrent(w http.ResponseWriter, r *http.Request) {
	data, err := s.getCurrentData()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (s *Server) handleAPIHistory(w http.ResponseWriter, r *http.Request) {
	stationID := r.URL.Query().Get("station")
	if stationID == "" {
		stationID = "IWANDI23"
	}

	hours := 24
	end := time.Now()
	start := end.Add(-time.Duration(hours) * time.Hour)

	observations, err := s.store.GetObservations(stationID, start, end)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(observations)
}

func (s *Server) handleAPIStations(w http.ResponseWriter, r *http.Request) {
	stations, err := s.store.GetActiveStations()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stations)
}

type ForecastData struct {
	Days     []ForecastDay
	WUStats  *models.VerificationStats
	BOMStats *models.VerificationStats
	HasStats bool
}

type ForecastDay struct {
	Date               time.Time
	DayName            string
	DateStr            string
	IsToday            bool
	WU                 *models.Forecast
	BOM                *models.Forecast
	WUCorrectedMax     *float64 `json:"wu_corrected_max,omitempty"`
	WUCorrectedMin     *float64 `json:"wu_corrected_min,omitempty"`
	BOMCorrectedMax    *float64 `json:"bom_corrected_max,omitempty"`
	BOMCorrectedMin    *float64 `json:"bom_corrected_min,omitempty"`
	GeneratedNarrative string   `json:"generated_narrative"`
}

func (s *Server) getForecastData() (*ForecastData, error) {
	forecasts, err := s.store.GetLatestForecasts()
	if err != nil {
		return nil, err
	}

	stats, err := s.store.GetVerificationStats()
	if err != nil {
		log.Printf("get verification stats: %v", err)
	}

	correctionStats, err := s.store.GetAllCorrectionStats()
	if err != nil {
		log.Printf("get correction stats: %v", err)
	}

	loc := s.loc
	today := time.Now().In(loc)
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)

	dayMap := make(map[string]*ForecastDay)
	
	for _, fc := range forecasts["wu"] {
		key := fc.ValidDate.Format("2006-01-02")
		if dayMap[key] == nil {
			dayMap[key] = &ForecastDay{
				Date:    fc.ValidDate,
				DayName: fc.ValidDate.Weekday().String()[:3],
				DateStr: fc.ValidDate.Format("Jan 2"),
				IsToday: fc.ValidDate.Equal(todayDate),
			}
		}
		f := fc
		dayMap[key].WU = &f

		if fc.TempMax.Valid {
			if bias := getCorrectionBias(correctionStats, "wu", "tmax", fc.DayOfForecast); bias != 0 {
				corrected := fc.TempMax.Float64 - bias
				dayMap[key].WUCorrectedMax = &corrected
			}
		}
		if fc.TempMin.Valid {
			if bias := getCorrectionBias(correctionStats, "wu", "tmin", fc.DayOfForecast); bias != 0 {
				corrected := fc.TempMin.Float64 - bias
				dayMap[key].WUCorrectedMin = &corrected
			}
		}
	}

	for _, fc := range forecasts["bom"] {
		key := fc.ValidDate.Format("2006-01-02")
		if dayMap[key] == nil {
			dayMap[key] = &ForecastDay{
				Date:    fc.ValidDate,
				DayName: fc.ValidDate.Weekday().String()[:3],
				DateStr: fc.ValidDate.Format("Jan 2"),
				IsToday: fc.ValidDate.Equal(todayDate),
			}
		}
		f := fc
		dayMap[key].BOM = &f

		if fc.TempMax.Valid {
			if bias := getCorrectionBias(correctionStats, "bom", "tmax", fc.DayOfForecast); bias != 0 {
				corrected := fc.TempMax.Float64 - bias
				dayMap[key].BOMCorrectedMax = &corrected
			}
		}
		if fc.TempMin.Valid {
			if bias := getCorrectionBias(correctionStats, "bom", "tmin", fc.DayOfForecast); bias != 0 {
				corrected := fc.TempMin.Float64 - bias
				dayMap[key].BOMCorrectedMin = &corrected
			}
		}
	}

	stations, _ := s.store.GetActiveStations()
	var primaryStationID string
	for _, st := range stations {
		if st.IsPrimary {
			primaryStationID = st.StationID
			break
		}
	}

	nowcaster := forecast.NewNowcaster(s.store, s.loc)
	biasCorrector := forecast.NewBiasCorrector(s.store)

	var days []ForecastDay
	for i := 0; i < 5; i++ {
		date := todayDate.AddDate(0, 0, i)
		key := date.Format("2006-01-02")
		if day, ok := dayMap[key]; ok {
			if day.IsToday && day.WU != nil && day.WU.TempMax.Valid && primaryStationID != "" {
				biasMax := biasCorrector.GetCorrection("wu", "tmax", 0)
				nowcast, err := nowcaster.ComputeNowcast(primaryStationID, day.WU.TempMax.Float64, biasMax)
				if err == nil && nowcast != nil {
					corrected := nowcast.CorrectedMax
					day.WUCorrectedMax = &corrected
				}
			}
			day.GeneratedNarrative = buildGeneratedNarrative(day)
			days = append(days, *day)
		}
	}

	data := &ForecastData{Days: days}
	if wuStats, ok := stats["wu"]; ok {
		data.WUStats = &wuStats
		data.HasStats = true
	}
	if bomStats, ok := stats["bom"]; ok {
		data.BOMStats = &bomStats
		data.HasStats = true
	}

	return data, nil
}

const minBiasSamples = 7

// BiasResult contains the bias correction and metadata about how it was determined
type BiasResult struct {
	Bias       float64
	DayUsed    int  // which day's stats were used (-1 if none)
	Samples    int  // sample size the bias is based on
	IsFallback bool // true if a fallback day was used
}

func getCorrectionBiasWithFallback(stats map[string]map[string]map[int]*store.CorrectionStats, source, target string, dayOfForecast int) BiasResult {
	if stats == nil || stats[source] == nil || stats[source][target] == nil {
		return BiasResult{DayUsed: -1}
	}

	targetStats := stats[source][target]

	// First, try the exact day
	if s := targetStats[dayOfForecast]; s != nil && s.SampleSize >= minBiasSamples {
		bias := s.MeanBias
		if bias > forecast.MaxBiasCorrection {
			bias = forecast.MaxBiasCorrection
		} else if bias < -forecast.MaxBiasCorrection {
			bias = -forecast.MaxBiasCorrection
		}
		return BiasResult{
			Bias:       bias,
			DayUsed:    dayOfForecast,
			Samples:    s.SampleSize,
			IsFallback: false,
		}
	}

	// Fallback: find the nearest day with sufficient samples
	// Search nearby days (prefer closer days, then lower days on tie)
	searchOrder := []int{}
	for delta := 1; delta <= 14; delta++ {
		// Try lower day first (prefer earlier lead times on tie)
		if dayOfForecast-delta >= 0 {
			searchOrder = append(searchOrder, dayOfForecast-delta)
		}
		if dayOfForecast+delta <= 14 {
			searchOrder = append(searchOrder, dayOfForecast+delta)
		}
	}

	for _, day := range searchOrder {
		if s := targetStats[day]; s != nil && s.SampleSize >= minBiasSamples {
			bias := s.MeanBias
			if bias > forecast.MaxBiasCorrection {
				bias = forecast.MaxBiasCorrection
			} else if bias < -forecast.MaxBiasCorrection {
				bias = -forecast.MaxBiasCorrection
			}
			return BiasResult{
				Bias:       bias,
				DayUsed:    day,
				Samples:    s.SampleSize,
				IsFallback: true,
			}
		}
	}

	return BiasResult{DayUsed: -1}
}

// getCorrectionBias is kept for backward compatibility with other parts of the code
func getCorrectionBias(stats map[string]map[string]map[int]*store.CorrectionStats, source, target string, dayOfForecast int) float64 {
	result := getCorrectionBiasWithFallback(stats, source, target, dayOfForecast)
	return result.Bias
}

func (s *Server) handleForecastPartial(w http.ResponseWriter, r *http.Request) {
	data, err := s.getForecastData()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.tmpl.ExecuteTemplate(w, "forecast.html", data)
}

func (s *Server) handleAPIForecast(w http.ResponseWriter, r *http.Request) {
	data, err := s.getForecastData()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

type AccuracyData struct {
	WUStats        *models.VerificationStats
	BOMStats       *models.VerificationStats
	CorrectedStats *store.CorrectedAccuracyStats
	UniqueDays     int
	History        []VerificationRow
	ChartLabels    []string
	ChartWUMax     []float64
	ChartWUMin     []float64
	ChartBOMMax    []float64
	ChartBOMMin    []float64
	LeadTimeData   []LeadTimeRow
	RegimeStats    []RegimeRow
}

type VerificationRow struct {
	Date         string
	Source       string
	ForecastMax  float64
	ForecastMin  float64
	ActualMax    float64
	ActualMin    float64
	BiasMax      float64
	MaxBiasClass string
	Regime       string
	RegimeBadge  string
}

type RegimeRow struct {
	Regime    string
	Label     string
	Badge     string
	Color     string
	WUMAEMax  float64
	WUMAEMin  float64
	BOMMAEMax float64
	BOMMAEMin float64
	WUDays    int
	BOMDays   int
}

type LeadTimeRow struct {
	LeadTime   int
	WUMAEMax   float64
	WUMAEMin   float64
	BOMMAEMax  float64
	BOMMAEMin  float64
	WUDays     int
	BOMDays    int
}

func biasClass(bias float64) string {
	abs := bias
	if abs < 0 {
		abs = -abs
	}
	if abs <= 1 {
		return "good"
	}
	if abs <= 3 {
		return "ok"
	}
	return "bad"
}

func regimeBadge(regime string) string {
	switch regime {
	case "heatwave":
		return "🔥"
	case "inversion":
		return "🏔️"
	case "clear_calm":
		return "☀️"
	default:
		return ""
	}
}

func regimeLabel(regime string) string {
	switch regime {
	case "heatwave":
		return "Heatwave"
	case "inversion":
		return "Inversion"
	case "clear_calm":
		return "Clear/Calm"
	case "normal":
		return "Normal"
	default:
		return regime
	}
}

func regimeColor(regime string) string {
	switch regime {
	case "heatwave":
		return "#ff7043"
	case "inversion":
		return "#4fc3f7"
	case "clear_calm":
		return "#4ecdc4"
	default:
		return "#888"
	}
}

func (s *Server) handleAccuracy(w http.ResponseWriter, r *http.Request) {
	data := &AccuracyData{}

	// Get day-1 stats for WU, day-2 for BOM (BOM doesn't have reliable day-1 data)
	day1Stats, err := s.store.GetDay1VerificationStats()
	if err != nil {
		log.Printf("get day1 verification stats: %v", err)
	}
	if wuStats, ok := day1Stats["wu"]; ok {
		data.WUStats = &wuStats
	}
	// For BOM, use day-2 stats since they don't reliably have day-1 forecasts before cutoff
	biasStats, err := s.store.GetBiasStatsFromVerification(30)
	if err != nil {
		log.Printf("get bias stats: %v", err)
	}
	for _, b := range biasStats {
		if b.Source == "bom" && b.DayOfForecast == 2 && b.CountMax > 0 {
			data.BOMStats = &models.VerificationStats{
				Count:        b.CountMax,
				AvgMaxBias:   sql.NullFloat64{Float64: b.AvgBiasMax, Valid: true},
				AvgMinBias:   sql.NullFloat64{Float64: b.AvgBiasMin, Valid: true},
				MAEMax:       sql.NullFloat64{Float64: b.MAEMax, Valid: true},
				MAEMin:       sql.NullFloat64{Float64: b.MAEMin, Valid: true},
			}
			break
		}
	}

	// Get corrected forecast accuracy stats
	primaryStation, _ := s.store.GetPrimaryStation()
	if primaryStation != nil {
		if corrStats, err := s.store.GetCorrectedAccuracyStats(primaryStation.StationID, 30); err != nil {
			log.Printf("get corrected accuracy stats: %v", err)
		} else if corrStats.Count > 0 {
			data.CorrectedStats = corrStats
		}
	}

	// Get best-lead history with regime data for chart and table (WU D+1, BOM D+2)
	history, err := s.store.GetBestLeadVerificationWithRegime(30)
	if err != nil {
		log.Printf("get verification history with regime: %v", err)
	}

	// Build history rows and chart data
	// Group by date for chart (one label per date, separate series for WU/BOM)
	type chartPoint struct {
		wuMax, wuMin, bomMax, bomMin float64
		hasWU, hasBOM                bool
	}
	chartData := make(map[string]*chartPoint)
	var dates []string
	seenDates := make(map[string]bool)

	for _, h := range history {
		dateStr := h.ValidDate.Format("Jan 2")
		if !seenDates[dateStr] {
			seenDates[dateStr] = true
			dates = append(dates, dateStr)
			chartData[dateStr] = &chartPoint{}
		}

		// Build table row
		row := VerificationRow{
			Date:   dateStr,
			Source: h.Source,
			Regime: h.Regime,
			RegimeBadge: regimeBadge(h.Regime),
		}
		if h.ForecastTempMax.Valid {
			row.ForecastMax = h.ForecastTempMax.Float64
		}
		if h.ForecastTempMin.Valid {
			row.ForecastMin = h.ForecastTempMin.Float64
		}
		if h.ActualTempMax.Valid {
			row.ActualMax = h.ActualTempMax.Float64
		}
		if h.ActualTempMin.Valid {
			row.ActualMin = h.ActualTempMin.Float64
		}
		if h.BiasTempMax.Valid {
			row.BiasMax = h.BiasTempMax.Float64
			row.MaxBiasClass = biasClass(h.BiasTempMax.Float64)
		}
		data.History = append(data.History, row)

		// Build chart data
		pt := chartData[dateStr]
		if h.Source == "wu" {
			pt.hasWU = true
			if h.BiasTempMax.Valid {
				pt.wuMax = h.BiasTempMax.Float64
			}
			if h.BiasTempMin.Valid {
				pt.wuMin = h.BiasTempMin.Float64
			}
		} else if h.Source == "bom" {
			pt.hasBOM = true
			if h.BiasTempMax.Valid {
				pt.bomMax = h.BiasTempMax.Float64
			}
			if h.BiasTempMin.Valid {
				pt.bomMin = h.BiasTempMin.Float64
			}
		}
	}

	// Reverse dates for chronological chart order
	for i := len(dates) - 1; i >= 0; i-- {
		dateStr := dates[i]
		pt := chartData[dateStr]
		data.ChartLabels = append(data.ChartLabels, dateStr)
		data.ChartWUMax = append(data.ChartWUMax, pt.wuMax)
		data.ChartWUMin = append(data.ChartWUMin, pt.wuMin)
		data.ChartBOMMax = append(data.ChartBOMMax, pt.bomMax)
		data.ChartBOMMin = append(data.ChartBOMMin, pt.bomMin)
	}
	data.UniqueDays = len(dates)

	// Get lead time breakdown (reuse biasStats from earlier)
	leadMap := make(map[int]*LeadTimeRow)
	for _, b := range biasStats {
		if _, ok := leadMap[b.DayOfForecast]; !ok {
			leadMap[b.DayOfForecast] = &LeadTimeRow{LeadTime: b.DayOfForecast}
		}
		lt := leadMap[b.DayOfForecast]
		if b.Source == "wu" {
			lt.WUMAEMax = b.MAEMax
			lt.WUMAEMin = b.MAEMin
			lt.WUDays = b.CountMax
		} else if b.Source == "bom" {
			lt.BOMMAEMax = b.MAEMax
			lt.BOMMAEMin = b.MAEMin
			lt.BOMDays = b.CountMax
		}
	}
	for i := 1; i <= 5; i++ {
		if lt, ok := leadMap[i]; ok {
			data.LeadTimeData = append(data.LeadTimeData, *lt)
		}
	}

	// Get regime-based accuracy stats
	regimeStats, err := s.store.GetRegimeVerificationStats(30)
	if err != nil {
		log.Printf("get regime verification stats: %v", err)
	}
	for _, rs := range regimeStats {
		data.RegimeStats = append(data.RegimeStats, RegimeRow{
			Regime:    rs.Regime,
			Label:     regimeLabel(rs.Regime),
			Badge:     regimeBadge(rs.Regime),
			Color:     regimeColor(rs.Regime),
			WUMAEMax:  rs.WUMAEMax,
			WUMAEMin:  rs.WUMAEMin,
			BOMMAEMax: rs.BOMMAEMax,
			BOMMAEMin: rs.BOMMAEMin,
			WUDays:    rs.WUDays,
			BOMDays:   rs.BOMDays,
		})
	}

	s.tmpl.ExecuteTemplate(w, "accuracy.html", data)
}

type HealthStatus struct {
	Status   string         `json:"status"`
	Stations []StationHealth `json:"stations"`
	Errors   []string       `json:"errors,omitempty"`
}

type StationHealth struct {
	StationID   string    `json:"station_id"`
	LastSeen    time.Time `json:"last_seen"`
	AgeMinutes  int       `json:"age_minutes"`
	Stale       bool      `json:"stale"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	stations, err := s.store.GetActiveStations()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "error": err.Error()})
		return
	}

	health := HealthStatus{
		Status:   "ok",
		Stations: make([]StationHealth, 0, len(stations)),
	}

	staleThreshold := 60 * time.Minute
	now := time.Now()

	for _, st := range stations {
		obs, err := s.store.GetLatestObservation(st.StationID)
		if err != nil {
			health.Errors = append(health.Errors, st.StationID+": "+err.Error())
			continue
		}

		sh := StationHealth{StationID: st.StationID}
		if obs != nil {
			sh.LastSeen = obs.ObservedAt
			sh.AgeMinutes = int(now.Sub(obs.ObservedAt).Minutes())
			sh.Stale = now.Sub(obs.ObservedAt) > staleThreshold
		} else {
			sh.Stale = true
			sh.AgeMinutes = -1
		}

		if sh.Stale {
			health.Status = "degraded"
		}
		health.Stations = append(health.Stations, sh)
	}

	if len(health.Errors) > 0 {
		health.Status = "error"
	}

	w.Header().Set("Content-Type", "application/json")
	if health.Status != "ok" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	if err := json.NewEncoder(w).Encode(health); err != nil {
		log.Printf("health: write response: %v", err)
	}
}

// extractCondition extracts the weather condition from a WU narrative,
// stripping out temperature information.
func extractCondition(narrative string) string {
	s := strings.TrimSpace(narrative)
	if s == "" {
		return ""
	}

	parts := strings.Split(s, ".")
	var conditions []string
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t == "" {
			continue
		}
		lower := strings.ToLower(t)
		if strings.Contains(lower, "highs") || strings.Contains(lower, "lows") ||
			strings.Contains(lower, "°c") || strings.Contains(lower, "degrees") {
			continue
		}
		conditions = append(conditions, t)
	}

	if len(conditions) == 0 {
		return ""
	}
	return strings.Join(conditions, ". ")
}

// chooseCondition picks the best condition text from available forecasts.
// Prefers WU when it mentions storms/thunder (more specific), otherwise BOM.
func chooseCondition(day *ForecastDay) string {
	var wuCond, bomCond string

	if day.WU != nil && day.WU.Narrative.Valid {
		wuCond = extractCondition(day.WU.Narrative.String)
	}
	if day.BOM != nil && day.BOM.Narrative.Valid {
		bomCond = strings.TrimSpace(day.BOM.Narrative.String)
		bomCond = strings.TrimRight(bomCond, ".")
	}

	// Prefer WU if it mentions storms/thunder (more detailed)
	if wuCond != "" {
		lower := strings.ToLower(wuCond)
		if strings.Contains(lower, "storm") || strings.Contains(lower, "thunder") {
			return wuCond
		}
	}

	// Otherwise prefer BOM (cleaner condition-only text)
	if bomCond != "" {
		return bomCond
	}

	return wuCond
}

// chooseTemps returns the best available temps, preferring corrected values.
func chooseTemps(day *ForecastDay) (hi, lo float64, haveHi, haveLo bool) {
	// Max: prefer corrected WU, then corrected BOM, then raw
	if day.WUCorrectedMax != nil {
		hi, haveHi = *day.WUCorrectedMax, true
	} else if day.BOMCorrectedMax != nil {
		hi, haveHi = *day.BOMCorrectedMax, true
	} else if day.WU != nil && day.WU.TempMax.Valid {
		hi, haveHi = day.WU.TempMax.Float64, true
	} else if day.BOM != nil && day.BOM.TempMax.Valid {
		hi, haveHi = day.BOM.TempMax.Float64, true
	}

	// Min: prefer corrected WU, then corrected BOM, then raw
	if day.WUCorrectedMin != nil {
		lo, haveLo = *day.WUCorrectedMin, true
	} else if day.BOMCorrectedMin != nil {
		lo, haveLo = *day.BOMCorrectedMin, true
	} else if day.WU != nil && day.WU.TempMin.Valid {
		lo, haveLo = day.WU.TempMin.Float64, true
	} else if day.BOM != nil && day.BOM.TempMin.Valid {
		lo, haveLo = day.BOM.TempMin.Float64, true
	}

	return
}

// buildGeneratedNarrative creates a clean narrative with corrected temps.
func buildGeneratedNarrative(day *ForecastDay) string {
	cond := chooseCondition(day)
	hi, lo, haveHi, haveLo := chooseTemps(day)

	var parts []string

	if cond != "" {
		parts = append(parts, cond+".")
	}

	// Build temp phrase
	switch {
	case haveHi && haveLo:
		parts = append(parts, fmt.Sprintf("High %d°C, low %d°C.", int(math.Round(hi)), int(math.Round(lo))))
	case haveHi:
		parts = append(parts, fmt.Sprintf("High %d°C.", int(math.Round(hi))))
	case haveLo:
		parts = append(parts, fmt.Sprintf("Low %d°C.", int(math.Round(lo))))
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, " ")
}

// handleWeatherImage serves a weather-appropriate header image.
// It checks cache first, generates on-demand if needed, and returns a placeholder while generating.
// Supports ?weather=condition_time override for testing (e.g., ?weather=storm_night).
func (s *Server) handleWeatherImage(w http.ResponseWriter, r *http.Request) {
	// Get current weather condition and time of day
	loc := s.loc
	now := time.Now().In(loc)
	tod := forecast.GetTimeOfDay(now)
	baseCondition := s.getCurrentCondition()
	hasOverride := false

	// Check for override query param
	if override := r.URL.Query().Get("weather"); override != "" {
		hasOverride = true
		if overrideCond, overrideTod, ok := parseWeatherOverride(override); ok {
			baseCondition = overrideCond
			tod = overrideTod
		} else {
			baseCondition = overrideCond
		}
	}

	condition := forecast.ConditionWithTime(baseCondition, tod)

	// Try cache first
	if data, ok := s.imageCache.Get(condition); ok {
		s.serveBannerImage(w, data)
		return
	}

	// Try any cached image as fallback (but not when testing with override)
	if !hasOverride {
		if data, ok := s.imageCache.GetAny(); ok {
			// Trigger async generation for the correct condition
			go s.generateAndCache(baseCondition, tod, now)
			s.serveBannerImage(w, data)
			return
		}
	}

	// No cache - if we can generate, do it synchronously
	if s.imageGen != nil {
		s.genMu.Lock()
		defer s.genMu.Unlock()

		// Double-check cache after acquiring lock
		if data, ok := s.imageCache.Get(condition); ok {
			s.serveBannerImage(w, data)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()

		log.Printf("Generating first banner image for condition: %s", condition)
		data, err := s.imageGen.Generate(ctx, baseCondition, tod, now)
		if err != nil {
			log.Printf("Banner generation failed: %v", err)
			http.Error(w, "Image generation failed", http.StatusServiceUnavailable)
			return
		}

		if err := s.imageCache.Set(condition, data); err != nil {
			log.Printf("Failed to cache banner: %v", err)
		}

		s.serveBannerImage(w, data)
		return
	}

	// No generator and no cache - return 503
	log.Printf("weather-image: no generator and no cached images available")
	http.Error(w, "Weather image service unavailable", http.StatusServiceUnavailable)
}

func (s *Server) serveBannerImage(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(data)
}

func (s *Server) generateAndCache(baseCondition forecast.WeatherCondition, tod forecast.TimeOfDay, t time.Time) {
	if s.imageGen == nil {
		return
	}

	condition := forecast.ConditionWithTime(baseCondition, tod)

	s.genMu.Lock()
	defer s.genMu.Unlock()

	// Check if already cached
	if _, ok := s.imageCache.Get(condition); ok {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	log.Printf("Background generating banner for condition: %s", condition)
	data, err := s.imageGen.Generate(ctx, baseCondition, tod, t)
	if err != nil {
		log.Printf("Background banner generation failed: %v", err)
		return
	}

	if err := s.imageCache.Set(condition, data); err != nil {
		log.Printf("Failed to cache banner: %v", err)
	}
	log.Printf("Cached banner for condition: %s", condition)
}

// parseWeatherOverride parses a "condition_time" string (e.g., "storm_night")
// into separate condition and time-of-day values. Returns ok=false if not valid.
func parseWeatherOverride(override string) (condition forecast.WeatherCondition, tod forecast.TimeOfDay, ok bool) {
	if override == "" {
		return "", "", false
	}

	// Try to split on known time suffixes
	times := []forecast.TimeOfDay{forecast.TimeDawn, forecast.TimeDay, forecast.TimeDusk, forecast.TimeNight}
	for _, t := range times {
		suffix := "_" + string(t)
		if strings.HasSuffix(override, suffix) {
			cond := strings.TrimSuffix(override, suffix)
			return forecast.WeatherCondition(cond), t, true
		}
	}

	// No time suffix - treat whole string as condition
	return forecast.WeatherCondition(override), "", false
}

// getCurrentCondition extracts the weather condition from today's forecast.
func (s *Server) getCurrentCondition() forecast.WeatherCondition {
	loc := s.loc
	today := time.Now().In(loc)
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)

	forecasts, err := s.store.GetLatestForecasts()
	if err != nil {
		return forecast.ConditionClearCool // Default fallback
	}

	// Check WU forecasts first
	for _, fc := range forecasts["wu"] {
		if fc.ValidDate.Format("2006-01-02") == todayDate.Format("2006-01-02") {
			narrative := ""
			if fc.Narrative.Valid {
				narrative = fc.Narrative.String
			}
			tempMax := 20.0
			tempMin := 10.0
			if fc.TempMax.Valid {
				tempMax = fc.TempMax.Float64
			}
			if fc.TempMin.Valid {
				tempMin = fc.TempMin.Float64
			}
			return forecast.ExtractCondition(narrative, tempMax, tempMin)
		}
	}

	return forecast.ConditionClearCool
}
