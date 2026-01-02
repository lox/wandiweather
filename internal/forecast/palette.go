package forecast

// Palette defines the color scheme for a weather condition + time of day.
type Palette struct {
	// Background is the main page background color
	Background string
	// Card is the background for cards/panels
	Card string
	// CardBorder is an optional border/highlight for cards
	CardBorder string
	// Text is the primary text color
	Text string
	// TextMuted is the secondary/muted text color
	TextMuted string
	// Accent is the primary accent color (links, highlights)
	Accent string
	// AccentAlt is a secondary accent (temperature high, etc.)
	AccentAlt string
}

// DefaultPalette is the fallback dark theme.
var DefaultPalette = Palette{
	Background: "#0f0f1a",
	Card:       "#1a1a2e",
	CardBorder: "#2a2a4e",
	Text:       "#eeeeee",
	TextMuted:  "#666666",
	Accent:     "#4fc3f7",
	AccentAlt:  "#ff7043",
}

// palettes maps condition+time keys to curated color schemes.
// Day palettes are lighter/warmer, night palettes are darker/cooler.
var palettes = map[string]Palette{
	// === CLEAR WARM ===
	"clear_warm_dawn": {
		Background: "#2a2520", // warm dark brown
		Card:       "#3a3530",
		CardBorder: "#554a40",
		Text:       "#fff8f0",
		TextMuted:  "#a09080",
		Accent:     "#ffaa66",
		AccentAlt:  "#ff6644",
	},
	"clear_warm_day": {
		Background: "#f5f0e8", // warm cream
		Card:       "#ffffff",
		CardBorder: "#e0d8c8",
		Text:       "#2a2520",
		TextMuted:  "#706050",
		Accent:     "#d07020",
		AccentAlt:  "#c04010",
	},
	"clear_warm_dusk": {
		Background: "#352820", // warm dark
		Card:       "#453830",
		CardBorder: "#604838",
		Text:       "#fff0e0",
		TextMuted:  "#a08060",
		Accent:     "#ff8844",
		AccentAlt:  "#ee5522",
	},
	"clear_warm_night": {
		Background: "#0a0a12",
		Card:       "#141420",
		CardBorder: "#252535",
		Text:       "#dde0e8",
		TextMuted:  "#556070",
		Accent:     "#7799cc",
		AccentAlt:  "#dd7755",
	},

	// === CLEAR COOL ===
	"clear_cool_dawn": {
		Background: "#1a1820", // cool dawn
		Card:       "#282630",
		CardBorder: "#3a3848",
		Text:       "#f0f0f8",
		TextMuted:  "#8888a0",
		Accent:     "#88aadd",
		AccentAlt:  "#dd8866",
	},
	"clear_cool_day": {
		Background: "#e8f0f5", // cool light blue-gray
		Card:       "#ffffff",
		CardBorder: "#c8d8e8",
		Text:       "#1a2530",
		TextMuted:  "#506070",
		Accent:     "#2080b0",
		AccentAlt:  "#c06030",
	},
	"clear_cool_dusk": {
		Background: "#201820", // purple dusk
		Card:       "#302830",
		CardBorder: "#483848",
		Text:       "#f0e8f0",
		TextMuted:  "#9080a0",
		Accent:     "#aa88cc",
		AccentAlt:  "#cc7755",
	},
	"clear_cool_night": {
		Background: "#060810",
		Card:       "#101420",
		CardBorder: "#1a2030",
		Text:       "#d0d8e8",
		TextMuted:  "#506080",
		Accent:     "#6688bb",
		AccentAlt:  "#cc7766",
	},

	// === PARTLY CLOUDY ===
	"partly_cloudy_dawn": {
		Background: "#201c20",
		Card:       "#302c30",
		CardBorder: "#484048",
		Text:       "#f0eff0",
		TextMuted:  "#908890",
		Accent:     "#99aabb",
		AccentAlt:  "#cc8866",
	},
	"partly_cloudy_day": {
		Background: "#e8ecf0", // light gray
		Card:       "#ffffff",
		CardBorder: "#d0d8e0",
		Text:       "#202830",
		TextMuted:  "#607080",
		Accent:     "#3090c0",
		AccentAlt:  "#d06030",
	},
	"partly_cloudy_dusk": {
		Background: "#251c20",
		Card:       "#352c30",
		CardBorder: "#504048",
		Text:       "#f5eff0",
		TextMuted:  "#a08890",
		Accent:     "#cc8899",
		AccentAlt:  "#cc6655",
	},
	"partly_cloudy_night": {
		Background: "#080810",
		Card:       "#121220",
		CardBorder: "#202030",
		Text:       "#d8d8e0",
		TextMuted:  "#606070",
		Accent:     "#7080a0",
		AccentAlt:  "#aa7766",
	},

	// === MOSTLY CLOUDY ===
	"mostly_cloudy_dawn": {
		Background: "#181818",
		Card:       "#252525",
		CardBorder: "#383838",
		Text:       "#e8e8e8",
		TextMuted:  "#888888",
		Accent:     "#8899aa",
		AccentAlt:  "#bb7755",
	},
	"mostly_cloudy_day": {
		Background: "#dde0e4", // overcast gray
		Card:       "#f0f2f4",
		CardBorder: "#c0c8d0",
		Text:       "#252830",
		TextMuted:  "#606870",
		Accent:     "#4080a0",
		AccentAlt:  "#b05530",
	},
	"mostly_cloudy_dusk": {
		Background: "#1a1618",
		Card:       "#282428",
		CardBorder: "#3a3638",
		Text:       "#e8e4e8",
		TextMuted:  "#888088",
		Accent:     "#998899",
		AccentAlt:  "#aa6655",
	},
	"mostly_cloudy_night": {
		Background: "#0a0a0c",
		Card:       "#141416",
		CardBorder: "#1e1e22",
		Text:       "#d0d0d4",
		TextMuted:  "#585860",
		Accent:     "#606878",
		AccentAlt:  "#886666",
	},

	// === LIGHT RAIN ===
	"light_rain_dawn": {
		Background: "#141618",
		Card:       "#1e2224",
		CardBorder: "#2a3035",
		Text:       "#e0e4e8",
		TextMuted:  "#707880",
		Accent:     "#5588aa",
		AccentAlt:  "#aa7060",
	},
	"light_rain_day": {
		Background: "#d8e0e8", // rainy gray-blue
		Card:       "#e8f0f4",
		CardBorder: "#b8c8d4",
		Text:       "#1a2028",
		TextMuted:  "#506068",
		Accent:     "#3070a0",
		AccentAlt:  "#a05535",
	},
	"light_rain_dusk": {
		Background: "#161418",
		Card:       "#22202a",
		CardBorder: "#32303a",
		Text:       "#e4e0e8",
		TextMuted:  "#787080",
		Accent:     "#7070a0",
		AccentAlt:  "#a06055",
	},
	"light_rain_night": {
		Background: "#06080a",
		Card:       "#0e1014",
		CardBorder: "#181c22",
		Text:       "#c8ccd4",
		TextMuted:  "#505860",
		Accent:     "#506080",
		AccentAlt:  "#887066",
	},

	// === HEAVY RAIN ===
	"heavy_rain_dawn": {
		Background: "#101214",
		Card:       "#181c20",
		CardBorder: "#242a30",
		Text:       "#d8dce0",
		TextMuted:  "#606870",
		Accent:     "#4a7090",
		AccentAlt:  "#886055",
	},
	"heavy_rain_day": {
		Background: "#c8d0d8", // dark rainy
		Card:       "#dce4e8",
		CardBorder: "#a8b8c4",
		Text:       "#181c20",
		TextMuted:  "#485058",
		Accent:     "#306088",
		AccentAlt:  "#904830",
	},
	"heavy_rain_dusk": {
		Background: "#121012",
		Card:       "#1c181c",
		CardBorder: "#282428",
		Text:       "#dcd8dc",
		TextMuted:  "#686068",
		Accent:     "#605880",
		AccentAlt:  "#885550",
	},
	"heavy_rain_night": {
		Background: "#050606",
		Card:       "#0a0c0e",
		CardBorder: "#141618",
		Text:       "#bcc0c4",
		TextMuted:  "#404448",
		Accent:     "#405060",
		AccentAlt:  "#705858",
	},

	// === STORM ===
	"storm_dawn": {
		Background: "#100c10",
		Card:       "#1a161c",
		CardBorder: "#2a2430",
		Text:       "#dcd8e0",
		TextMuted:  "#686078",
		Accent:     "#8855aa",
		AccentAlt:  "#aa5555",
	},
	"storm_day": {
		Background: "#c0c4cc", // stormy gray
		Card:       "#d4d8e0",
		CardBorder: "#a0a8b4",
		Text:       "#181820",
		TextMuted:  "#484858",
		Accent:     "#6050a0",
		AccentAlt:  "#a04040",
	},
	"storm_dusk": {
		Background: "#120c10",
		Card:       "#1e1418",
		CardBorder: "#302028",
		Text:       "#e0d8dc",
		TextMuted:  "#785868",
		Accent:     "#a05080",
		AccentAlt:  "#aa5050",
	},
	"storm_night": {
		Background: "#040406",
		Card:       "#0a080c",
		CardBorder: "#141218",
		Text:       "#c0b8c0",
		TextMuted:  "#484050",
		Accent:     "#604070",
		AccentAlt:  "#804848",
	},

	// === FOG ===
	"fog_dawn": {
		Background: "#181a1c",
		Card:       "#242628",
		CardBorder: "#343638",
		Text:       "#e0e2e4",
		TextMuted:  "#808284",
		Accent:     "#8090a0",
		AccentAlt:  "#a08070",
	},
	"fog_day": {
		Background: "#d8dce0", // misty light
		Card:       "#eaecf0",
		CardBorder: "#c0c4c8",
		Text:       "#202428",
		TextMuted:  "#606468",
		Accent:     "#607080",
		AccentAlt:  "#906858",
	},
	"fog_dusk": {
		Background: "#141214",
		Card:       "#201e20",
		CardBorder: "#302e30",
		Text:       "#dcdadc",
		TextMuted:  "#787678",
		Accent:     "#887888",
		AccentAlt:  "#906858",
	},
	"fog_night": {
		Background: "#080808",
		Card:       "#101010",
		CardBorder: "#1c1c1c",
		Text:       "#c4c4c8",
		TextMuted:  "#505054",
		Accent:     "#585860",
		AccentAlt:  "#706060",
	},

	// === HOT ===
	"hot_dawn": {
		Background: "#281c14", // warm brown
		Card:       "#382c20",
		CardBorder: "#504030",
		Text:       "#fff4e8",
		TextMuted:  "#a08868",
		Accent:     "#ee9944",
		AccentAlt:  "#ee5522",
	},
	"hot_day": {
		Background: "#f8f0e0", // hot cream/tan
		Card:       "#ffffff",
		CardBorder: "#e8d8c0",
		Text:       "#302010",
		TextMuted:  "#806040",
		Accent:     "#d07010",
		AccentAlt:  "#d03000",
	},
	"hot_dusk": {
		Background: "#281810", // burnt orange dark
		Card:       "#382418",
		CardBorder: "#503020",
		Text:       "#fff0e0",
		TextMuted:  "#a07858",
		Accent:     "#dd6622",
		AccentAlt:  "#cc3300",
	},
	"hot_night": {
		Background: "#100c08",
		Card:       "#1a1410",
		CardBorder: "#28201a",
		Text:       "#e0d8cc",
		TextMuted:  "#706050",
		Accent:     "#aa7744",
		AccentAlt:  "#bb5544",
	},

	// === FROST ===
	"frost_dawn": {
		Background: "#101820", // cold blue
		Card:       "#182430",
		CardBorder: "#283848",
		Text:       "#e8f0f8",
		TextMuted:  "#7090a8",
		Accent:     "#66a0cc",
		AccentAlt:  "#cc8866",
	},
	"frost_day": {
		Background: "#e4ecf4", // icy light blue
		Card:       "#f4f8fc",
		CardBorder: "#c4d4e4",
		Text:       "#102030",
		TextMuted:  "#406080",
		Accent:     "#2080b8",
		AccentAlt:  "#c06040",
	},
	"frost_dusk": {
		Background: "#0c1018", // cold twilight
		Card:       "#141c28",
		CardBorder: "#202c3c",
		Text:       "#e0e8f0",
		TextMuted:  "#6080a0",
		Accent:     "#7090bb",
		AccentAlt:  "#bb7766",
	},
	"frost_night": {
		Background: "#040810",
		Card:       "#0a1018",
		CardBorder: "#121c28",
		Text:       "#d0d8e4",
		TextMuted:  "#506080",
		Accent:     "#5080a0",
		AccentAlt:  "#a07070",
	},
}

// GetPalette returns the color palette for a weather condition and time of day.
func GetPalette(condition WeatherCondition, tod TimeOfDay) Palette {
	key := string(ConditionWithTime(condition, tod))
	if p, ok := palettes[key]; ok {
		return p
	}
	return DefaultPalette
}
