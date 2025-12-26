

| ![][image1] | Location Service \- Near \- v3.0 Domain Portfolio: Utility  |  Domain: Search  |  Usage Classification: Standard Geography: Global Attribution Required: N/A Attribution Requirements:  N/A  |
| :---- | ----- |

### **Overview**

The Location Service \- Near API provides the ability to lookup valid Personal Weather Station (PWS) station identifiers (stationId), Airports identifiers to retrieve a set of valid identifiers, Ski Resorts, or METAR Station Observations, and distance information matching the request. 

* Airport: The response shall include up to 10 Airports nearest the requested location  
* Observation: The response shall include up to 10 Observations for METAR Stations nearest the requested location  
* PWS: The response shall include up to 10 Personal Weather Stations nearest the requested location  
* Ski: The response shall include up to 10 Ski Resorts nearest the requested location

### **HTTP Headers and Data Lifetime \- Caching and Expiration**

For details on appropriate header values as well as caching and expiration definitions, please see [**The Weather Company Data | API Common Usage Guide**](https://twcapi.co/APICUG)**.**

**URL Construction**

| Atomic API URL Examples:  | Aggregate Product Name | v3-location-near |
| :---- | :---- | :---- |
| **Search Airport by Geocode**: **Required Parameters:** **geocode, format, product, apiKey** |  |  |
| https://api.weather.com/v3/location/near?**geocode=33.74,-84.39**&**product=airport**\&format=json\&apiKey=**yourApiKey** |  |  |
| **Search Observation by Geocode**: **Required Parameters:** **geocode, format, product, apiKey**  |  |  |
| https://api.weather.com/v3/location/near?**geocode=33.74,-84.39**&**product=observation**\&format=json\&apiKey=**yourApiKey** |  |  |
| **Search PWS by Geocode**: **Required Parameters:** **geocode, format, product, apiKey**  |  |  |
| https://api.weather.com/v3/location/near?**geocode=33.74,-84.39**&**product=pws**\&format=json\&apiKey=**yourApiKey** |  |  |
| **Search Ski Resort by Geocode**: **Required Parameters:** **geocode, format, product, apiKey** |  |  |
| https://api.weather.com/v3/location/near?**geocode=33.74,-84.39**&**product=ski**\&format=json\&apiKey=**yourApiKey** |  |  |

### **Parameter Definitions**

| Parameter | Name | Description | Example |
| :---- | :---- | :---- | :---- |
| API Key | apiKey  | The “apiKey” is used for access using the api; usually customer specific. | ..v3/location/near?**apiKey=yourApiKey** |
| Format | format | The format of the response (json) | ..v3/location/near?**format=json** |
| Product | product | The product is the identifier type used in the query. Valid values include; airport, observation, pws, ski | ..v3/location/near?**product=airport** ..v3/location/near?**product=observation**  ..v3/location/near?**product=pws** ..v3/location/near?**product=ski** |
| **Location Parameter** | **Name** | **Description** | **Example** |
| Geocode | geocode | TWC uses valid latitudes and longitude coordinates to identify locations worldwide.  Uses WGS84 geocode coordinate reference system. [https://www.w3.org/2003/01/geo/](https://www.w3.org/2003/01/geo/) **33.40,83.19** is the manner in which a latitude and longitude is accepted for the weather APIs | ..v3/location/near?**geocode=34.063,-84.217** |

### **Data Elements & Definitions**

| Field Name | Description | Type | Range | Sample | Nulls Allowed |
| :---- | :---- | :---: | :---- | :---- | :---: |
| location | Object for location data | \[array\] |  |  |  |
| latitude | Latitude of the requested location | \[decimal\] | Any valid latitude value | 33.638271, | N |
| longitude | Longitude of the requested location | \[decimal\] | Any valid longitude value | \-84.429369, | N |
| distanceKm | Distance from requested location in kilometers | \[decimal\] |  | 0.4 | Y |
| distanceMi | Distance from requested location in miles | \[decimal\] |  | 0.25 | Y |
| **Fields Unique to PWS** |  |  |  |  |  |
| stationId | The PWS station identifier | \[string\] |  | KGAATLAN81 | Y |
| stationName | The station name near the requested location | \[string\] |  | Atlanta | N |
| partnerId | For internal use only and to be disregarded | \[string\] |  | null | Y |
| qcStatus | The indicator to signify whether the station has passed quality control checks: \-1=not checked (default value); 0=failed; 1=OK | \[string\] |  | 1 | Y |
| updateTimeUtc | An epoch time indicating the date of the last update received from the station | \[epoch\] |  | 1557819905 | Y |
| **Fields Unique to Airport** |  |  |  |  |  |
|  airportName | The airport names near the requested location | \[string\] |  | Hartsfield-Jackson Intl | Y |
| iataCode | The International Air Transport Association (IATA) airport codes of the requested location | \[string\] | Any valid IATA code | ATL | Y |
| icaoCode | The International Civil Aviation Organization (ICAO) airport codes of the requested location | \[string\] | Any valid ICAO code | KATL | Y |
| **Fields Unique to Ski Resorts** |  |  |  |  |  |
| adminDistrictCode | The internationalized State, Region, District or Province identifier code for ‘state’ or geopolitical area \- level 1 administrative division \- codes are ISO 3166-2 compliant | \[string\] | Any valid state, region, district or province code | GA, | Y |
| countryCode | ISO Country Code | \[string\] | Any valid ISO country code | US, | Y |
| ianaTimeZone | The standard IANA Time Zone for the location requested | \[string\] | Any valid IANA time zone | America/New\_York, | Y |
| skiId | The ski resort identifier near the requested location | \[string\] |  | 347 | Y |
| skiName | The ski resort name of the requested location | \[string\] |  | Sapphire Valley | Y |

### **JSON Sample**

| Airport Search |
| :---- |
| {   "location": {      "latitude":\["33.9152806","33.7791264","33.8756111","34.0131667","33.6366996"\]      "longitude":\["-84.5163528","-84.521366","-84.3019722","-84.5970278","-84.427864"\]      "distanceKm":\[5.91,13.57,14.68,18.64,28.33\]      "distanceMi":\[3.67, 8.43, 9.12, 11.58, 17.6\]      "airportName":\["Dobbins Air","Fulton County","Dekalb-Peachtree","Cobb County","Hartsfield-Jackson Intl"\]      "iataCode":\["MGE","FTY","PDK","RYY","ATL"\]      "icaoCode":\["KMGE","KFTY","KPDK","KRYY","KATL"\]   } } |
| **Observation Search** |
| {  “location”: {     “adminDistrictCode”:\[null,null,null,null,null\]     “stationName”:\["Aberdeen/Amory","Columbus AFB","Haleyville","Columbus","Jasper"\]     “countryCode”:\["US","US","US","US",“US"\]     “stationId”:\["KM40","KCBM","K1M4","KGTR","KJFX"\]     “ianaTimeZone”:\["America/Chicago","America/Chicago","America/Chicago","America/Chicago","America/Chicago"\]     “obsType”:\["METAR","METAR","METAR","METAR","METAR"\]     “latitude”:\[33.867,33.65,34.2833,33.45,33.9\]     “longitude”:\[-88.483,-88.45,-87.6,-88.5833,-87.317\]     “distanceKm”:\[36.46,43.36,62.07,67.73,71.34\]     “distanceMi”:\[22.66,26.94,38.57,42.09,44.33\]   } } |
| **PWS Search** |
| {  "location": {     "stationName":\["Atlanta","Atlanta","Atlanta","Forest Park"\],     "stationId":\["KGAATLAN398","KGAATLAN41","KGAATLAN378","KGAFORES2"\],     "latitude":\[“33.66058”,”33.68513”,”33.68098”,”33.63761”\],     "longitude":\[“-84.46119”,”-84.45192”,”-84.48737”,”-84.35225”\],     "distanceKm":\[3.4,5.21,6.71,7.48\],     "distanceMi":\[2.11,3.24,4.17,4.65\]     "partnerId":\[null,null,null,null\],     "qcStatus":\[1,1,-1,-1\],     "updateTimeUtc":\[1557811139,1557797749,null,null\]   } } |
| **Ski Search** |
| {   "location": {      "adminDistrictCode”:\["NC","NC","TN"\]      "countryCode”:\["US","US","US"\]      "distanceKm”:\[202.54,246.66,247.72\]      "distanceMi”:\[125.85,153.27,153.92\]      "ianaTimeZone”:\["America/New\_York","America/New\_York","America/New\_York"\]      "latitude”:\["35.089","35.5621","35.7231"\]      "longitude”:\["-83.097","-83.0909","-83.4802"\]      "skiId”:\["347","103","303"\]      "skiName”:\["Sapphire Valley","Cataloochee Ski Area","Ober Gatlinburg Ski Resort"\]   } } |

[image1]: <data:image/png;base64,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>