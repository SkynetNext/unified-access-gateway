package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds all gateway configuration
// Configuration is divided into two categories:
// 1. Business Configuration: Can be changed without affecting gateway readiness
// 2. Infrastructure Configuration: Critical for gateway operation, affects readiness
type Config struct {
	// Business Configuration
	Server    ServerConfig    `yaml:"server"`    // Listening ports, max connections
	Backends  BackendsConfig  `yaml:"backends"`  // Forwarding rules
	Lifecycle LifecycleConfig `yaml:"lifecycle"` // Shutdown timeouts

	// Infrastructure Configuration
	Metrics  MetricsConfig  `yaml:"metrics"`  // Prometheus metrics server
	Security SecurityConfig `yaml:"security"` // Redis, Auth, WAF (affects readiness)
}

// ServerConfig - Business Configuration
// Controls gateway's listening address and connection limits
type ServerConfig struct {
	ListenAddr string `yaml:"listen_addr" env:"GATEWAY_LISTEN_ADDR"` // Business: Listening port
	// Maximum concurrent connections
	MaxConnections int `yaml:"max_connections" env:"GATEWAY_MAX_CONNECTIONS"` // Business: Max online connections
}

// MetricsConfig - Infrastructure Configuration
// Prometheus metrics server configuration
// If metrics server fails, gateway continues running but monitoring is unavailable
type MetricsConfig struct {
	Enabled    bool   `yaml:"enabled" env:"METRICS_ENABLED"`           // Infrastructure: Enable metrics
	ListenAddr string `yaml:"listen_addr" env:"METRICS_LISTEN_ADDR"`    // Infrastructure: Metrics port
}

// BackendsConfig - Business Configuration
// Forwarding rules for HTTP and TCP traffic
type BackendsConfig struct {
	HTTP HTTPBackend `yaml:"http"` // Business: HTTP forwarding rules
	TCP  TCPBackend  `yaml:"tcp"`  // Business: TCP forwarding rules
}

// HTTPBackend - Business Configuration
// HTTP backend service forwarding configuration
type HTTPBackend struct {
	TargetURL string        `yaml:"target_url" env:"HTTP_BACKEND_URL"`       // Business: Backend URL
	Timeout   time.Duration `yaml:"timeout" env:"HTTP_BACKEND_TIMEOUT"`      // Business: Request timeout
}

// TCPBackend - Business Configuration
// TCP backend service forwarding configuration
type TCPBackend struct {
	TargetAddr string        `yaml:"target_addr" env:"TCP_BACKEND_ADDR"`    // Business: Backend address
	Timeout    time.Duration `yaml:"timeout" env:"TCP_BACKEND_TIMEOUT"`       // Business: Connection timeout
}

// LifecycleConfig - Business Configuration
// Graceful shutdown and drain mode configuration
type LifecycleConfig struct {
	// Graceful shutdown timeout (for draining connections)
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" env:"SHUTDOWN_TIMEOUT"` // Business: Shutdown timeout
	// Drain mode wait time (for long-lived TCP connections)
	DrainWaitTime time.Duration `yaml:"drain_wait_time" env:"DRAIN_WAIT_TIME"`     // Business: Drain wait time
}

// SecurityConfig - Infrastructure Configuration
// Security-related configuration including Redis
// If Redis is enabled but unavailable, gateway should be Running but NOT Ready
type SecurityConfig struct {
	Auth      AuthConfig      `yaml:"auth"`       // Security: Authentication config
	RateLimit RateLimitConfig `yaml:"rate_limit"` // Security: Rate limiting config
	Audit     AuditConfig     `yaml:"audit"`      // Security: Audit logging config
	WAF       WAFConfig       `yaml:"waf"`       // Security: WAF config
	Redis     RedisConfig     `yaml:"redis"`      // Infrastructure: Redis config (affects readiness)
}

