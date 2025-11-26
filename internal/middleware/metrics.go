package middleware

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ============================================================================
	// Request Metrics (Industry Standard: Envoy/Kong/Traefik style)
	// ============================================================================

	// RequestsTotal: Total number of requests (Counter)
	// Labels: protocol, method, status, upstream
	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_requests_total",
			Help: "Total number of requests processed by the gateway",
		},
		[]string{"protocol", "method", "status", "upstream"},
	)

	// RequestDuration: Request latency histogram (Histogram)
	// Labels: protocol, method, upstream
	// Buckets optimized for gateway latency (1ms to 10s)
	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_request_duration_seconds",
			Help:    "Request latency in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"protocol", "method", "upstream"},
	)

	// RequestBytes: Request/Response bytes (Counter)
	// Labels: protocol, direction (in/out)
	RequestBytes = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_request_bytes_total",
			Help: "Total bytes transferred (request + response)",
		},
		[]string{"protocol", "direction"},
	)

	// ============================================================================
	// Connection Metrics
	// ============================================================================

	// ActiveConnections: Current active connections (Gauge)
	// Labels: protocol
	ActiveConnections = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gateway_active_connections",
			Help: "Current number of active connections",
		},
		[]string{"protocol"},
	)

	// ConnectionsTotal: Total connections accepted (Counter)
	// Labels: protocol
	ConnectionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_connections_total",
			Help: "Total number of connections accepted",
		},
		[]string{"protocol"},
	)

	// ConnectionDuration: Connection lifetime (Histogram)
	// Labels: protocol
	ConnectionDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_connection_duration_seconds",
			Help:    "Connection lifetime in seconds",
			Buckets: []float64{1, 5, 10, 30, 60, 300, 600, 1800, 3600},
		},
		[]string{"protocol"},
	)

	// ============================================================================
	// Upstream/Backend Metrics
	// ============================================================================

	// UpstreamRequestsTotal: Requests to upstream (Counter)
	// Labels: upstream, status
	UpstreamRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_upstream_requests_total",
			Help: "Total requests sent to upstream services",
		},
		[]string{"upstream", "status"},
	)

	// UpstreamDuration: Upstream response time (Histogram)
	// Labels: upstream
	UpstreamDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_upstream_duration_seconds",
			Help:    "Upstream response time in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		},
		[]string{"upstream"},
	)

	// UpstreamHealth: Upstream health status (Gauge, 1=healthy, 0=unhealthy)
	// Labels: upstream
	UpstreamHealth = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gateway_upstream_health",
			Help: "Upstream health status (1=healthy, 0=unhealthy)",
		},
		[]string{"upstream"},
	)

	// ============================================================================
	// Security & Policy Metrics
	// ============================================================================

	// SecurityBlocksTotal: Total security blocks (Counter)
	// Labels: reason (waf, rate_limit, acl, etc.)
	SecurityBlocksTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_security_blocks_total",
			Help: "Total requests blocked by security policies",
		},
		[]string{"reason"},
	)

	// RateLimitHits: Rate limit hits (Counter)
	// Labels: limit_name
	RateLimitHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_ratelimit_hits_total",
			Help: "Total rate limit hits",
		},
		[]string{"limit_name"},
	)
)

// RecordHTTPMetrics records comprehensive HTTP request metrics
func RecordHTTPMetrics(method, status, upstream string, durationSeconds float64, bytesIn, bytesOut int64) {
	RequestsTotal.WithLabelValues("http", method, status, upstream).Inc()
	RequestDuration.WithLabelValues("http", method, upstream).Observe(durationSeconds)
	RequestBytes.WithLabelValues("http", "in").Add(float64(bytesIn))
	RequestBytes.WithLabelValues("http", "out").Add(float64(bytesOut))
}

// RecordTCPMetrics records TCP connection metrics
func RecordTCPMetrics(upstream string, durationSeconds float64, bytesIn, bytesOut int64) {
	RequestsTotal.WithLabelValues("tcp", "tcp", "success", upstream).Inc()
	RequestDuration.WithLabelValues("tcp", "tcp", upstream).Observe(durationSeconds)
	RequestBytes.WithLabelValues("tcp", "in").Add(float64(bytesIn))
	RequestBytes.WithLabelValues("tcp", "out").Add(float64(bytesOut))
}

// RecordMetrics is kept for backward compatibility
func RecordMetrics(protocol string, status string, durationSeconds float64) {
	RequestsTotal.WithLabelValues(protocol, "unknown", status, "unknown").Inc()
	RequestDuration.WithLabelValues(protocol, "unknown", "unknown").Observe(durationSeconds)
}

func IncActiveConnections(protocol string) {
	ActiveConnections.WithLabelValues(protocol).Inc()
	ConnectionsTotal.WithLabelValues(protocol).Inc()
}

func DecActiveConnections(protocol string) {
	ActiveConnections.WithLabelValues(protocol).Dec()
}

// RecordConnectionDuration records connection lifetime
func RecordConnectionDuration(protocol string, durationSeconds float64) {
	ConnectionDuration.WithLabelValues(protocol).Observe(durationSeconds)
}

// RecordUpstreamRequest records upstream request metrics
func RecordUpstreamRequest(upstream, status string, durationSeconds float64) {
	UpstreamRequestsTotal.WithLabelValues(upstream, status).Inc()
	UpstreamDuration.WithLabelValues(upstream).Observe(durationSeconds)
}

// SetUpstreamHealth sets upstream health status
func SetUpstreamHealth(upstream string, healthy bool) {
	health := 0.0
	if healthy {
		health = 1.0
	}
	UpstreamHealth.WithLabelValues(upstream).Set(health)
}

// RecordSecurityBlock records a security block event
func RecordSecurityBlock(reason string) {
	SecurityBlocksTotal.WithLabelValues(reason).Inc()
}

// RecordRateLimitHit records a rate limit hit
func RecordRateLimitHit(limitName string) {
	RateLimitHits.WithLabelValues(limitName).Inc()
}
