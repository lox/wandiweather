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

func (b *BOMClient) FetchForecasts() ([]models.Forecast, string, *FetchResult, error) {
	result := &FetchResult{}

	conn, err := ftp.Dial(bomFTPHost, ftp.DialWithTimeout(30*time.Second))
	if err != nil {
		result.Error = fmt.Errorf("ftp dial: %w", err)
		return nil, "", result, result.Error
	}
	defer conn.Quit()

	if err := conn.Login("anonymous", "anonymous"); err != nil {
		result.Error = fmt.Errorf("ftp login: %w", err)
		return nil, "", result, result.Error
	}

	resp, err := conn.Retr(bomForecastFile)
	if err != nil {
		result.Error = fmt.Errorf("ftp retr: %w", err)
		return nil, "", result, result.Error
	}
	defer resp.Close()

	body, err := io.ReadAll(resp)
	if err != nil {
		result.Error = fmt.Errorf("read body: %w", err)
		return nil, "", result, result.Error
	}
	result.ResponseSize = len(body)
	result.HTTPStatus = 200 // FTP success

	var product bomProduct
	if err := xml.Unmarshal(body, &product); err != nil {
		result.Error = fmt.Errorf("unmarshal xml: %w", err)
		return nil, string(body), result, result.Error
	}

	var targetArea *bomArea
	for i := range product.Forecast.Areas {
		if product.Forecast.Areas[i].AAC == b.areaCode && product.Forecast.Areas[i].Type == "location" {
			targetArea = &product.Forecast.Areas[i]
			break
		}
	}
	if targetArea == nil {
		result.Error = fmt.Errorf("area %s not found in forecast", b.areaCode)
		return nil, string(body), result, result.Error
	}

	fetchedAt := time.Now().UTC()
	var forecasts []models.Forecast
	var parseErrors []string

	mel, _ := time.LoadLocation("Australia/Melbourne")

	for _, period := range targetArea.Periods {
		startTime, err := time.Parse(time.RFC3339, period.StartTime)
		if err != nil {
			parseErrors = append(parseErrors, fmt.Sprintf("period[%d].StartTime=%q: %v", period.Index, period.StartTime, err))
			continue
		}
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

	result.RecordCount = len(forecasts)
	if len(parseErrors) > 0 {
		result.ParseErrors = len(parseErrors)
		result.ParseError = fmt.Sprintf("%d parse errors: %v", len(parseErrors), parseErrors[0])
	}

	return forecasts, string(body), result, nil
}
