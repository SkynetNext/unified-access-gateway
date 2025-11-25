package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds all gateway configuration
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Metrics   MetricsConfig   `yaml:"metrics"`
	Backends  BackendsConfig  `yaml:"backends"`
	Lifecycle LifecycleConfig `yaml:"lifecycle"`
	Security  SecurityConfig  `yaml:"security"`
}

type ServerConfig struct {
	ListenAddr string `yaml:"listen_addr" env:"GATEWAY_LISTEN_ADDR"`
	// Maximum concurrent connections
	MaxConnections int `yaml:"max_connections" env:"GATEWAY_MAX_CONNECTIONS"`
}

type MetricsConfig struct {
	Enabled    bool   `yaml:"enabled" env:"METRICS_ENABLED"`
	ListenAddr string `yaml:"listen_addr" env:"METRICS_LISTEN_ADDR"`
}

type BackendsConfig struct {
	HTTP HTTPBackend `yaml:"http"`
	TCP  TCPBackend  `yaml:"tcp"`
}

type HTTPBackend struct {
	TargetURL string        `yaml:"target_url" env:"HTTP_BACKEND_URL"`
	Timeout   time.Duration `yaml:"timeout" env:"HTTP_BACKEND_TIMEOUT"`
}

type TCPBackend struct {
	TargetAddr string        `yaml:"target_addr" env:"TCP_BACKEND_ADDR"`
	Timeout    time.Duration `yaml:"timeout" env:"TCP_BACKEND_TIMEOUT"`
}

type LifecycleConfig struct {
	// Graceful shutdown timeout (for draining connections)
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" env:"SHUTDOWN_TIMEOUT"`
	// Drain mode wait time (for long-lived TCP connections)
	DrainWaitTime time.Duration `yaml:"drain_wait_time" env:"DRAIN_WAIT_TIME"`
}

type SecurityConfig struct {
	Auth      AuthConfig      `yaml:"auth"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
	Audit     AuditConfig     `yaml:"audit"`
	WAF       WAFConfig       `yaml:"waf"`
	Redis     RedisConfig     `yaml:"redis"`
}

type RedisConfig struct {
	Enabled   bool   `yaml:"enabled" env:"REDIS_ENABLED"`
	Addr      string `yaml:"addr" env:"REDIS_ADDR"`
	Password  string `yaml:"password" env:"REDIS_PASSWORD"`
	DB        int    `yaml:"db" env:"REDIS_DB"`
	KeyPrefix string `yaml:"key_prefix" env:"REDIS_KEY_PREFIX"`
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

// LoadConfig loads configuration from environment variables with defaults
func LoadConfig() *Config {
	defaultSecurity := DefaultSecurityState()
	return &Config{
		Server: ServerConfig{
			ListenAddr:     getEnv("GATEWAY_LISTEN_ADDR", ":8080"),
			MaxConnections: getEnvInt("GATEWAY_MAX_CONNECTIONS", 10000),
		},
		Metrics: MetricsConfig{
			Enabled:    getEnvBool("METRICS_ENABLED", true),
			ListenAddr: getEnv("METRICS_LISTEN_ADDR", ":9090"),
		},
		Backends: BackendsConfig{
			HTTP: HTTPBackend{
				TargetURL: getEnv("HTTP_BACKEND_URL", "http://localhost:5000"),
				Timeout:   getEnvDuration("HTTP_BACKEND_TIMEOUT", 30*time.Second),
			},
			TCP: TCPBackend{
				TargetAddr: getEnv("TCP_BACKEND_ADDR", "localhost:6000"),
				Timeout:    getEnvDuration("TCP_BACKEND_TIMEOUT", 60*time.Second),
			},
		},
		Lifecycle: LifecycleConfig{
			ShutdownTimeout: getEnvDuration("SHUTDOWN_TIMEOUT", 60*time.Second),
			DrainWaitTime:   getEnvDuration("DRAIN_WAIT_TIME", 3600*time.Second), // 1 hour for gaming
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
