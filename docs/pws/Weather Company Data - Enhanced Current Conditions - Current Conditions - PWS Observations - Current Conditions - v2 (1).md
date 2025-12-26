

| ![][image1] | Weather Company Data |  PWS Observations \- Current Conditions \- v2 Domain Portfolio: Observations  |  Domain: Current Conditions  |  API Name: PWS Observations \- Current Conditions \- v2 Geography: Global Attribution Required: NO Attribution Requirements:  N/A  |
| :---- | ----- |

### **Overview**

Personal Weather Stations (PWS) Current Conditions returns the current conditions observations for the current record.Current record is the last record reported within 60 minutes. If the station has not reported a current conditions in the past 60 minutes, the response will not return an expired observation record (older than 60 minutes); a 'Data Expired' message will be returned instead.

### **HTTP Headers and Data Lifetime \- Caching and Expiration**

For details on appropriate header values as well as caching and expiration definitions, please see [**The Weather Company Data | API Common Usage Guide**](https://twcapi.co/APICUG)**.**

### **Unit of Measure Requirement**

The unit of measure for the response. The following values are supported:

| e \= English units | m \= Metric units | h \= Hybrid units (UK) |
| :---- | :---- | :---- |

**\*Note Aggregate Product Name not intended for use unless it is in conjuction with one or more additional products.**  
**URL Construction**

| Atomic API URL Examples:  | Aggregate Product Name | v2pwsObsCur\* |
| :---- | ----: | :---: |
| **Request by PWS Station ID: Required Parameters: stationId, format, units, apiKey**  **| Optional Parameter: numericPrecision** |  |  |
| https://api.weather.com/v2/pws/observations/current?stationId=KMAHANOV10\&format=json\&units=e\&apiKey=yourApiKey |  |  |

### 

### **Valid Parameter Definitions**

| Parameter Name | Valid Parameter Value | Description |
| :---- | :---- | :---- |
| numericPrecision | decimal | Optional parameter.  Set to ‘decimal’ to ensure data is returned in decimal format when needed.  Will return integers if this value is not used. |

### 

### **Data Elements & Definitions**

Note: Field names are sorted alphabetically in the table below for presentation purposes. The table below does not represent the sort order of the API response. 

| Field Name | Description | Type | Range | Sample | Nulls Allowed |
| :---- | :---- | :---: | :---- | :---- | :---: |
| country | Country Code | string |  | US | Y |
| epoch | Time in UNIX seconds | epoch |  | 1475157931 | Y |
| humidity | The relative humidity of the air. | integer/decimal |  | 71 | Y |
| lat | Latitude of PWS | decimal | Any valid latitude value. \-90 to 90 | 32.50828934 | Y |
| lon | Longitude of PWS | decimal | Any valid longitude value. \-180 to 180 | \-110.8763962 | Y |
| neighborhood | Neighborhood associated with the PWS location | string |  | WOW Arizona\! | Y |
| obsTimeLocal | Time observation is valid in local apparent time by timezone \- tz | ISO | YYYYY-MM-dd HH:mm:ss | 2016-09-29 14:05:31 | Y |
| obsTimeUtc | GMT(UTC) time | ISO | ISO 8601 \- yyyy-MM-dd'T'HH:mm:ssZZ  | 2016-09-29T14:05:31Z | Y |
| qcStatus | Quality control indicator: \-1: No quality control check performed  0: This observation was marked as possibly incorrect by our quality control algorithm  1: This observation passed quality control checks | integer | \-1 to 1 | 1 | N |
| realtimeFrequency | Frequency of data report updates in minutes | integer/decimal |  | 5 | Y |
| softwareType | Software type of the PWS | string |  | WS-1001 V2.2.9 | Y |
| solarRadiation | Solar Radiation | integer/decimal |  | 92.0 | Y |
| stationID | ID as registered by wunderground.com | string |  | KAZTUCSO539 | N |
| uv | UV reading of the intensity of solar radiation | integer/decimal |  | 1.2 | Y |
| winddir | Wind Direction | integer/decimal |  | 52 | Y |
| imperial metric metric\_si uk\_hybrid | Object containing fields that use a defined unit of measure. The object label is dependent on the units parameter assigned in the request. "imperial", "metric", "metric\_si", "uk\_hybrid" | object |  | imperial: {.....} | N |
| dewpt | The temperature which air must be cooled at constant pressure to reach saturation. The Dew Point is also an indirect measure of the humidity of the air. The Dew Point will never exceed the Temperature. When the Dew Point and Temperature are equal, clouds or fog will typically form. The closer the values of Temperature and Dew Point, the higher the relative humidity. | integer/decimal | \-80 to 100 (°F) or \-62 to 37 (°C) | 58 | Y |
| elev | Elevation | integer/decimal |  | 3094 | Y |
| heatIndex | Heat Index \- An apparent temperature. It represents what the air temperature “feels like” on exposed human skin due to the combined effect of warm temperatures and high humidity. When the temperature is 70°F or higher, the Feels Like value represents the computed Heat Index. | integer/decimal |  | 67 | Y |
| precipRate | Rate of precipitation \- instantaneous precipitation rate.  How much rain would fall if the precipitation intensity did not change for one hour | integer/decimal |  | 0.0 | Y |
| precipTotal | Accumulated precipitation for today from midnight to present. | integer/decimal |  | 0.0 | Y |
| pressure | Mean Sea Level Pressure, the equivalent pressure reading at sea level recorded at this station | integer/decimal |  | 30.06 | Y |
| temp | Temperature in defined unit of measure. | integer/decimal |  | 67 | Y |
| windChill | Wind Chill \- An apparent temperature. It represents what the air temperature “feels like” on exposed human skin due to the combined effect of the cold temperatures and wind speed. When the temperature is 61°F or lower the Feels Like value represents the computed Wind Chill so display the Wind Chill value. | integer/decimal |  | \-34 | Y |
| windGust | Wind Gust \- sudden and temporary variations of the average Wind Speed. The report always shows the maximum wind gust speed recorded during the observation period. It is a required display field if Wind Speed is shown. | integer/decimal |  | 56 | Y |
| windSpeed | Wind Speed \- The wind is treated as a vector; hence, winds must have direction and magnitude (speed). The wind information reported in the hourly current conditions corresponds to a 10-minute average called the sustained wind speed. Sudden or brief variations in the wind speed are known as “wind gusts” and are reported in a separate data field. Wind directions are always expressed as ""from whence the wind blows"" meaning that a North wind blows from North to South. If you face North in a North wind the wind is at your face. Face southward and the North wind is at your back. | integer/decimal |  | 56 | Y |

### 

### **JSON Sample**

| { observations: \[ { stationID: "KNCCARY89", obsTimeUtc: "2019-02-04T14:53:14Z", obsTimeLocal: "2019-02-04 09:53:14", neighborhood: "Highcroft Village", softwareType: "GoWunder 1337.9041ac1", country: "US", solarRadiation: 436.0, lon: \-78.8759613, realtimeFrequency: null, epoch: 1549291994, lat: 35.80221176, uv: 1.2, winddir: 329, humidity: 71, qcStatus: 1, imperial: { temp: 53, heatIndex: 53, dewpt: 44, windChill: 53, windSpeed: 2, windGust: null, pressure: 30.09, precipRate: 0.0, precipTotal: 0.0, elev: 413 } } \] } |
| :---- |

[image1]: <data:image/png;base64,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>