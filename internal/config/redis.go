package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/SkynetNext/unified-access-gateway/pkg/xlog"
	"github.com/redis/go-redis/v9"
)

var (
	ErrRedisNotEnabled        = errors.New("redis store not enabled")
	ErrBusinessConfigNotFound = errors.New("business config not found in redis")
	ErrSecurityConfigNotFound = errors.New("security config not found in redis")
)

// RedisStore manages configuration loaded from Redis
// IMPORTANT: Gateway is READ-ONLY. All configuration writes are done by external admin tools.
type RedisStore struct {
	client  *redis.Client
	prefix  string
	ctx     context.Context
	pubsub  *redis.PubSub
	updates chan ConfigUpdate
}

// ConfigUpdate represents a configuration change notification from Redis pub/sub
type ConfigUpdate struct {
	Type string          `json:"type"` // "business", "security", "rate_limit", "waf", etc.
	Data json.RawMessage `json:"data"`
}

// NewRedisStore creates a new Redis configuration store (READ-ONLY)
func NewRedisStore(cfg *RedisConfig) (*RedisStore, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	ctx := context.Background()
	// Test connection
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	store := &RedisStore{
		client:  client,
		prefix:  cfg.KeyPrefix,
		ctx:     ctx,
		updates: make(chan ConfigUpdate, 10),
	}

	// Subscribe to configuration changes (for hot-reload)
	pubsub := client.Subscribe(ctx, store.prefix+"config:changed")
	store.pubsub = pubsub

	// Start listening for updates in background
	go store.listenUpdates()

	xlog.Infof("Redis config store initialized (READ-ONLY): addr=%s, prefix=%s", cfg.Addr, cfg.KeyPrefix)
	return store, nil
}

// listenUpdates listens for Redis pub/sub messages for config hot-reload
func (r *RedisStore) listenUpdates() {
	ch := r.pubsub.Channel()
	for msg := range ch {
		var update ConfigUpdate
		if err := json.Unmarshal([]byte(msg.Payload), &update); err != nil {
			xlog.Warnf("Failed to parse config update: %v", err)
			continue
		}
		select {
		case r.updates <- update:
			xlog.Infof("Received config update: type=%s", update.Type)
		default:
			xlog.Warnf("Config update channel full, dropping update")
		}
	}
}

// Updates returns a channel for receiving configuration updates
func (r *RedisStore) Updates() <-chan ConfigUpdate {
	if r == nil {
		return nil
	}
	return r.updates
}

// Close closes the Redis connection
func (r *RedisStore) Close() error {
	if r == nil {
		return nil
	}
	if r.pubsub != nil {
		r.pubsub.Close()
	}
	return r.client.Close()
}

// CheckHealth checks if Redis connection is healthy
func (r *RedisStore) CheckHealth() error {
	if r == nil {
		return ErrRedisNotEnabled
	}
	return r.client.Ping(r.ctx).Err()
}

// =============================================================================
// Business Configuration - READ ONLY
// =============================================================================

// BusinessConfig represents business configuration stored in Redis
// Gateway ONLY reads this, never writes. External admin tools manage this.
type BusinessConfig struct {
	Server    ServerConfig    `json:"server"`
	Backends  BackendsConfig  `json:"backends"`
	Lifecycle LifecycleConfig `json:"lifecycle"`
}

// LoadBusinessConfig loads business configuration from Redis
// Returns error if Redis is unavailable or config is missing
// Gateway will NOT start listener if this fails
func (r *RedisStore) LoadBusinessConfig() (*BusinessConfig, error) {
	if r == nil {
		return nil, ErrRedisNotEnabled
	}

	key := r.prefix + "business:config"
	exists, err := r.client.Exists(r.ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to check business config: %w", err)
	}
	if exists == 0 {
		return nil, ErrBusinessConfigNotFound
	}

	// Load config from Redis hash
	result, err := r.client.HGetAll(r.ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to load business config: %w", err)
	}

	cfg := &BusinessConfig{}

	// Server config
	if v, ok := result["server.listen_addr"]; ok && v != "" {
		cfg.Server.ListenAddr = v
	}
	if v, ok := result["server.max_connections"]; ok && v != "" {
		fmt.Sscanf(v, "%d", &cfg.Server.MaxConnections)
	}

	// HTTP Backend
	if v, ok := result["backends.http.target_url"]; ok && v != "" {
		cfg.Backends.HTTP.TargetURL = v
	}
	if v, ok := result["backends.http.timeout"]; ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Backends.HTTP.Timeout = d
		}
	}

	// TCP Backend
	if v, ok := result["backends.tcp.target_addr"]; ok && v != "" {
		cfg.Backends.TCP.TargetAddr = v
	}
	if v, ok := result["backends.tcp.timeout"]; ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Backends.TCP.Timeout = d
		}
	}

	// Lifecycle config
	if v, ok := result["lifecycle.shutdown_timeout"]; ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Lifecycle.ShutdownTimeout = d
		}
	}
	if v, ok := result["lifecycle.drain_wait_time"]; ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Lifecycle.DrainWaitTime = d
		}
	}

	return cfg, nil
}

// =============================================================================
// Security Configuration - READ ONLY
// =============================================================================

// LoadSecurityConfig loads security configuration from Redis
// Gateway ONLY reads this, never writes. External admin tools manage this.
func (r *RedisStore) LoadSecurityConfig() (*SecurityConfig, error) {
	if r == nil {
		return nil, ErrRedisNotEnabled
	}

	cfg := DefaultSecurityState()

	// Load Auth config
	if authCfg, err := r.client.HGetAll(r.ctx, r.prefix+"auth:config").Result(); err == nil && len(authCfg) > 0 {
		if v, ok := authCfg["enabled"]; ok {
			cfg.Auth.Enabled = v == "1" || v == "true"
		}
		if v, ok := authCfg["header_subject"]; ok && v != "" {
			cfg.Auth.HeaderSubject = v
		}
	}

	// Load allowed subjects
	if subjects, err := r.client.SMembers(r.ctx, r.prefix+"auth:allowed_subjects").Result(); err == nil {
		cfg.Auth.AllowedSubjects = subjects
	}

	// Load Rate Limit config
	if rateCfg, err := r.client.HGetAll(r.ctx, r.prefix+"rate_limit").Result(); err == nil && len(rateCfg) > 0 {
		if v, ok := rateCfg["enabled"]; ok {
			cfg.RateLimit.Enabled = v == "1" || v == "true"
		}
		if v, ok := rateCfg["rps"]; ok && v != "" {
			fmt.Sscanf(v, "%f", &cfg.RateLimit.RequestsPerSecond)
		}
		if v, ok := rateCfg["burst"]; ok && v != "" {
			fmt.Sscanf(v, "%d", &cfg.RateLimit.Burst)
		}
	}

	// Load WAF config
	if wafCfg, err := r.client.HGetAll(r.ctx, r.prefix+"waf:config").Result(); err == nil && len(wafCfg) > 0 {
		if v, ok := wafCfg["enabled"]; ok {
			cfg.WAF.Enabled = v == "1" || v == "true"
		}
	}

	// Load blocked IPs (using Set for atomic add/remove without overwrite)
	if ips, err := r.client.SMembers(r.ctx, r.prefix+"waf:blocked_ips").Result(); err == nil {
		cfg.WAF.BlockedIPs = ips
	}

	// Load blocked patterns (using Set for atomic add/remove without overwrite)
	if patterns, err := r.client.SMembers(r.ctx, r.prefix+"waf:blocked_patterns").Result(); err == nil {
		cfg.WAF.BlockedPatterns = patterns
	}

	return &cfg, nil
}
