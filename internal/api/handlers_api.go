package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) handleAPICurrent(w http.ResponseWriter, r *http.Request) {
	data, err := s.getCurrentData()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (s *Server) handleAPIHistory(w http.ResponseWriter, r *http.Request) {
	stationID := r.URL.Query().Get("station")
	if stationID == "" {
		stationID = "IWANDI23"
	}

	hours := 24
	end := time.Now()
	start := end.Add(-time.Duration(hours) * time.Hour)

	observations, err := s.store.GetObservations(stationID, start, end)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(observations)
}

func (s *Server) handleAPIAirQuality(w http.ResponseWriter, r *http.Request) {
	hours := 24
	if rawHours := r.URL.Query().Get("hours"); rawHours != "" {
		parsed, err := strconv.Atoi(rawHours)
		if err != nil || parsed <= 0 {
			http.Error(w, "invalid hours", http.StatusBadRequest)
			return
		}
		hours = parsed
	}

	end := time.Now()
	start := end.Add(-time.Duration(hours) * time.Hour)

	readings, err := s.store.GetAirQualityReadings(start, end)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(readings)
}

func (s *Server) handleAPIStations(w http.ResponseWriter, r *http.Request) {
	stations, err := s.store.GetActiveStations()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stations)
}

func (s *Server) handleAPIForecast(w http.ResponseWriter, r *http.Request) {
	data, err := s.getForecastData()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
