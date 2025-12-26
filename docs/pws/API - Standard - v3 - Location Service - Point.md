

| ![][image1] | Location Service \- Point \- v3.0 Domain Portfolio: Utility  |  Domain: Search  |  Usage Classification: Standard Geography: Global Attribution Required: N/A Attribution Requirements:  N/A  |
| :---- | ----- |

### **Overview**

The Location Service APIs provide the ability to lookup a location name or geocode (latitude and longitude) to retrieve a set of locations matching the request. 

The Location Point API provides detailed location data for a specific point on the globe, searchable by: Geocode (Latitude and Longitude), Postal Key (postal code:country code composite), IATA Code, ICAO Code, Place ID (encrypted location identifier), Canonical City ID (city level feature identifier), and Location ID (legacy location identifier).  

The Location Point API is most commonly used for GPS based location searches where a pair of coordinates or known location codes are queried to identify its location features and other related metadata.

Note: Points over a body of water will generate a 404 error.  

### **HTTP Headers and Data Lifetime \- Caching and Expiration**

For details on appropriate header values as well as caching and expiration definitions, please see [**The Weather Company Data | API Common Usage Guide**](https://twcapi.co/APICUG)**.**

**Translated Fields:**  
This TWC API handles the translation of phrases where translated values exist. However, when formatting a request URL a valid language must be passed along (see the language code table for the supported codes).  The following fields will be translated into the requested language where possible:

*address, adminDistrict, city, country, displayName, displayContext, locale, neighborhood, disputedCountries*

**URL Construction**

| Search by Geocode: Required Parameters: geocode, language, format || Optional Parameters: locationType Returns information for a search via a latitude,longitude pair like “33.43,-84.22”.   | Aggregate Product Name |
| :---- | :---- |
| https://api.weather.com/v3/location/point?**geocode=33.74,-84.39**\&language=en-US\&format=json\&apiKey=**yourApiKey** | v3-location-point |
| **Search by Postal Key: Required Parameters:** postalKey, language, format **|| Optional Parameters:** locationType Returns information for search via a Postal Key like “30339:US”.   A Postal Key is a composite of \<Postal Code\>:\<Country Code\>. | **Aggregate Product Name** |
| https://api.weather.com/v3/location/point?**postalKey=30339:US**\&language=en-US\&format=json\&apiKey=**yourApiKey** | v3-location-point |
| **Search by IATA Code \[location/point only\] : Required Parameters:** iataCode, language, format  Returns information for search via an IATA code like “ATL”.   | **Aggregate Product Name** |
| https://api.weather.com/v3/location/point?**iataCode=ATL**\&language=en-US\&format=json\&apiKey=**yourApiKey**  | v3-location-point |
| **Search by ICAO Code \[location/point only\]: Required Parameters:** icaoCode, language, format  Returns information for search via an ICAO code like “KATL”. | **Aggregate Product Name** |
| https://api.weather.com/v3/location/point?**icaoCode=KATL**\&language=en-US\&format=json\&apiKey=**yourApiKey** | v3-location-point |
| **Search by Place ID: Required Parameters:** placeid, language, format Returns information for search via a Place ID like “ee0214ae7fb1d265f3e4d2509e77557119b2cac9d0ac1013b593c74a1567f2b7” | **Aggregate Product Name** |
| https://api.weather.com/v3/location/point?**placeid=ee0214ae7fb1d265f3e4d2509e77557119b2cac9d0ac1013b593c74a1567f2b7**\&language=en-US\&format=json\&apiKey=**yourApiKey** | v3-location-point |
| **Search by Canonical City ID**: **Required Parameters:** locid, language, format Returns information for search via a Canonical City ID like “3e933388e3fb38c28c2d0806165bf7e3185d84bbb370417734798573ee243240”.  | **Aggregate Product Name** |
| https://api.weather.com/v3/location/point?**canonicalCityId\=3e933388e3fb38c28c2d0806165bf7e3185d84bbb370417734798573ee243240**\&language=en-US\&format=json\&apiKey=**yourApiKey** | v3-location-point |
| **Search by Location ID**: **Required Parameters:** locid, language, format Returns information for search via a Location ID like “USWY0183:1:US”.  This method of request is no longer actively supported. | **Aggregate Product Name** |
| https://api.weather.com/v3/location/point?**locid=USWY0183:1:US**\&language=en-US\&format=json\&apiKey=**yourApiKey** | v3-location-point |

### **Optional Query Parameters**

| Parameter Name | Description | Type | Range | Sample |
| :---- | :---- | :---: | :---- | :---- |
| locationType | Specify what type of location should be returned in the request.  This determines the level of granularity expected in the response.  Multiple location types may be requested as comma separated values in one request. Note: it is recommended to limit the types requested for best results See more details in the [Location Types](https://docs.google.com/document/d/17RrOJum_UNSNDAlNXEImTaU1ltElC5Y160Nm-roQ0jE/edit#heading=h.o2ibxl8fn4iy) section. |  | city, locality, neighborhood, state, address, pws, country, postal, locale (includes district, city, locality, postal, and neighborhood), locid (not actively supported) | city,locality,neighborhood |

### 

### **Location Types**

| Types | Description |
| :---- | :---- |
| country | Features that have been given a designated country code under ISO 3166-1. |
| state | Top-level sub-national administrative features.  ‘region’ is a comparable type as ‘state’. |
| district | Features that are smaller than states/regions but larger than cities. |
| city | Features generally recognized as cities, villages, municipalities, etc.  Typically locations used in postal addressing.  Most commonly used for end-user location presentation. |
| locality | Official sub-city features used in postal addressing or commonly known to local residents. |
| neighborhood | Colloquial sub-city features which often lack official administrative status and lack agreed-upon boundaries. |
| locale | A location type sub-grouping combining district, city, locality, postal, and neighborhood features.  This type has been made obsolete by the ability to request multiple location types via a comma separated list. |
| postal | Postal codes used in country-specific national addressing systems.   |
| address | Individual residential or business addresses. |
| airport | Valid, active airports.  Curated internally by The Weather Company. |
| pws | Valid, active Personal Weather Stations.  Curated internally by The Weather Company. |

### 

### **Data Elements & Definitions \- Location Service \- Point**

| Field Name | Description | Type | Range | Sample | Nulls Allowed |
| :---- | :---- | :---: | :---- | :---- | :---: |
| location | Object for location data | object |  |  |  |
| address | Locale level location detail. | string |  | Atlanta, Georgia, United States | Y |
| adminDistrict | The internationalized State, Region, District or Province identifier for ‘state’ or geopolitical area. \- level 1 administrative division. | string | Any valid state, region, district or province name. | Georgia, | Y |
| adminDistrictCode | The identifier code for ‘state’ or geopolitical area. \- level 1 administrative division. | string | US states only.  | GA, | Y |
| airportName | The airport name associated to the (ICAO / IATA) airport code.   Note: Only returned when the iataCode or iataCode query parameter is used in the request. | string | Any valid airport name. | Hartsfield-Jackson Intl, | Y |
| canonicalCityId | A city level place identifier.  The canonical city id encompasses lower level place types. Note: This value should be used for SEO purposes only. | string | Any valid canonicalCityId | b9561b6ddb213b878ba672863570cf55d936ecb80cacd7fe8c45fe9379288343 | Y |
| city | Full name of location City | string | Any valid city name. | Atlanta,  | Y |
| countyId | Governmental assigned county identifier.  Internal use only | string | Any valid County ID | FLC117, | Y |
| country | Full name of location Country | string | Any valid country name. | United States, | Y |
| countryCode | ISO Country Code | string | Any valid ISO country code. | US, | Y |
| displayName | The common display name for a location. | string | Any valid location display name. | Atlanta, | N |
| displayContext | The recommended location context to include with the displayName.  This is a recommendation from TWC and is subject to change. | \[string\] | Any free form string | GA, United States | Y |
| disputedArea | Point falls in an area with political sensitivity. | boolean | true, false | false, | N |
| disputedCountries | List of countries claiming territory in the provided disputedArea.   Note: Currently only available for disputedAreas with India, Israel, South Korea, Taiwan | \[string\] | Any valid country name. | \[“United States”,”Canada”\] | Y |
| disputedCountryCodes | List of ISO country codes representing the disputedCountries. | \[string\] | Any valid ISO country code. | \[“US”, “CA”\] | Y |
| disputedCustomers | Customer identifier for custom logic pertaining to areas with political sensitivity. | \[array\] | Any valid internal code. | \[\[“ABC”\],\[\]\] | Y |
| disputedShowCountry | Customer designation for display of country names. | \[boolean\] | true, false | true, | N |
| dmaCd | DMA Code. Internal use only. A Designated Market Area (DMA) is a group of counties in the United States that are covered by a specific group of television stations. The term was coined by Nielsen Media Research, and they control the trademark on it. There are 210 DMAs in the United States. | string | Any valid DMA Code | 534 | Y |
| dstEnd | The date time which the location ends daylight savings time observation. | ISO | Any valid ISO date time. | 2017-11-05T01:00:00-0500 | Y |
| dstStart | The date time which the location starts daylight savings time observation. | ISO | Any valid ISO date time. | 2017-03-12T03:00:00-0400 | Y |
| ianaTimeZone | The standard IANA Time Zone for the location requested. | string | Any valid IANA time zone or UTC offset. | America/New\_York, | Y |
| iataCode | The International Air Transport Association (IATA) airport codes of the requested location.  Note: Only returned when the iataCode query parameter is used in the request. | string | Any valid IATA code. | ATL, | Y |
| icaoCode | The International Civil Aviation Organization (ICAO) airport codes of the requested location.  Note: Only returned when the iacoCode query parameter is used in the request. | string | Any valid ICAO code. | KATL, | Y |
| latitude | Center latitude coordinate of the requested location. | decimal | Any valid latitude value. | 33.63, | N |
| locId | Legacy TWC location identifier corresponding to a unique location record.  This location type is provided to ensure compatibility with legacy queries. | string | Any valid locId | USWY0183:1:US | Y |
| locationCategory | A sub-type of the 'type' field that describes a more specific grouping for the location record.  Currently returns a non-null response when "type"= "poi".  In all other instances, this field will return 'null'. | string | Any valid locationCategory | national park | Y |
| longitude | Center longitude coordinate of the requested location. | decimal | Any valid longitude value. | \-84.42, | N |
| neighborhood | The recognized Neighborhood name of the requested location. | string | Any valid neighborhood name. | Eagan Park, | Y |
| placeId | A unique place identifier.   Note: A request using a placeid query parameter is expected to return the same placeid in the response body. | string | Unique Place Identifier | 25d07eca1bcda02800c1a9e699d7eb1c8132cad9bc2d6efa8a2531f0ee4a81cd | N |
| pollenId | The pollen station identifier.  Only valid in the United States. | string | Any valid pollenId | ATL | Y |
| postalCode | The Postal Code of the requested location. | string | Any valid postal code. | 30337, | Y |
| postalKey | Postal Key is a composite location identifier key of \<Postal Code\>:\<Country Code\> | string | Any valid postal key value. | 30337:US, | Y |
| pwsId | Personal Weather Station identifier | string | Any valid pwsId | KTXSANMA13, | Y |
| regionalSatellite | A TWC defined area that identifies a satellite region on the globe. | string | Any valid reginal satellite value. | se | Y |
| tideId | The tide station identifier.  Only available for locations near coastlines | string | Any valid tideId | 8729511 | Y |
| type | Geospatial definition for the location record. (location type). | string | Any valid type | city, address, poi,neighborhood, state | N |
| zoneId | Government assigned location identifier | string | Any valid zoneId | GAZ033, | Y |
| locale | Object for additional city & sub-city locale information. | object |  |  |  |
| locale1 | City or Sub-City locale information \- depending on country.  Broadest area local. (district) | string | Any valid city or sub-city locale |  | Y |
| locale2 | City or Sub-City locale information \- depending on country.  1 level more granular than ‘locale1’ (city) | string | Any valid city or sub-city locale |  | Y |
| locale3 | City or Sub-City locale information \- depending on country.  1 level more granular than ‘locale2’ (locality) | string | Any valid city or sub-city locale |  | Y |
| locale4 | City or Sub-City locale information \- depending on country.  Smallest area local. (neighborhood) | string | Any valid city or sub-city locale |  | Y |

### **JSON Sample \- Location Service \- Point**

| *// Response Collapsed for Presentation Purposes* {   "location": {     "latitude": 29.162,     "longitude": \-81.525,     "city": "Astor",     "locale": {       "locale1": "Lake County",       "locale2": "Astor",       "locale3": "Manhatten",       "locale4": null     },     "neighborhood": null,     "adminDistrict": "Florida",     "adminDistrictCode": "FL",     "postalCode": "32102",     "postalKey": "32180:US",     "country": "United States",     "countryCode": "US",     "ianaTimeZone": "America/New\_York",     "displayName": "Manhatten",     "displayContext": "Astor, United States",     "dstEnd": "2023-11-05T01:00:00-0500",     "dstStart": "2024-03-10T03:00:00-0400",     "dmaCd": "534",     "placeId": "4911fe91dedcc116bd7ebc5258a098b6bc68bca2b81c326ca329d7965af519bf",     "disputedArea": false,     "disputedCountries": null,     "disputedCountryCodes": null,     "disputedCustomers": null,     "disputedShowCountry": \[       false     \],     "canonicalCityId": "d76c26f1eef1099f8fa128c897644e4a158172913748c815d44f660548883aa5",     "countyId": "FLC069",     "locId": "USFL0018:1:US",     "locationCategory": null,     "pollenId": null,     "pwsId": "KFLASTOR16",     "regionalSatellite": "se",     "tideId": "8720832",     "type": "postal",     "zoneId": "FLZ044"     "displayContext": "Florida, Unites States"   } } |
| :---- |

[image1]: <data:image/png;base64,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>