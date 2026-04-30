package api

import (
	"log"
	"net/http"
	"net/url"

	"github.com/lox/wandiweather/internal/forecast"
)

type weatherImagePreviewsPageData struct {
	WeatherRows  []weatherImagePreviewRow
	SmokeRows    []weatherImagePreviewRow
	WeatherCount int
	SmokeCount   int
}

type weatherImagePreviewRow struct {
	Title       string
	Description string
	Cards       []weatherImagePreviewCard
}

type weatherImagePreviewCard struct {
	Title    string
	Detail   string
	Query    string
	ImageURL string
}

var previewConditions = []struct {
	condition   forecast.WeatherCondition
	description string
}{
	{condition: forecast.ConditionClearWarm, description: "Bright valley light, dry eucalyptus detail, and the cleanest sunny palette."},
	{condition: forecast.ConditionClearCool, description: "Cooler clear-air scenes with more restrained colour and sharper alpine contrast."},
	{condition: forecast.ConditionPartlyCloudy, description: "Broken cloud cover to test how the model balances sunlight and shade."},
	{condition: forecast.ConditionMostlyCloudy, description: "Soft, flattened light for checking cloud-heavy atmospheric scenes."},
	{condition: forecast.ConditionLightRain, description: "Wet foliage and gentle rain without the full drama of storm conditions."},
	{condition: forecast.ConditionHeavyRain, description: "Dark, saturated rain scenes with more forceful weather cues."},
	{condition: forecast.ConditionStorm, description: "Threatening skies and high-drama cloud structure across each light phase."},
	{condition: forecast.ConditionFog, description: "Low-visibility valley scenes where edge softness matters most."},
	{condition: forecast.ConditionHot, description: "Dry summer heat, pale grass, and harsher midday colour temperature."},
	{condition: forecast.ConditionFrost, description: "Cold mornings and blue-toned night scenes with crisp frost detail."},
}

var previewTimes = []struct {
	tod    forecast.TimeOfDay
	title  string
	detail string
}{
	{tod: forecast.TimeDawn, title: "Dawn", detail: "Soft horizon glow before full daylight."},
	{tod: forecast.TimeDay, title: "Day", detail: "Bright midday light and maximum terrain definition."},
	{tod: forecast.TimeDusk, title: "Dusk", detail: "Golden-hour warmth and longer shadows."},
	{tod: forecast.TimeNight, title: "Night", detail: "Moonlit landscapes and star-led contrast."},
}

var previewSmokeStudies = []struct {
	condition   forecast.WeatherCondition
	tod         forecast.TimeOfDay
	title       string
	description string
}{
	{condition: forecast.ConditionClearWarm, tod: forecast.TimeDay, title: "Clear Warm / Day", description: "Baseline daytime visibility makes smoke shifts easiest to compare."},
	{condition: forecast.ConditionMostlyCloudy, tod: forecast.TimeDusk, title: "Mostly Cloudy / Dusk", description: "Diffuse evening light shows how haze blends into softer skies."},
	{condition: forecast.ConditionStorm, tod: forecast.TimeNight, title: "Storm / Night", description: "Low-light stress test for dense smoke over already dramatic skies."},
}

var previewSmokeLevels = []struct {
	smoke  forecast.SmokeLevel
	title  string
	detail string
}{
	{smoke: forecast.SmokeClear, title: "Clear Air", detail: "No smoke prompt applied."},
	{smoke: forecast.SmokeHaze, title: "Light Haze", detail: "Subtle woodsmoke softening distant ridges."},
	{smoke: forecast.SmokeVisible, title: "Visible Smoke", detail: "Noticeable valley smoke and reduced contrast."},
	{smoke: forecast.SmokeDense, title: "Dense Smoke", detail: "Heavy smoke veil with the strongest visibility loss."},
}

func (s *Server) handleWeatherImagePreviews(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/weather-image-previews" {
		http.NotFound(w, r)
		return
	}

	weatherRows := buildWeatherPreviewRows()
	smokeRows := buildSmokePreviewRows()
	data := weatherImagePreviewsPageData{
		WeatherRows:  weatherRows,
		SmokeRows:    smokeRows,
		WeatherCount: len(previewConditions) * len(previewTimes),
		SmokeCount:   len(previewSmokeStudies) * len(previewSmokeLevels),
	}

	if err := s.tmpl.ExecuteTemplate(w, "weather-image-previews.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func buildWeatherPreviewRows() []weatherImagePreviewRow {
	rows := make([]weatherImagePreviewRow, 0, len(previewConditions))
	for _, previewCondition := range previewConditions {
		row := weatherImagePreviewRow{
			Title:       conditionToReadable(previewCondition.condition),
			Description: previewCondition.description,
			Cards:       make([]weatherImagePreviewCard, 0, len(previewTimes)),
		}

		for _, previewTime := range previewTimes {
			weather := string(forecast.ConditionWithTime(previewCondition.condition, previewTime.tod))
			row.Cards = append(row.Cards, weatherImagePreviewCard{
				Title:    previewTime.title,
				Detail:   previewTime.detail,
				Query:    previewImageQuery(weather, ""),
				ImageURL: previewImageURL(weather, ""),
			})
		}

		rows = append(rows, row)
	}
	return rows
}

func buildSmokePreviewRows() []weatherImagePreviewRow {
	rows := make([]weatherImagePreviewRow, 0, len(previewSmokeStudies))
	for _, study := range previewSmokeStudies {
		row := weatherImagePreviewRow{
			Title:       study.title,
			Description: study.description,
			Cards:       make([]weatherImagePreviewCard, 0, len(previewSmokeLevels)),
		}

		weather := string(forecast.ConditionWithTime(study.condition, study.tod))
		for _, previewSmoke := range previewSmokeLevels {
			row.Cards = append(row.Cards, weatherImagePreviewCard{
				Title:    previewSmoke.title,
				Detail:   previewSmoke.detail,
				Query:    previewImageQuery(weather, previewSmoke.smoke),
				ImageURL: previewImageURL(weather, previewSmoke.smoke),
			})
		}

		rows = append(rows, row)
	}
	return rows
}

func previewImageURL(weather string, smoke forecast.SmokeLevel) string {
	query := previewImageQuery(weather, smoke)
	if query == "" {
		return "/weather-image"
	}
	return "/weather-image?" + query
}

func previewImageQuery(weather string, smoke forecast.SmokeLevel) string {
	if weather == "" {
		return ""
	}

	query := "weather=" + url.QueryEscape(weather)
	if smoke != "" {
		query += "&smoke=" + url.QueryEscape(string(smoke))
	}
	return query
}
