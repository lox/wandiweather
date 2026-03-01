package main

import (
	"reflect"
	"testing"

	"github.com/lox/wandiweather/internal/models"
)

func TestActiveStationIDs(t *testing.T) {
	t.Parallel()

	stations := []models.Station{
		{StationID: "A", Active: true},
		{StationID: "B", Active: false},
		{StationID: "C", Active: true},
	}

	got := activeStationIDs(stations)
	want := []string{"A", "C"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("activeStationIDs() = %v, want %v", got, want)
	}
}
