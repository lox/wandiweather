# WandiWeather Research Summary

> **Last updated**: December 26, 2025

## Key Findings (from 7 days of data)

### WU vs BOM Forecast Accuracy

Early verification shows WU (GRAF model) significantly outperforms BOM for Wandiligong:

| Date | Source | Forecast Max | Actual Max | Bias |
|------|--------|-------------|------------|------|
| Dec 26 | WU | 21°C | 21°C | **0°C** ✓ |
| Dec 26 | BOM | 29°C | 21°C | **+8°C** |

BOM forecasts for Wangaratta (35km away) consistently predict 4-8°C warmer maximums than actually occur in Wandiligong valley.

### Station Data Quality

**IWANDI8 excluded** - Rain gauge appears broken:
- Recorded 2.1mm on Dec 21 when nearby stations got 40-85mm
- Other sensors (temp, humidity) appear functional
- Marked `active=false` in database

### Inversion Detection

Valley floor inversions are weaker than expected:
- Typical overnight difference: 1°C (valley vs upper)
- Expected based on elevation: ~2°C
- Threshold set at >1°C for detection
- Clear nights (Dec 23-26) showed consistent but mild inversions

### Wind Patterns (Unexpected)

Valley floor stations are **windier** than mid-slope/upper:

| Tier | Avg Wind | Calm % |
|------|----------|--------|
| Valley floor (96-161m) | 2-5 km/h | 0-4% |
| Mid-slope (316-367m) | 0.6-1.3 km/h | 46-63% |
| Upper (400m) | 0.4 km/h | 69% |

Suggests mid-slope/upper stations are in sheltered locations, or valley funnels wind.

### Precipitation Variability

Major rain event Dec 21 showed significant local variation:
- IWANDI22 (367m): 84.6mm
- IVICTORI162 (400m): 31.5mm (same elevation band, 50% less rain)

---

## Data Sources - Verified Working

### 1. Weather Underground PWS API ✅

**Base URL**: `https://api.weather.com/v2/pws/`

**Authentication**: API key via `apiKey` query param (stored in `PWS_API_KEY` env var)

#### Available Local Stations (ALL TESTED & WORKING)

| Station | Location | Lat | Lon | Elev | Distance |
|---------|----------|-----|-----|------|----------|
| IWANDI8 | Wandiligong | -36.779 | 146.977 | 364m | 0.3km |
| IWANDI24 | Wandiligong | -36.786 | 146.992 | 161m | 1.3km |
| IWANDI10 | Wandiligong | -36.767 | 146.981 | 355m | 1.5km |
| IWANDI22 | Wandiligong | -36.767 | 146.982 | 367m | 1.5km |
| **IWANDI23** | Wandiligong | -36.794 | 146.977 | 117m | 1.6km | ⭐ YOUR STATION |
| IVICTORI162 | Wandiligong | -36.757 | 146.986 | 400m | 2.6km |
| IBRIGH55 | Bright | -36.742 | 146.973 | 336m | 4.3km |
| IBRIGH127 | Bright | -36.732 | 146.973 | 316m | 5.4km |
| IBRIGH169 | Bright | -36.731 | 146.985 | 101m | 5.5km |
| **IBRIGH180** | Bright | -36.729 | 146.968 | 96m | 5.8km |

**Note**: Elevation varies significantly (96m - 400m) which is useful for detecting inversions!

#### Endpoints

| Endpoint | Description | Data Resolution |
|----------|-------------|-----------------|
| `/observations/current?stationId={id}` | Current conditions | Real-time (last 60min) |
| `/observations/all/1day?stationId={id}` | Today's history | ~5 min intervals |
| `/observations/hourly/7day?stationId={id}` | 7-day hourly history | Hourly summaries |