// RedisConfig - Infrastructure Configuration
// Redis configuration for dynamic security config storage
// CRITICAL: If enabled but unavailable, gateway is Running but NOT Ready
// - /ready returns 503 Service Unavailable
// - /health returns 200 OK (gateway is still alive)
// - K8s removes pod from service endpoints (no traffic routed)
type RedisConfig struct {
	Enabled   bool   `yaml:"enabled" env:"REDIS_ENABLED"`         // Infrastructure: Enable Redis
	Addr      string `yaml:"addr" env:"REDIS_ADDR"`               // Infrastructure: Redis address
	Password  string `yaml:"password" env:"REDIS_PASSWORD"`       // Infrastructure: Redis password
	DB        int    `yaml:"db" env:"REDIS_DB"`                    // Infrastructure: Redis database
	KeyPrefix string `yaml:"key_prefix" env:"REDIS_KEY_PREFIX"`   // Infrastructure: Redis key prefix
}

type AuthConfig struct {
	Enabled         bool     `yaml:"enabled"`
	HeaderSubject   string   `yaml:"header_subject"`
	AllowedSubjects []string `yaml:"allowed_subjects"`
}

type RateLimitConfig struct {
	Enabled           bool    `yaml:"enabled"`
	RequestsPerSecond float64 `yaml:"requests_per_second"`
	Burst             int     `yaml:"burst"`
}

type AuditConfig struct {
	Enabled bool   `yaml:"enabled" env:"AUDIT_ENABLED"`
	Sink    string `yaml:"sink" env:"AUDIT_SINK"`
}

type WAFConfig struct {
	Enabled         bool     `yaml:"enabled"`
	BlockedIPs      []string `yaml:"blocked_ips"`
	BlockedPatterns []string `yaml:"blocked_patterns"`
}

// DefaultSecurityState returns the built-in security configuration used before Redis hydrate.
func DefaultSecurityState() SecurityConfig {
	return SecurityConfig{
		Auth: AuthConfig{
			Enabled:         false,
			HeaderSubject:   "X-Client-Subject",
			AllowedSubjects: nil,
		},
		RateLimit: RateLimitConfig{
			Enabled:           true,
			RequestsPerSecond: 100,
			Burst:             200,
		},
		Audit: AuditConfig{
			Enabled: true,
			Sink:    "stdout",
		},
		WAF: WAFConfig{
			Enabled:         false,
			BlockedIPs:      nil,
			BlockedPatterns: nil,
		},
	}
}

// LoadConfig loads INFRASTRUCTURE configuration from environment variables
// NOTE: Business config (Server, Backends, Lifecycle) is loaded from Redis, not here
func LoadConfig() *Config {
	defaultSecurity := DefaultSecurityState()
	return &Config{
		// Business Configuration - NO DEFAULTS
		// These MUST be loaded from Redis in main.go
		Server:    ServerConfig{},
		Backends:  BackendsConfig{},
		Lifecycle: LifecycleConfig{},

		// Infrastructure Configuration - Has defaults
		Metrics: MetricsConfig{
			Enabled:    getEnvBool("METRICS_ENABLED", true),
			ListenAddr: getEnv("METRICS_LISTEN_ADDR", ":9090"),
		},
		Security: SecurityConfig{
			Auth:      defaultSecurity.Auth,
			RateLimit: defaultSecurity.RateLimit,
			Audit: AuditConfig{
				Enabled: getEnvBool("AUDIT_ENABLED", defaultSecurity.Audit.Enabled),
				Sink:    getEnv("AUDIT_SINK", defaultSecurity.Audit.Sink),
			},
			WAF: defaultSecurity.WAF,
			Redis: RedisConfig{
				Enabled:   getEnvBool("REDIS_ENABLED", true),
				Addr:      getEnv("REDIS_ADDR", "localhost:6379"),
				Password:  getEnv("REDIS_PASSWORD", ""),
				DB:        getEnvInt("REDIS_DB", 0),
				KeyPrefix: getEnv("REDIS_KEY_PREFIX", "gateway:"),
			},
		},
	}
}

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if v := os.Getenv(key); v != "" {
		var result int
		fmt.Sscanf(v, "%d", &result)
		return result
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if v := os.Getenv(key); v != "" {
		return v == "true" || v == "1"
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if v := os.Getenv(key); v != "" {
		var result float64
		fmt.Sscanf(v, "%f", &result)
		return result
	}
	return defaultValue
}

func getEnvSlice(key string) []string {
	if v := os.Getenv(key); v != "" {
		parts := strings.Split(v, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	}
	return nil
}
