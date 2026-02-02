package api

import (
	"database/sql"
	"log"
	"sort"
	"time"

	"github.com/lox/wandiweather/internal/forecast"
	"github.com/lox/wandiweather/internal/models"
)

// getCurrentData aggregates all current weather data for display.
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
			// Build input for shared temperature computation
			var currentTemp float64
			var hasCurrentTemp bool
			if data.Primary != nil && data.Primary.Temp.Valid {
				currentTemp = data.Primary.Temp.Float64
				hasCurrentTemp = true
			}

			var observedMax, observedMin float64
			var observedMaxValid, observedMinValid bool
			if data.TodayStats != nil {
				observedMax = data.TodayStats.MaxTemp
				observedMaxValid = observedMax > 0 || data.TodayStats.MinTemp > 0 // proxy for valid
				observedMin = data.TodayStats.MinTemp
				observedMinValid = true
			}

			tempInput := TodayTempInput{
				WUForecast:       wuForecast,
				BOMForecast:      bomForecast,
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
				Hour:             now.Hour(),
				TempFalling:      data.TempChangeRate != nil && *data.TempChangeRate < -0.5,
				LogNowcast:       true, // Log nowcast for the main display
			}

			tempResult := computeTodayTemps(tempInput)

			tf := &TodayForecast{
				TempMax:           tempResult.TempMax,
				TempMin:           tempResult.TempMin,
				TempMaxRaw:        tempResult.TempMaxRaw,
				NowcastApplied:    tempResult.NowcastApplied,
				NowcastAdjustment: tempResult.NowcastAdjustment,
				Explanation:       tempResult.Explanation,
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
			if wuForecast != nil && bomForecast != nil {
				dayOfForecast := bomForecast.DayOfForecast
				df := models.DisplayedForecast{
					DisplayedAt:   time.Now().UTC(),
					ValidDate:     todayDate,
					DayOfForecast: dayOfForecast,
				}
				exp := tf.Explanation
				df.WUForecastID = sql.NullInt64{Int64: wuForecast.ID, Valid: wuForecast != nil}
				df.BOMForecastID = sql.NullInt64{Int64: bomForecast.ID, Valid: bomForecast != nil}
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

// avg calculates the average of a slice of floats.
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

// median calculates the median of a slice of floats.
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
