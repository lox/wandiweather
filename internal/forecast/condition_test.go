package forecast

import "testing"

func TestExtractCondition(t *testing.T) {
	tests := []struct {
		name      string
		narrative string
		tempMax   float64
		tempMin   float64
		want      WeatherCondition
	}{
		{
			name:      "hot day overrides narrative",
			narrative: "Partly cloudy",
			tempMax:   38,
			tempMin:   22,
			want:      ConditionHot,
		},
		{
			name:      "frost overrides narrative",
			narrative: "Clear skies",
			tempMax:   8,
			tempMin:   -2,
			want:      ConditionFrost,
		},
		{
			name:      "thunderstorm detection",
			narrative: "Thunderstorms developing in the afternoon",
			tempMax:   28,
			tempMin:   18,
			want:      ConditionStorm,
		},
		{
			name:      "storm detection",
			narrative: "Severe storms possible",
			tempMax:   30,
			tempMin:   20,
			want:      ConditionStorm,
		},
		{
			name:      "heavy rain",
			narrative: "Heavy rain expected",
			tempMax:   20,
			tempMin:   15,
			want:      ConditionHeavyRain,
		},
		{
			name:      "light rain - showers",
			narrative: "Scattered showers",
			tempMax:   22,
			tempMin:   14,
			want:      ConditionLightRain,
		},
		{
			name:      "light rain - drizzle",
			narrative: "Light drizzle in the morning",
			tempMax:   18,
			tempMin:   12,
			want:      ConditionLightRain,
		},
		{
			name:      "fog",
			narrative: "Morning fog clearing",
			tempMax:   20,
			tempMin:   10,
			want:      ConditionFog,
		},
		{
			name:      "mist",
			narrative: "Mist and low cloud",
			tempMax:   18,
			tempMin:   12,
			want:      ConditionFog,
		},
		{
			name:      "mostly cloudy",
			narrative: "Mostly cloudy with little sun",
			tempMax:   22,
			tempMin:   14,
			want:      ConditionMostlyCloudy,
		},
		{
			name:      "overcast",
			narrative: "Overcast skies",
			tempMax:   20,
			tempMin:   12,
			want:      ConditionMostlyCloudy,
		},
		{
			name:      "partly cloudy",
			narrative: "Partly cloudy",
			tempMax:   26,
			tempMin:   16,
			want:      ConditionPartlyCloudy,
		},
		{
			name:      "mix of sun and clouds",
			narrative: "A mix of sun and clouds",
			tempMax:   24,
			tempMin:   14,
			want:      ConditionPartlyCloudy,
		},
		{
			name:      "clear warm - sunny warm day",
			narrative: "Sunny and pleasant",
			tempMax:   28,
			tempMin:   18,
			want:      ConditionClearWarm,
		},
		{
			name:      "clear cool - sunny cool day",
			narrative: "Sunny",
			tempMax:   18,
			tempMin:   8,
			want:      ConditionClearCool,
		},
		{
			name:      "WU style narrative",
			narrative: "Partly cloudy. Highs 28 to 30°C and lows 12 to 14°C.",
			tempMax:   29,
			tempMin:   13,
			want:      ConditionPartlyCloudy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractCondition(tt.narrative, tt.tempMax, tt.tempMin)
			if got != tt.want {
				t.Errorf("ExtractCondition() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildPrompt(t *testing.T) {
	prompt := BuildPrompt(ConditionStorm)

	if prompt == "" {
		t.Error("BuildPrompt() returned empty string")
	}

	// Should contain base style
	if len(prompt) < 100 {
		t.Error("BuildPrompt() returned unexpectedly short prompt")
	}
}
