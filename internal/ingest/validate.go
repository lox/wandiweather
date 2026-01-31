package ingest

import (
	"encoding/json"

	"github.com/lox/wandiweather/internal/models"
)

const (
	FlagTempOutOfRange      = "temp_out_of_range"
	FlagHumidityInvalid     = "humidity_invalid"
	FlagWindDirInvalid      = "wind_dir_invalid"
	FlagWindSpeedUnlikely   = "wind_speed_unlikely"
	FlagPressureOutOfRange  = "pressure_out_of_range"
	FlagSolarNegative       = "solar_negative"
	FlagPrecipNegative      = "precip_negative"
)

func ValidateObservation(obs *models.Observation) []string {
	var flags []string

	if obs.Temp.Valid {
		if obs.Temp.Float64 < -10 || obs.Temp.Float64 > 50 {
			flags = append(flags, FlagTempOutOfRange)
		}
	}

	if obs.Humidity.Valid {
		if obs.Humidity.Int64 < 0 || obs.Humidity.Int64 > 100 {
			flags = append(flags, FlagHumidityInvalid)
		}
	}

	if obs.WindDir.Valid {
		if obs.WindDir.Int64 < 0 || obs.WindDir.Int64 > 360 {
			flags = append(flags, FlagWindDirInvalid)
		}
	}

	if obs.WindSpeed.Valid {
		if obs.WindSpeed.Float64 < 0 || obs.WindSpeed.Float64 > 200 {
			flags = append(flags, FlagWindSpeedUnlikely)
		}
	}

	if obs.Pressure.Valid {
		if obs.Pressure.Float64 < 900 || obs.Pressure.Float64 > 1100 {
			flags = append(flags, FlagPressureOutOfRange)
		}
	}

	if obs.SolarRadiation.Valid {
		if obs.SolarRadiation.Float64 < 0 {
			flags = append(flags, FlagSolarNegative)
		}
	}

	if obs.PrecipRate.Valid && obs.PrecipRate.Float64 < 0 {
		flags = append(flags, FlagPrecipNegative)
	}
	if obs.PrecipTotal.Valid && obs.PrecipTotal.Float64 < 0 {
		flags = append(flags, FlagPrecipNegative)
	}

	return flags
}

func QualityFlagsToJSON(flags []string) string {
	if len(flags) == 0 {
		return ""
	}
	b, _ := json.Marshal(flags)
	return string(b)
}
