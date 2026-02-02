package api

import (
	"log"
	"math"

	"github.com/lox/wandiweather/internal/forecast"
	"github.com/lox/wandiweather/internal/models"
	"github.com/lox/wandiweather/internal/store"
)

// TodayTempInput contains all the inputs needed to compute today's display temperatures.
type TodayTempInput struct {
	WUForecast       *models.Forecast
	BOMForecast      *models.Forecast
	CorrectionStats  map[string]map[string]map[int]*store.CorrectionStats
	BiasCorrector    *forecast.BiasCorrector
	Nowcaster        *forecast.Nowcaster
	PrimaryStationID string
	CurrentTemp      float64
	HasCurrentTemp   bool
	ObservedMax      float64
	ObservedMaxValid bool
	ObservedMin      float64
	ObservedMinValid bool
	Hour             int
	TempFalling      bool // true if temp is falling > 0.5°C/hr
	LogNowcast       bool // whether to log nowcast to DB
}

// TodayTempResult contains the computed display temperatures and explanation.
type TodayTempResult struct {
	TempMax           float64
	TempMin           float64
	TempMaxRaw        float64
	NowcastApplied    bool
	NowcastAdjustment float64
	Explanation       ForecastExplanation
	HaveMax           bool
	HaveMin           bool
}