#### Sample Current Response (IWANDI8)
```json
{
  "stationID": "IWANDI8",
  "obsTimeUtc": "2025-12-26T01:38:53Z",
  "obsTimeLocal": "2025-12-26 12:38:53",
  "neighborhood": "Wandiligong",
  "lat": -36.779,
  "lon": 146.977,
  "humidity": 38,
  "uv": 7.0,
  "winddir": 83,
  "solarRadiation": 765.2,
  "metric": {
    "temp": 18,
    "heatIndex": 18,
    "dewpt": 4,
    "windChill": 18,
    "windSpeed": 3,
    "windGust": 7,
    "pressure": 974.53,
    "precipRate": 0.00,
    "precipTotal": 0.00,
    "elev": 364
  }
}
```

#### Sample 1-Day History
- Returns ~150+ records per day (~5 min intervals)
- Each record includes: tempHigh/Low/Avg, windspeed, windgust, dewpt, humidity, pressure, precip

### 2. BOM Data ✅ (via FTP)

**Note**: Direct HTTP requests are blocked (403 Forbidden). FTP access works reliably.

#### FTP Access
- **Base URL**: `ftp://ftp.bom.gov.au/anon/gen/fwo/`
- **Forecasts (Victoria)**: `IDV10753.xml` - 7-day précis forecasts for all VIC locations
- **Observations (Victoria)**: `IDV60920.xml` - Current conditions from all VIC AWS stations

#### Relevant Forecast Locations (from IDV10753.xml)
| Location | AAC Code | Notes |
|----------|----------|-------|
| Wangaratta | VIC_PT075 | Closest BOM forecast point (~35km from Wandiligong) |
| Mt Hotham | VIC_PT048 | Alpine reference (1849m) |
| Falls Creek | VIC_PT022 | Alpine reference (1765m) |

#### Nearest BOM Observation Stations (from IDV60920.xml)
| Station | WMO ID | BOM ID | Elevation | Distance | Notes |
|---------|--------|--------|-----------|----------|-------|
| Falls Creek | 94903 | 083084 | 1765m | 32km | Best alpine reference |
| Mt Hotham | 94906 | 083085 | 1849m | 34km | Alpine, often calm |
| Wangaratta Aero | 94889 | 082138 | 153m | 68km | Lowland, poor valley rep |

#### Sample BOM Forecast (Wangaratta, Dec 26 2025)
```xml
<area aac="VIC_PT075" description="Wangaratta">
  <forecast-period index="0">
    <element type="air_temperature_maximum" units="Celsius">25</element>
    <text type="precis">Sunny.</text>
    <text type="probability_of_precipitation">0%</text>
  </forecast-period>
  <forecast-period index="1">
    <element type="air_temperature_minimum" units="Celsius">6</element>
    <element type="air_temperature_maximum" units="Celsius">29</element>
  </forecast-period>
</area>
```

#### Sample BOM Observation (Falls Creek)
```xml
<station wmo-id="94903" stn-name="FALLS CREEK" stn-height="1765.00">
  <period time-utc="2025-12-26T02:20:00+00:00">
    <element type="air_temperature" units="Celsius">6.1</element>
    <element type="dew_point" units="Celsius">-1.5</element>
    <element type="rel-humidity" units="%">58</element>
    <element type="msl_pres" units="hPa">1011.0</element>
    <element type="wind_spd_kmh" units="km/h">20</element>
  </period>
</station>
```

#### Comparison: WU vs BOM Forecasts
| Source | Today Max | Tomorrow Min | Tomorrow Max |
|--------|-----------|--------------|--------------|
| WU (GRAF) for Wandiligong | 20°C | 6°C | 25°C |
| BOM for Wangaratta | 25°C | 6°C | 29°C |

BOM Wangaratta is 35km away and at similar elevation (153m vs 117m), so forecasts should be comparable. The bias between BOM forecast and Wandiligong actuals will show valley cooling effect.

### 3. Forecast API ✅

**Endpoint**: `https://api.weather.com/v3/wx/forecast/daily/5day`

**Parameters**:
- `geocode=-36.794,146.977` (Wandiligong coords)
- `format=json`
- `units=m`
- `language=en-AU`
- `apiKey={key}`

**Available tiers**: 3, 5 day (free tier), 7, 10, 15 day (paid tiers)

