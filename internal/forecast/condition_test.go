package forecast

import (
	"strings"
	"testing"
	"time"
)

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

func TestGetTimeOfDay(t *testing.T) {
	tests := []struct {
		name string
		hour int
		want TimeOfDay
	}{
		{"midnight", 0, TimeNight},
		{"early morning", 4, TimeNight},
		{"dawn start", 5, TimeDawn},
		{"dawn end", 6, TimeDawn},
		{"day start", 7, TimeDay},
		{"midday", 12, TimeDay},
		{"afternoon", 15, TimeDay},
		{"day end", 16, TimeDay},
		{"dusk start", 17, TimeDusk},
		{"dusk middle", 18, TimeDusk},
		{"dusk end", 19, TimeDusk},
		{"night start", 20, TimeNight},
		{"late night", 23, TimeNight},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testTime := time.Date(2025, 1, 15, tt.hour, 30, 0, 0, time.UTC)
			got := GetTimeOfDay(testTime)
			if got != tt.want {
				t.Errorf("GetTimeOfDay(%d:30) = %v, want %v", tt.hour, got, tt.want)
			}
		})
	}
}

func TestGetMoonPhase(t *testing.T) {
	tests := []struct {
		name string
		date time.Time
		want MoonPhase
	}{
		{
			name: "known new moon - Jan 6 2000",
			date: time.Date(2000, 1, 6, 18, 14, 0, 0, time.UTC),
			want: MoonNew,
		},
		{
			name: "approx full moon - Jan 21 2000 (15 days after new)",
			date: time.Date(2000, 1, 21, 18, 14, 0, 0, time.UTC),
			want: MoonFull,
		},
		{
			name: "waxing crescent - Jan 10 2000 (4 days after new)",
			date: time.Date(2000, 1, 10, 18, 14, 0, 0, time.UTC),
			want: MoonWaxingCrescent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetMoonPhase(tt.date)
			if got != tt.want {
				t.Errorf("GetMoonPhase() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMoonIllumination(t *testing.T) {
	newMoon := time.Date(2000, 1, 6, 18, 14, 0, 0, time.UTC)
	newIllum := MoonIllumination(newMoon)
	if newIllum > 5 {
		t.Errorf("New moon illumination = %d%%, want ~0%%", newIllum)
	}

	fullMoon := time.Date(2000, 1, 21, 18, 14, 0, 0, time.UTC)
	fullIllum := MoonIllumination(fullMoon)
	if fullIllum < 95 {
		t.Errorf("Full moon illumination = %d%%, want ~100%%", fullIllum)
	}
}

func TestMoonDescription(t *testing.T) {
	tests := []struct {
		phase    MoonPhase
		wantName string
	}{
		{MoonNew, "New Moon"},
		{MoonFull, "Full Moon"},
		{MoonFirstQuarter, "First Quarter"},
		{MoonLastQuarter, "Last Quarter"},
	}

	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			name, prompt := MoonDescription(tt.phase)
			if name != tt.wantName {
				t.Errorf("MoonDescription().name = %q, want %q", name, tt.wantName)
			}
			if prompt == "" {
				t.Error("MoonDescription().prompt should not be empty")
			}
		})
	}
}

func TestConditionWithTime(t *testing.T) {
	tests := []struct {
		condition WeatherCondition
		tod       TimeOfDay
		want      WeatherCondition
	}{
		{ConditionClearWarm, TimeDay, "clear_warm_day"},
		{ConditionStorm, TimeNight, "storm_night"},
		{ConditionFog, TimeDawn, "fog_dawn"},
	}

	for _, tt := range tests {
		t.Run(string(tt.want), func(t *testing.T) {
			got := ConditionWithTime(tt.condition, tt.tod)
			if got != tt.want {
				t.Errorf("ConditionWithTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildPromptWithTime(t *testing.T) {
	prompt := BuildPromptWithTime(ConditionClearWarm, TimeNight)
	if prompt == "" {
		t.Error("BuildPromptWithTime() returned empty string")
	}

	if len(prompt) < 200 {
		t.Error("BuildPromptWithTime() returned unexpectedly short prompt")
	}

	// Verify night prompt contains night-specific keywords
	if !strings.Contains(prompt, "NIGHT") {
		t.Error("Night prompt should contain 'NIGHT'")
	}
}

func TestBuildPromptWithTimeAndMoon(t *testing.T) {
	prompt := BuildPromptWithTimeAndMoon(ConditionClearCool, TimeNight, MoonFull)
	if prompt == "" {
		t.Error("BuildPromptWithTimeAndMoon() returned empty string")
	}

	dayPrompt := BuildPromptWithTimeAndMoon(ConditionClearWarm, TimeDay, MoonFull)
	if dayPrompt == "" {
		t.Error("BuildPromptWithTimeAndMoon() for day returned empty string")
	}
}

func TestBuildPrompt_UnknownCondition(t *testing.T) {
	prompt := BuildPrompt(WeatherCondition("unknown_condition"))
	if prompt == "" {
		t.Error("BuildPrompt with unknown condition should still return a prompt")
	}
}