// computeTodayTemps calculates today's display temperatures using standardized logic:
// - Max temp: prefer BOM (with sanity checks), apply bias correction + nowcast, use observed as floor
// - Min temp: prefer WU, apply bias correction, use observed as ceiling
func computeTodayTemps(input TodayTempInput) TodayTempResult {
	result := TodayTempResult{}
	exp := &result.Explanation

	wuForecast := input.WUForecast
	bomForecast := input.BOMForecast

	// MAX TEMP: prefer BOM (better accuracy), but fall back to WU if BOM is unreasonable
	// "Unreasonable" = current temp already exceeds BOM forecast by >3°C, or BOM differs from WU by >10°C
	useBOMMax := bomForecast != nil && bomForecast.TempMax.Valid
	if useBOMMax && input.HasCurrentTemp && input.CurrentTemp > bomForecast.TempMax.Float64+3 {
		useBOMMax = false // Current temp already exceeds BOM forecast
	}
	if useBOMMax && wuForecast != nil && wuForecast.TempMax.Valid {
		if wuForecast.TempMax.Float64-bomForecast.TempMax.Float64 > 10 {
			useBOMMax = false // WU and BOM differ by more than 10°C, BOM likely wrong
		}
	}

	if useBOMMax {
		exp.MaxSource = "bom"
		exp.MaxRaw = bomForecast.TempMax.Float64
		result.TempMax = bomForecast.TempMax.Float64
		result.HaveMax = true

		biasResult := getCorrectionBiasWithFallback(input.CorrectionStats, "bom", "tmax", bomForecast.DayOfForecast)
		if biasResult.DayUsed >= 0 {
			exp.MaxBiasApplied = biasResult.Bias
			exp.MaxBiasDayUsed = biasResult.DayUsed
			exp.MaxBiasSamples = biasResult.Samples
			exp.MaxBiasFallback = biasResult.IsFallback
			result.TempMax = bomForecast.TempMax.Float64 - biasResult.Bias
		} else {
			exp.MaxBiasDayUsed = -1
		}
		result.TempMaxRaw = math.Round(result.TempMax)

		// Nowcast using BOM as base
		if bomForecast.DayOfForecast == 0 && input.PrimaryStationID != "" {
			biasMax := input.BiasCorrector.GetCorrection("bom", "tmax", 0)
			nowcast, err := input.Nowcaster.ComputeNowcast(input.PrimaryStationID, bomForecast.TempMax.Float64, biasMax)
			if err == nil && nowcast != nil {
				exp.MaxNowcast = nowcast.Adjustment
				result.TempMax = nowcast.CorrectedMax
				result.NowcastApplied = true
				result.NowcastAdjustment = nowcast.Adjustment
				if input.LogNowcast {
					if err := input.Nowcaster.LogNowcast(input.PrimaryStationID, bomForecast.TempMax.Float64, nowcast); err != nil {
						log.Printf("api: log nowcast: %v", err)
					}
				}
			}
		}
		result.TempMax = math.Round(result.TempMax)
		exp.MaxFinal = result.TempMax
	} else if wuForecast != nil && wuForecast.TempMax.Valid {
		// Fallback to WU if BOM unavailable
		exp.MaxSource = "wu"
		exp.MaxRaw = wuForecast.TempMax.Float64
		result.TempMax = wuForecast.TempMax.Float64
		result.HaveMax = true

		biasResult := getCorrectionBiasWithFallback(input.CorrectionStats, "wu", "tmax", wuForecast.DayOfForecast)
		if biasResult.DayUsed >= 0 {
			exp.MaxBiasApplied = biasResult.Bias
			exp.MaxBiasDayUsed = biasResult.DayUsed
			exp.MaxBiasSamples = biasResult.Samples
			exp.MaxBiasFallback = biasResult.IsFallback
			result.TempMax = wuForecast.TempMax.Float64 - biasResult.Bias
		} else {
			exp.MaxBiasDayUsed = -1
		}
		result.TempMaxRaw = math.Round(result.TempMax)
		result.TempMax = math.Round(result.TempMax)
		exp.MaxFinal = result.TempMax
	}

	// Use observed max as floor if it exceeds the corrected forecast
	if result.HaveMax && input.ObservedMaxValid && input.ObservedMax > result.TempMax {
		result.TempMax = math.Round(input.ObservedMax)
		exp.MaxFinal = result.TempMax
	}

	// After ~3 PM local time, if temp is falling, just use observed max
	// The day's max has likely already occurred
	if result.HaveMax && input.Hour >= 15 && input.TempFalling && input.ObservedMaxValid && input.ObservedMax > 0 {
		result.TempMax = math.Round(input.ObservedMax)
		exp.MaxFinal = result.TempMax
	}

	// Sanity check: if the corrected forecast exceeds both the raw forecast
	// AND the observed max by more than 3°C, the correction is likely wrong.
	if result.HaveMax && input.ObservedMaxValid {
		rawMax := exp.MaxRaw
		observedMax := input.ObservedMax
		correctedMax := result.TempMax
		if correctedMax > rawMax+3 && correctedMax > observedMax+3 {
			if observedMax > rawMax {
				result.TempMax = math.Round(observedMax)
			} else {
				result.TempMax = math.Round(rawMax)
			}
			exp.MaxFinal = result.TempMax
			exp.MaxBiasApplied = 0 // Mark that correction was rejected
		}
	}

	// MIN TEMP: prefer WU (better accuracy)
	if wuForecast != nil && wuForecast.TempMin.Valid {
		exp.MinSource = "wu"
		exp.MinRaw = wuForecast.TempMin.Float64
		result.TempMin = wuForecast.TempMin.Float64
		result.HaveMin = true

		biasResult := getCorrectionBiasWithFallback(input.CorrectionStats, "wu", "tmin", wuForecast.DayOfForecast)
		if biasResult.DayUsed >= 0 {
			exp.MinBiasApplied = biasResult.Bias
			exp.MinBiasDayUsed = biasResult.DayUsed
			exp.MinBiasSamples = biasResult.Samples
			exp.MinBiasFallback = biasResult.IsFallback
			result.TempMin = wuForecast.TempMin.Float64 - biasResult.Bias
		} else {
			exp.MinBiasDayUsed = -1
		}
		result.TempMin = math.Round(result.TempMin)
		exp.MinFinal = result.TempMin
	} else if bomForecast != nil && bomForecast.TempMin.Valid {
		// Fallback to BOM if WU unavailable
		exp.MinSource = "bom"
		exp.MinRaw = bomForecast.TempMin.Float64
		result.TempMin = bomForecast.TempMin.Float64
		result.HaveMin = true

		biasResult := getCorrectionBiasWithFallback(input.CorrectionStats, "bom", "tmin", bomForecast.DayOfForecast)
		if biasResult.DayUsed >= 0 {
			exp.MinBiasApplied = biasResult.Bias
			exp.MinBiasDayUsed = biasResult.DayUsed
			exp.MinBiasSamples = biasResult.Samples
			exp.MinBiasFallback = biasResult.IsFallback
			result.TempMin = bomForecast.TempMin.Float64 - biasResult.Bias
		} else {
			exp.MinBiasDayUsed = -1
		}
		result.TempMin = math.Round(result.TempMin)
		exp.MinFinal = result.TempMin
	}

	// Use observed min as ceiling (can't predict higher than what we've already seen)
	if result.HaveMin && input.ObservedMinValid && input.ObservedMin < result.TempMin {
		result.TempMin = math.Round(input.ObservedMin)
		exp.MinFinal = result.TempMin
	}

	return result
}
