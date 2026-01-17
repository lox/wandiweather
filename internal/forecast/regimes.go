package forecast

import (
	"github.com/lox/wandiweather/internal/models"
)

type RegimeFlags struct {
	Heatwave       bool
	InversionNight bool
	ClearCalm      bool
}

func ClassifyRegime(
	forecast *models.Forecast,
	summary *models.DailySummary,
	prevDays []models.DailySummary,
) RegimeFlags {
	return RegimeFlags{
		Heatwave:       classifyHeatwave(forecast, prevDays),
		InversionNight: summary != nil && summary.InversionDetected.Valid && summary.InversionDetected.Bool,
		ClearCalm:      classifyClearCalm(summary),
	}
}

func classifyHeatwave(fc *models.Forecast, prevDays []models.DailySummary) bool {
	// Forecast ≥35°C triggers heatwave
	if fc != nil && fc.TempMax.Valid && fc.TempMax.Float64 >= 35 {
		return true
	}

	// Two consecutive days ≥32°C triggers heatwave
	if len(prevDays) >= 2 {
		if prevDays[0].TempMax.Valid && prevDays[0].TempMax.Float64 >= 32 &&
			prevDays[1].TempMax.Valid && prevDays[1].TempMax.Float64 >= 32 {
			return true
		}
	}
	return false
}

func classifyClearCalm(summary *models.DailySummary) bool {
	if summary == nil {
		return false
	}

	// Clear/calm regime indicates good radiative conditions:
	// - No/minimal precipitation (dry day)
	// - High solar radiation (clear skies)
	// - Calm overnight winds (no mixing)

	// Check for dry conditions (precip < 0.5mm)
	isDry := summary.PrecipTotal.Valid && summary.PrecipTotal.Float64 < 0.5

	// Check for high solar (> 10 MJ/m² indicates mostly clear day)
	// Based on observed range: 1-30 MJ, avg 13 MJ
	isHighSolar := summary.SolarIntegral.Valid && summary.SolarIntegral.Float64 > 10

	// Check for calm night (> 40% of observations below 1.5 m/s)
	// Based on observed range: 0-83%, avg 34%
	isCalmNight := summary.CalmFractionNight.Valid && summary.CalmFractionNight.Float64 > 0.4

	return isDry && isHighSolar && isCalmNight
}

func RegimeToString(flags RegimeFlags) string {
	if flags.Heatwave {
		return "heatwave"
	}
	if flags.InversionNight {
		return "inversion"
	}
	if flags.ClearCalm {
		return "clear_calm"
	}
	return "all"
}
