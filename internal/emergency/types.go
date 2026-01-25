package emergency

import "encoding/json"

// GeoJSON represents the VicEmergency events feed structure.
type GeoJSON struct {
	Type     string    `json:"type"`
	Features []Feature `json:"features"`
}

// Feature represents a single emergency event.
type Feature struct {
	Type       string     `json:"type"`
	Geometry   *Geometry  `json:"geometry"`
	Properties Properties `json:"properties"`
}

// Geometry represents GeoJSON geometry (Point, Polygon, GeometryCollection).
type Geometry struct {
	Type        string     `json:"type"`
	Coordinates Coords     `json:"coordinates,omitempty"`
	Geometries  []Geometry `json:"geometries,omitempty"`
}

// Coords handles variable coordinate formats in GeoJSON.
// For Point: [lon, lat]
// For Polygon: [[[lon, lat], ...]]
type Coords []float64

func (c *Coords) UnmarshalJSON(data []byte) error {
	// Try as simple array first (Point)
	var simple []float64
	if err := json.Unmarshal(data, &simple); err == nil {
		*c = simple
		return nil
	}

	// For polygons/complex types, we just need the first point
	// Try as nested array
	var nested [][][]float64
	if err := json.Unmarshal(data, &nested); err == nil {
		if len(nested) > 0 && len(nested[0]) > 0 && len(nested[0][0]) >= 2 {
			*c = nested[0][0]
			return nil
		}
	}

	// Return empty if can't parse
	*c = nil
	return nil
}

// Properties contains the metadata for an emergency event.
// Note: Several fields use FlexString/FlexAny because VicEmergency API 
// inconsistently returns them as either strings, numbers, or arrays 
// depending on the source.
type Properties struct {
	FeedType    string     `json:"feedType"`
	SourceOrg   string     `json:"sourceOrg"`
	SourceID    FlexString `json:"sourceId"`
	SourceFeed  string     `json:"sourceFeed"`
	SourceTitle string     `json:"sourceTitle"`
	ID          FlexString `json:"id"`
	Category1   string     `json:"category1"`
	Category2   string     `json:"category2"`
	Status      string     `json:"status"`
	Name        string     `json:"name"`
	Action      string     `json:"action"`
	Location    string     `json:"location"`
	Created     string     `json:"created"`
	Updated     string     `json:"updated"`
	WebHeadline string     `json:"webHeadline"`
	WebBody     string     `json:"webBody"`
	Text        string     `json:"text"`
	URL         string     `json:"url"`
	Size        FlexAny    `json:"size"`
	SizeFmt     FlexAny    `json:"sizeFmt"`
	Statewide   string     `json:"statewide"`

	// CAP (Common Alerting Protocol) fields
	CAP *CAPInfo `json:"cap,omitempty"`
}

// FlexAny handles JSON values that can be string, number, array, or null.
// We just ignore non-string values since we don't use these fields.
type FlexAny string

func (f *FlexAny) UnmarshalJSON(data []byte) error {
	// Try string first
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*f = FlexAny(s)
		return nil
	}
	// Ignore other types (arrays, numbers, etc.)
	*f = ""
	return nil
}

// FlexString handles JSON values that can be either string or number.
type FlexString string

func (f *FlexString) UnmarshalJSON(data []byte) error {
	// Try string first
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*f = FlexString(s)
		return nil
	}
	// Try number
	var n json.Number
	if err := json.Unmarshal(data, &n); err == nil {
		*f = FlexString(n.String())
		return nil
	}
	// Fallback - just use raw value
	*f = FlexString(string(data))
	return nil
}

// CAPInfo contains Common Alerting Protocol metadata.
type CAPInfo struct {
	Category     string `json:"category"`
	Event        string `json:"event"`
	EventCode    string `json:"eventCode"`
	Urgency      string `json:"urgency"`
	Severity     string `json:"severity"`
	Certainty    string `json:"certainty"`
	Contact      string `json:"contact"`
	SenderName   string `json:"senderName"`
	ResponseType string `json:"responseType"`
}
