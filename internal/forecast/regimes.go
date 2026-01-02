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
	if fc != nil && fc.TempMax.Valid && fc.TempMax.Float64 >= 32 {
		return true
	}

	for _, d := range prevDays {
		if d.TempMax.Valid && d.TempMax.Float64 >= 30 {
			return true
		}
	}

	if len(prevDays) >= 2 {
		if prevDays[0].TempMax.Valid && prevDays[1].TempMax.Valid {
			avg := (prevDays[0].TempMax.Float64 + prevDays[1].TempMax.Float64) / 2
			if avg >= 28 {
				return true
			}
		}
	}
	return false
}

func classifyClearCalm(summary *models.DailySummary) bool {
	if summary == nil {
		return false
	}
	return false
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
