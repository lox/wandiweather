package ingest

import (
	"database/sql"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/jlaffaye/ftp"
	"github.com/lox/wandiweather/internal/models"
)

const (
	bomFTPHost     = "ftp.bom.gov.au:21"
	bomForecastFile = "/anon/gen/fwo/IDV10753.xml"
	wangarattaAAC  = "VIC_PT075"
)

type BOMClient struct {
	areaCode string
}

func NewBOMClient(areaCode string) *BOMClient {
	if areaCode == "" {
		areaCode = wangarattaAAC
	}
	return &BOMClient{areaCode: areaCode}
}

type bomProduct struct {
	XMLName      xml.Name       `xml:"product"`
	AmocBulletin bomAmoc        `xml:"amoc"`
	Forecast     bomForecastDoc `xml:"forecast"`
}

type bomAmoc struct {
	IssueTime string `xml:"issue-time-utc"`
}

type bomForecastDoc struct {
	Areas []bomArea `xml:"area"`
}

type bomArea struct {
	AAC         string            `xml:"aac,attr"`
	Description string            `xml:"description,attr"`
	Type        string            `xml:"type,attr"`
	Periods     []bomForecastPeriod `xml:"forecast-period"`
}

type bomForecastPeriod struct {
	Index       int           `xml:"index,attr"`
	StartTime   string        `xml:"start-time-utc,attr"`
	EndTime     string        `xml:"end-time-utc,attr"`
	Elements    []bomElement  `xml:"element"`
	TextItems   []bomText     `xml:"text"`
}

type bomElement struct {
	Type  string `xml:"type,attr"`
	Units string `xml:"units,attr"`
	Value string `xml:",chardata"`
}

type bomText struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

func (b *BOMClient) FetchForecasts() ([]models.Forecast, string, error) {
	conn, err := ftp.Dial(bomFTPHost, ftp.DialWithTimeout(30*time.Second))
	if err != nil {
		return nil, "", fmt.Errorf("ftp dial: %w", err)
	}
	defer conn.Quit()

	if err := conn.Login("anonymous", "anonymous"); err != nil {
		return nil, "", fmt.Errorf("ftp login: %w", err)
	}

	resp, err := conn.Retr(bomForecastFile)
	if err != nil {
		return nil, "", fmt.Errorf("ftp retr: %w", err)
	}
	defer resp.Close()

	body, err := io.ReadAll(resp)
	if err != nil {
		return nil, "", fmt.Errorf("read body: %w", err)
	}

	var product bomProduct
	if err := xml.Unmarshal(body, &product); err != nil {
		return nil, "", fmt.Errorf("unmarshal xml: %w", err)
	}

	var targetArea *bomArea
	for i := range product.Forecast.Areas {
		if product.Forecast.Areas[i].AAC == b.areaCode && product.Forecast.Areas[i].Type == "location" {
			targetArea = &product.Forecast.Areas[i]
			break
		}
	}
	if targetArea == nil {
		return nil, "", fmt.Errorf("area %s not found in forecast", b.areaCode)
	}

	fetchedAt := time.Now().UTC()
	var forecasts []models.Forecast

	// BOM uses Australian Eastern time for forecast periods
	mel, _ := time.LoadLocation("Australia/Melbourne")

	for _, period := range targetArea.Periods {
		startTime, err := time.Parse(time.RFC3339, period.StartTime)
		if err != nil {
			continue
		}
		// Convert to local time first, then extract the date
		// BOM period start times are at midnight local time for the forecast day
		localStart := startTime.In(mel)
		validDate := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), 0, 0, 0, 0, time.UTC)

		fc := models.Forecast{
			Source:        "bom",
			FetchedAt:     fetchedAt,
			ValidDate:     validDate,
			DayOfForecast: period.Index,
			RawJSON:       "", // Don't store raw XML to save memory
		}

		for _, elem := range period.Elements {
			switch elem.Type {
			case "air_temperature_maximum":
				if v, err := strconv.ParseFloat(elem.Value, 64); err == nil {
					fc.TempMax = sql.NullFloat64{Float64: v, Valid: true}
				}
			case "air_temperature_minimum":
				if v, err := strconv.ParseFloat(elem.Value, 64); err == nil {
					fc.TempMin = sql.NullFloat64{Float64: v, Valid: true}
				}
			case "precipitation_range":
				fc.PrecipRange = sql.NullString{String: elem.Value, Valid: elem.Value != ""}
			}
		}

		for _, text := range period.TextItems {
			switch text.Type {
			case "precis":
				fc.Narrative = sql.NullString{String: text.Value, Valid: true}
			case "probability_of_precipitation":
				// Parse "20%" -> 20
				s := text.Value
				if len(s) > 0 && s[len(s)-1] == '%' {
					if v, err := strconv.Atoi(s[:len(s)-1]); err == nil {
						fc.PrecipChance = sql.NullInt64{Int64: int64(v), Valid: true}
					}
				}
			}
		}

		forecasts = append(forecasts, fc)
	}

	return forecasts, string(body), nil
}
