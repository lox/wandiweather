package api

import (
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/lox/wandiweather/internal/forecast"
)

// getForecastData assembles the multi-day forecast data.
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

	// Get today's observed stats and current temp for the shared helper
	var observedMax, observedMin float64
	var observedMaxValid, observedMinValid bool
	var currentTemp float64
	var hasCurrentTemp bool
	if primaryStationID != "" {
		if todayStats, err := s.store.GetTodayStatsExtended(primaryStationID, today); err == nil {
			if todayStats.MaxTemp.Valid {
				observedMax = todayStats.MaxTemp.Float64
				observedMaxValid = true
			}
			if todayStats.MinTemp.Valid {
				observedMin = todayStats.MinTemp.Float64
				observedMinValid = true
			}
		}
		// Get current temp from latest observation
		if obs, err := s.store.GetLatestObservation(primaryStationID); err == nil && obs != nil && obs.Temp.Valid {
			currentTemp = obs.Temp.Float64
			hasCurrentTemp = true
		}
	}

	var days []ForecastDay
	for i := 0; i < 5; i++ {
		date := todayDate.AddDate(0, 0, i)
		key := date.Format("2006-01-02")
		if day, ok := dayMap[key]; ok {
			if day.IsToday && primaryStationID != "" {
				// Use shared helper for consistent temperature computation
				tempInput := TodayTempInput{
					WUForecast:       day.WU,
					BOMForecast:      day.BOM,
					CorrectionStats:  correctionStats,
					BiasCorrector:    biasCorrector,
					Nowcaster:        nowcaster,
					PrimaryStationID: primaryStationID,
					CurrentTemp:      currentTemp,
					HasCurrentTemp:   hasCurrentTemp,
					ObservedMax:      observedMax,
					ObservedMaxValid: observedMaxValid,
					ObservedMin:      observedMin,
					ObservedMinValid: observedMinValid,
					Hour:             today.Hour(),
					TempFalling:      false, // We don't have temp change rate here, safer to not assume
					LogNowcast:       false, // Don't log again, main display already logged
				}

				tempResult := computeTodayTemps(tempInput)

				if tempResult.HaveMax {
					day.DisplayMax = &tempResult.TempMax
				}
				if tempResult.HaveMin {
					day.DisplayMin = &tempResult.TempMin
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
