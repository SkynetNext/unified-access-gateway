package middleware

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// RequestsTotal: Counter for total requests
	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_requests_total",
			Help: "The total number of processed requests",
		},
		[]string{"protocol", "status"},
	)

	// RequestDuration: Histogram for request latency
	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_request_duration_seconds",
			Help:    "The duration of requests",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"protocol"},
	)

	// ActiveConnections: Gauge for current active connections
	ActiveConnections = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gateway_active_connections",
			Help: "Current number of active connections",
		},
		[]string{"protocol"},
	)
)

func RecordMetrics(protocol string, status string, durationSeconds float64) {
	RequestsTotal.WithLabelValues(protocol, status).Inc()
	RequestDuration.WithLabelValues(protocol).Observe(durationSeconds)
}

func IncActiveConnections(protocol string) {
	ActiveConnections.WithLabelValues(protocol).Inc()
}

func DecActiveConnections(protocol string) {
	ActiveConnections.WithLabelValues(protocol).Dec()
}
