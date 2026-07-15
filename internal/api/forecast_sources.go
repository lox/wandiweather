package api

import (
	"sort"

	"github.com/lox/wandiweather/internal/models"
)

const (
	legacyBOMForecastSource = "bom"
	liveBOMForecastSource   = "bom_daily_api"
)

func liveBOMForecasts(forecasts map[string][]models.Forecast) []models.Forecast {
	dailyAPI := forecasts[liveBOMForecastSource]
	if len(dailyAPI) == 0 {
		return forecasts[legacyBOMForecastSource]
	}

	result := append([]models.Forecast(nil), dailyAPI...)
	liveDates := make(map[string]struct{}, len(dailyAPI))
	for _, fc := range dailyAPI {
		liveDates[fc.ValidDate.Format("2006-01-02")] = struct{}{}
	}

	for _, fc := range forecasts[legacyBOMForecastSource] {
		if _, ok := liveDates[fc.ValidDate.Format("2006-01-02")]; ok {
			continue
		}
		result = append(result, fc)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ValidDate.Before(result[j].ValidDate)
	})
	return result
}

func bomCorrectionSource(f models.Forecast) string {
	if f.Source != "" {
		return f.Source
	}
	return legacyBOMForecastSource
}
