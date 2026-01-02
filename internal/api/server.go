package api

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/lox/wandiweather/internal/models"
	"github.com/lox/wandiweather/internal/store"
)

//go:embed templates/*
var templateFS embed.FS

type Server struct {
	store  *store.Store
	port   string
	tmpl   *template.Template
}

func NewServer(store *store.Store, port string) *Server {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/*.html"))
	return &Server{
		store: store,
		port:  port,
		tmpl:  tmpl,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/accuracy", s.handleAccuracy)
	mux.HandleFunc("/health", s.handleHealth)
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
	Primary       *models.Observation
	ValleyTemp    float64
	Stations      map[string]*models.Observation
	StationMeta   map[string]models.Station
	ValleyFloor   []StationReading
	MidSlope      []StationReading
	Upper         []StationReading
	Inversion     *InversionStatus
	TodayForecast *TodayForecast
	TodayStats    *TodayStats
	LastUpdated   time.Time
}

type TodayForecast struct {
	TempMax      float64
	TempMin      float64
	PrecipChance int64
	PrecipAmount float64
	Narrative    string
	HasPrecip    bool
}

type TodayStats struct {
	MinTemp   float64
	MaxTemp   float64
	RainTotal float64
	HasRain   bool
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
		LastUpdated: time.Now(),
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
		}

		reading := StationReading{Station: st, Obs: obs}
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

	loc, _ := time.LoadLocation("Australia/Melbourne")
	today := time.Now().In(loc)
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)

	minTemp, maxTemp, rainTotal, err := s.store.GetTodayStats("IWANDI23", today)
	if err == nil {
		ts := &TodayStats{}
		if minTemp.Valid {
			ts.MinTemp = minTemp.Float64
		}
		if maxTemp.Valid {
			ts.MaxTemp = maxTemp.Float64
		}
		if rainTotal.Valid && rainTotal.Float64 > 0 {
			ts.RainTotal = rainTotal.Float64
			ts.HasRain = true
		}
		data.TodayStats = ts
	}

	forecasts, err := s.store.GetLatestForecasts()
	if err == nil {
		correctionStats, _ := s.store.GetAllCorrectionStats()

		for _, fc := range forecasts["wu"] {
			fcDate := fc.ValidDate.Format("2006-01-02")
			todayStr := todayDate.Format("2006-01-02")
			if fcDate == todayStr {
				tf := &TodayForecast{}

				// Apply bias correction to temps and round to match narrative
				if fc.TempMax.Valid {
					tf.TempMax = fc.TempMax.Float64
					if bias := getCorrectionBias(correctionStats, "wu", "tmax", fc.DayOfForecast); bias != 0 {
						tf.TempMax = fc.TempMax.Float64 - bias
					}
					tf.TempMax = math.Round(tf.TempMax)
				}
				if fc.TempMin.Valid {
					tf.TempMin = fc.TempMin.Float64
					if bias := getCorrectionBias(correctionStats, "wu", "tmin", fc.DayOfForecast); bias != 0 {
						tf.TempMin = fc.TempMin.Float64 - bias
					}
					tf.TempMin = math.Round(tf.TempMin)
				}

				if fc.PrecipChance.Valid {
					tf.PrecipChance = fc.PrecipChance.Int64
					tf.HasPrecip = fc.PrecipChance.Int64 > 10
				}
				if fc.PrecipAmount.Valid {
					tf.PrecipAmount = fc.PrecipAmount.Float64
				}

				// Build a ForecastDay to generate narrative with corrected temps
				var bomForecast *models.Forecast
				for _, bfc := range forecasts["bom"] {
					if bfc.ValidDate.Format("2006-01-02") == todayStr {
						b := bfc
						bomForecast = &b
						break
					}
				}
				wuFc := fc
				day := &ForecastDay{WU: &wuFc, BOM: bomForecast}
				if fc.TempMax.Valid {
					day.WUCorrectedMax = &tf.TempMax
				}
				if fc.TempMin.Valid {
					day.WUCorrectedMin = &tf.TempMin
				}
				tf.Narrative = buildGeneratedNarrative(day)

				data.TodayForecast = tf
				break
			}
		}
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
	s.tmpl.ExecuteTemplate(w, "index.html", data)
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

	valleyObs, _ := s.store.GetObservations("IWANDI23", start, end)
	midObs, _ := s.store.GetObservations("IWANDI8", start, end)
	upperObs, _ := s.store.GetObservations("IVICTORI162", start, end)

	chartData := ChartData{
		Labels: make([]string, 0),
		Series: []ChartSeries{
			{Name: "Valley (117m)", Data: make([]float64, 0), Color: "#4fc3f7"},
			{Name: "Mid-slope (364m)", Data: make([]float64, 0), Color: "#81c784"},
			{Name: "Upper (400m)", Data: make([]float64, 0), Color: "#ffb74d"},
		},
	}

	for _, obs := range valleyObs {
		if obs.Temp.Valid {
			chartData.Labels = append(chartData.Labels, obs.ObservedAt.Format("15:04"))
			chartData.Series[0].Data = append(chartData.Series[0].Data, obs.Temp.Float64)
		}
	}
	for _, obs := range midObs {
		if obs.Temp.Valid {
			chartData.Series[1].Data = append(chartData.Series[1].Data, obs.Temp.Float64)
		}
	}
	for _, obs := range upperObs {
		if obs.Temp.Valid {
			chartData.Series[2].Data = append(chartData.Series[2].Data, obs.Temp.Float64)
		}
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

	loc, _ := time.LoadLocation("Australia/Melbourne")
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

	var days []ForecastDay
	for i := 0; i < 7; i++ {
		date := todayDate.AddDate(0, 0, i)
		key := date.Format("2006-01-02")
		if day, ok := dayMap[key]; ok {
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

func getCorrectionBias(stats map[string]map[string]map[int]*store.CorrectionStats, source, target string, dayOfForecast int) float64 {
	if stats == nil {
		return 0
	}
	if stats[source] == nil {
		return 0
	}
	if stats[source][target] == nil {
		return 0
	}
	if s := stats[source][target][dayOfForecast]; s != nil {
		if s.SampleSize < 5 {
			return 0
		}
		return s.MeanBias
	}
	return 0
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
	Source      string
	SourceName  string
	Stats       *models.VerificationStats
	History     []VerificationRow
	ChartLabels []string
	ChartMax    []float64
	ChartMin    []float64
}

type VerificationRow struct {
	models.ForecastVerification
	MaxBiasClass string
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

func (s *Server) handleAccuracy(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.GetVerificationStats()
	if err != nil {
		log.Printf("get verification stats: %v", err)
	}

	data := &AccuracyData{}

	source := "wu"
	sourceName := "Weather Underground"
	if wuStats, ok := stats["wu"]; ok {
		data.Stats = &wuStats
	} else if bomStats, ok := stats["bom"]; ok {
		data.Stats = &bomStats
		source = "bom"
		sourceName = "Bureau of Meteorology (Wangaratta)"
	}
	data.Source = source
	data.SourceName = sourceName

	history, err := s.store.GetVerificationHistory(source, 14)
	if err != nil {
		log.Printf("get history: %v", err)
	}
	for _, v := range history {
		row := VerificationRow{ForecastVerification: v}
		if v.BiasTempMax.Valid {
			row.MaxBiasClass = biasClass(v.BiasTempMax.Float64)
		}
		data.History = append(data.History, row)
	}

	for i := len(history) - 1; i >= 0; i-- {
		v := history[i]
		data.ChartLabels = append(data.ChartLabels, v.ValidDate.Format("Jan 2"))
		if v.BiasTempMax.Valid {
			data.ChartMax = append(data.ChartMax, v.BiasTempMax.Float64)
		} else {
			data.ChartMax = append(data.ChartMax, 0)
		}
		if v.BiasTempMin.Valid {
			data.ChartMin = append(data.ChartMin, v.BiasTempMin.Float64)
		} else {
			data.ChartMin = append(data.ChartMin, 0)
		}
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

	staleThreshold := 15 * time.Minute
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
	json.NewEncoder(w).Encode(health)
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