**Forecast Source**: The Weather Company's **GRAF** (Global High-Resolution Atmospheric Forecasting) model:
- **NOT from BOM** - proprietary IBM/Weather Company model
- Resolution: 3.5km over land (vs ~10km for GFS/ECMWF)
- Updates: Hourly (vs 6-12 hourly for other global models)
- Inputs: GFS, ECMWF, NAM models + PWS network data + smartphone barometers

**Why bias correction is needed**: GRAF is a global model that doesn't account for:
- Valley cold air drainage (Wandiligong can be 5-10°C colder than predicted overnight)
- Local temperature inversions
- Sheltered microclimate effects

PWS data (including IWANDI23) feeds into GRAF for current conditions, but forecasts are derived from coarse atmospheric modeling that misses valley-specific effects.

#### Sample Response
```json
{
  "calendarDayTemperatureMax": [20, 25, 29, 31, 30, 24],
  "calendarDayTemperatureMin": [4, 6, 9, 12, 15, 11],
  "dayOfWeek": ["Friday", "Saturday", "Sunday", "Monday", "Tuesday", "Wednesday"],
  "narrative": ["Generally clear. Highs 19 to 21°C and lows 5 to 7°C.", ...]
}
```

---

## Valley Microclimate Considerations

Wandiligong/Bright sits in the Ovens Valley (~350-400m elevation):

1. **Cold Air Drainage**: Valley floors trap cold air at night → much colder than surrounding areas
2. **Temperature Inversions**: Common in winter - valley can be 10°C+ colder than alpine stations
3. **Rain Shadow**: Alpine stations may show rain that doesn't reach the valley
4. **Wind Shelter**: Valley is protected from prevailing winds

### Model Strategy

```
PRIMARY SOURCE: IWANDI23 (your station, 117m valley floor)
  - Ground truth for your exact location
  - Best for capturing cold air pooling/frost

ELEVATION TIERS (for inversion detection):
  Valley floor (96-161m):  IWANDI23*, IWANDI24, IBRIGH169, IBRIGH180
  Mid-slope (316-367m):    IWANDI8, IWANDI10, IWANDI22, IBRIGH55, IBRIGH127  
  Upper (400m):            IVICTORI162

INVERSION DETECTION:
  If valley_floor_temp < upper_temp → inversion active
  Useful for frost warnings, fog prediction

For Temperature:
  - Primary: IWANDI23 (your station)
  - Cross-check: IWANDI24, IBRIGH169 (similar elevation)
  - Inversion ref: IVICTORI162 (highest local)

For Precipitation:
  - Primary: IWANDI23
  - Cross-check: nearby stations for storm tracking

For Pressure/Trends:
  - Use IWANDI23 directly (all stations report MSL-adjusted pressure)
  - Compare with neighbours for trend validation

For Wind:
  - IWANDI23 primary (sheltered valley = light winds typical)
```

---

## Technical Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    wandiweather.com                         │
├─────────────────────────────────────────────────────────────┤
│  INGEST LAYER (cron every 5 min)                           │
│  ├── WU API: IWANDI8, IBRIGH180 → Current + History        │
│  └── (Future) BOM via FTP/proxy → Alpine context           │
│                                                             │
│  STORAGE                                                    │
│  └── SQLite: observations, daily_summaries, forecasts      │
│                                                             │
│  PROCESSING                                                 │
│  ├── Data validation (qcStatus check)                      │
│  ├── Gap filling (interpolation)                           │
│  └── Derived metrics (feels like, trend detection)         │
│                                                             │
│  API                                                        │
│  ├── GET /current      → Latest blended conditions         │
│  ├── GET /history      → Historical data                   │
│  ├── GET /forecast     → Short-term forecast               │
│  └── GET /stats        → Daily/monthly summaries           │
│                                                             │
│  FRONTEND                                                   │
│  └── Simple dashboard with current, forecast, charts       │
└─────────────────────────────────────────────────────────────┘
```

---

## Next Steps

1. **MVP**: Build ingest for IWANDI8 + simple display
2. **Storage**: Set up SQLite for historical data
3. **Frontend**: Basic dashboard
4. **Enhancement**: Add IBRIGH180 for redundancy
5. **Future**: BOM integration for alpine context
