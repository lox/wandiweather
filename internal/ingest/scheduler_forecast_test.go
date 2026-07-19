package ingest

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIngestBOMHourlyForecastsMarksStorageFailuresUnsuccessful(t *testing.T) {
	forecastTime := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Hour)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"data": [{
				"time": %q,
				"relative_humidity": 101
			}]
		}`, forecastTime.Format(time.RFC3339))
	}))
	t.Cleanup(server.Close)

	_, forecastStore, _ := setupDailyJobsTest(t)
	client := NewBOMHourlyAPIClient("r3811m")
	client.baseURL = server.URL
	client.client = server.Client()
	scheduler := &Scheduler{
		store:        forecastStore,
		bomHourlyAPI: client,
	}

	scheduler.ingestBOMHourlyForecasts()

	runs, err := forecastStore.GetRecentIngestErrors(10)
	if err != nil {
		t.Fatalf("GetRecentIngestErrors: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("failed ingest runs = %d, want 1", len(runs))
	}
	if runs[0].Source != bomHourlyAPISource || !runs[0].ErrorMessage.Valid {
		t.Fatalf("failed ingest run = %+v", runs[0])
	}
}
