package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	PWSAPICallsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "wandiweather_pws_api_calls_total",
			Help: "Total Weather Underground PWS API calls",
		},
		[]string{"station", "endpoint", "status"},
	)

	PWSAPILatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "wandiweather_pws_api_latency_seconds",
			Help:    "PWS API call latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"station", "endpoint"},
	)

	ObservationsIngested = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "wandiweather_observations_ingested_total",
			Help: "Total observations successfully ingested",
		},
		[]string{"station"},
	)

	ForecastsIngested = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "wandiweather_forecasts_ingested_total",
			Help: "Total forecasts successfully ingested",
		},
		[]string{"station"},
	)
)
