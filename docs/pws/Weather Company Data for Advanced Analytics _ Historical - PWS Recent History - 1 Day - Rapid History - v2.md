

| ![][image1] | Weather Company Data  |  PWS Recent History \- 1 Day \- Rapid History \- v2 Domain Portfolio: Observations  |  Domain: Historical  |  API Name: PWS Recent History \- 1 Day \- Rapid History \- v2 Geography: Global Attribution Required: NO Attribution Requirements:  N/A  |
| :---- | ----- |

### **Overview**

Personal Weather Station (PWS) Rapid Historical Observations returns the daily observations records in rapid frequency as frequent as every 5 minutes. Actual frequency of reports ranges and is dependent on how frequently an individual Personal Weather Station (PWS) reports data.

### **HTTP Headers and Data Lifetime \- Caching and Expiration**

For details on appropriate header values as well as caching and expiration definitions, please see [**The Weather Company Data | API Common Usage Guide**](https://twcapi.co/APICUG)**.**

### **Unit of Measure Requirement**

The unit of measure for the response. The following values are supported:

| e \= English units | m \= Metric units | h \= Hybrid units (UK) |
| :---- | :---- | :---- |

**URL Construction**

| Request by Geocode (Latitude & Longitude): Required Parameters: format, units, stationId, apiKey | Optional Parameter: numericPrecision  |
| :---- |
| https://api.weather.com/v2/pws/observations/all/1day?stationId=KMAHANOV10\&format=json\&units=e\&apiKey=yourApiKey |

### **Valid Parameter Definitions**

| Parameter Name | Valid Parameter Value | Description |
| :---- | :---- | :---- |
| numericPrecision | decimal | Optional parameter.  Set to ‘decimal’ to ensure data is returned in decimal format when needed.  Will return integers if this value is not used. |

### **Data Elements & Definitions**

Note: Field names are sorted alphabetically in the table below for presentation purposes. The table below does not represent the sort order of the API response. 

| Field Name | Description | Type | Range | Sample | Nulls Allowed |
| :---- | :---- | :---: | :---- | :---- | :---: |
| epoch | Time in UNIX seconds | epoch |  | 1369252800 | Y |
| humidityAvg | Average Humidity of the period | integer/decimal |  | 32 | Y |
| humidityHigh | Highest Humidity of the period | integer/decimal |  | 32 | Y |
| humidityLow | Lowest Humidity of the period | integer/decimal |  | 32 | Y |
| lat | Latitude of PWS | decimal | Any valid latitude value. \-90 to 90 | 29.8972 | Y |
| lon | Longitude of PWS | decimal | Any valid longitude value. \-180 to 180 | \-97.9362 | Y |
| obsTimeLocal | Time observation is valid in local apparent time by timezone | ISO | YYYYY-MM-dd HH:mm:ss | 2016-09-27 00:59:39 | Y |
| obsTimeUtc | GMT(UTC) time | ISO | ISO 8601 \- yyyy-MM-dd'T'HH:mm:ssZZ | 2016-09-27T06:59:39Z | Y |
| solarRadiationHigh | Highest Solar Radiation of the period | integer/decimal |  | 947 | Y |
| stationID | ID as registered by wunderground.com | string |  | KAZTUCSO539 | N |
| tz | Time zone of PWS | string |  | America/Chicago | Y |
| uvHigh | Highest UV Index of the period | decimal |  | 2 | Y |
| winddirAvg | Wind direction average of the period | integer/decimal |  | 170 | Y |
| imperial metric metric\_si uk\_hybrid | Object containing fields that use a defined unit of measure. The object label is dependent on the units parameter assigned in the request. "imperial", "metric", "metric\_si", "uk\_hybrid" | object |  | imperial: {.....} | N |
| dewptAvg | Average dew point of the period | integer/decimal | \-80 to 100 (°F) or \-62 to 37 (°C) | 43.0 | Y |
| dewptHigh | Maximum dew point of the period | integer/decimal | \-80 to 100 (°F) or \-62 to 37 (°C) | 43.0 | Y |
| dewptLow | Minimum dew point of the period | integer/decimal | \-80 to 100 (°F) or \-62 to 37 (°C) | 43.0 | Y |
| heatindexAvg | Heat index average of the period | integer/decimal |  | 68.2 | Y |
| heatindexHigh | Heat index high temperature of the period | integer/decimal |  | 71.8 | Y |
| heatindexLow | Heat index low temperature of the period | integer/decimal |  | 61.7 | Y |
| precipRate | Rate of precipitation \- instantaneous precipitation rate. How much rain would fall if the precipitation intensity did not change for one hour | integer/decimal |  | 0.03 | Y |
| precipTotal | Accumulated Rain for the day in defined unit of measure | integer/decimal |  | 0.03 | Y |
| pressureMax | Highest Barometric pressure in defined unit of measure of the period | integer/decimal |  | 30.12 | Y |
| pressureMin | Lowest Barometric pressure in defined unit of measure of the period | integer/decimal |  | 0.01 | Y |
| pressureTrend | Pressure tendency over the preceding period | integer/decimal |  | 28.09 | Y |
| qcStatus | Quality control indicator: \-1: No quality control check performed  0: This observation was marked as possibly incorrect by our quality control algorithm  1: This observation passed quality control checks | integer | \-1 to 1 | 1 | N |
| tempAvg | Temperature average of the period | integer/decimal |  | 72.7 | Y |
| tempHigh | High Temperature of the period | integer/decimal |  | 87.3 | Y |
| tempLow | Low Temperature of the period | integer/decimal |  | 63.7 | Y |
| windchillAvg | Windchill average of the period | integer/decimal |  | 32 | Y |
| windchillHigh | High Windchill temperature of the period | integer/decimal |  | 45 | Y |
| windchillLow | Low Windchill temperature of the period | integer/decimal |  | 35 | Y |
| windgustAvg | Wind gust average of the period | integer/decimal |  | 54 | Y |
| windgustHigh | Highest Wind gust of the period | integer/decimal |  | 56 | Y |
| windgustLow | Lowest Wind gust of the period | integer/decimal |  | 43 | Y |
| windspeedAvg | Wind speed average of the period | integer/decimal |  | 3 | Y |
| windspeedHigh | Highest Wind speed of the period | integer/decimal |  | 5 | Y |
| windspeedLow | Lowest Wind speed of the period | integer/decimal |  | 1 | Y |

### **JSON Sample**

| {   "observations": \[     {       "stationID": "KMAHANOV10",       "tz": "America/New\_York",       "obsTimeUtc": "2016-10-03T04:04:57Z",       "obsTimeLocal": "2016-10-03 00:04:57",       "epoch": 1475467497,       "lat": 42.09263229,       "lon": \-70.86485291,       "solarRadiationHigh": null,       "uvHigh": null,       "winddirAvg": 0,       "humidityHigh": 100,       "humidityLow": 100,       "humidityAvg": 100,       "qcStatus": \-1,       "metric": {         "tempHigh": 12,         "tempLow": 12,         "tempAvg": 12,         "windspeedHigh": 0,         "windspeedLow": 0,         "windspeedAvg": 0,         "windgustHigh": 0,         "windgustLow": 0,         "windgustAvg": 0,         "dewptHigh": 12,         "dewptLow": 12,         "dewptAvg": 12,         "windchillHigh": null,         "windchillLow": null,         "windchillAvg": null,         "heatindexHigh": 12,         "heatindexLow": 12,         "heatindexAvg": 12,         "pressureMax": 1015.58,         "pressureMin": 1015.58,         "pressureTrend": 0,         "precipRate": 0,         "precipTotal": 0       }     },  *// Response Collapsed for Presentation Purposes*   \] } |
| :---- |

[image1]: <data:image/png;base64,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>