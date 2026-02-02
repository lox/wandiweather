package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/lox/wandiweather/internal/forecast"
	"github.com/lox/wandiweather/internal/models"
)

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

func (s *Server) handleForecastPartial(w http.ResponseWriter, r *http.Request) {
	data, err := s.getForecastData()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.tmpl.ExecuteTemplate(w, "forecast.html", data)
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
				Count:      b.CountMax,
				AvgMaxBias: sql.NullFloat64{Float64: b.AvgBiasMax, Valid: true},
				AvgMinBias: sql.NullFloat64{Float64: b.AvgBiasMin, Valid: true},
				MAEMax:     sql.NullFloat64{Float64: b.MAEMax, Valid: true},
				MAEMin:     sql.NullFloat64{Float64: b.MAEMin, Valid: true},
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
			Date:        dateStr,
			Source:      h.Source,
			Regime:      h.Regime,
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



func (s *Server) handleData(w http.ResponseWriter, r *http.Request) {
	data := DataPageData{
		UpdatedAt: time.Now().In(s.loc).Format("Jan 2, 3:04 PM"),
	}

	stats, err := s.store.GetDataHealthStats()
	if err != nil {
		log.Printf("get data health stats: %v", err)
	} else {
		data.SchemaVersion = stats.SchemaVersion
		data.TotalObservations = stats.TotalObservations
		data.TotalForecasts = stats.TotalForecasts
		data.RawPayloadCount = stats.RawPayloadCount
		data.RawPayloadSizeKB = stats.RawPayloadSizeKB
		data.DatabaseSizeMB = float64(stats.DatabaseSizeKB) / 1024.0
		data.ObsWithFlags = stats.ObsWithFlags
		data.CleanObservations = stats.CleanObservations
		data.ParseErrors24h = stats.ParseErrors24h
	}

	if health, err := s.store.GetIngestHealth(1); err != nil {
		log.Printf("get ingest health: %v", err)
	} else {
		data.IngestHealth = health
	}

	if obsTypes, err := s.store.GetObsTypeCounts(); err != nil {
		log.Printf("get obs types: %v", err)
	} else {
		data.ObsTypes = obsTypes
	}

	if coverage, err := s.store.GetForecastCoverage(); err != nil {
		log.Printf("get forecast coverage: %v", err)
	} else {
		data.ForecastCoverage = coverage
	}

	if errors, err := s.store.GetRecentIngestErrorsForDisplay(5); err != nil {
		log.Printf("get recent errors: %v", err)
	} else {
		data.RecentErrors = errors
	}

	s.tmpl.ExecuteTemplate(w, "data.html", data)
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

// Helper functions for accuracy page

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
