package config

import (
	"fmt"
	"os"
	"time"
)

// Config holds all gateway configuration
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Metrics   MetricsConfig   `yaml:"metrics"`
	Backends  BackendsConfig  `yaml:"backends"`
	Lifecycle LifecycleConfig `yaml:"lifecycle"`
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

// LoadConfig loads configuration from environment variables with defaults
func LoadConfig() *Config {
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
