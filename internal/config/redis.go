package config

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/SkynetNext/unified-access-gateway/pkg/xlog"
	"github.com/redis/go-redis/v9"
)

// RedisStore manages dynamic security configuration in Redis
type RedisStore struct {
	client  *redis.Client
	prefix  string
	ctx     context.Context
	pubsub  *redis.PubSub
	updates chan ConfigUpdate
}

type ConfigUpdate struct {
	Type string          `json:"type"` // "rate_limit", "waf_ips", "waf_patterns", "auth_subjects"
	Data json.RawMessage `json:"data"`
}

// NewRedisStore creates a new Redis configuration store
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

	// Subscribe to configuration changes
	pubsub := client.Subscribe(ctx, store.prefix+"config:changed")
	store.pubsub = pubsub

	// Start listening for updates in background
	go store.listenUpdates()

	xlog.Infof("Redis config store initialized: addr=%s, prefix=%s", cfg.Addr, cfg.KeyPrefix)
	return store, nil
}

// listenUpdates listens for Redis pub/sub messages
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

func (r *RedisStore) keyExists(key string) (bool, error) {
	if r == nil {
		return false, fmt.Errorf("Redis store not enabled")
	}
	count, err := r.client.Exists(r.ctx, key).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Rate Limit Operations

func (r *RedisStore) GetRateLimit() (enabled bool, rps float64, burst int, err error) {
	if r == nil {
		return false, 0, 0, fmt.Errorf("Redis store not enabled")
	}

	enabledStr := r.client.HGet(r.ctx, r.prefix+"rate_limit", "enabled").Val()
	rpsStr := r.client.HGet(r.ctx, r.prefix+"rate_limit", "rps").Val()
	burstStr := r.client.HGet(r.ctx, r.prefix+"rate_limit", "burst").Val()

	enabled = enabledStr == "1" || enabledStr == "true"
	if rpsStr != "" {
		fmt.Sscanf(rpsStr, "%f", &rps)
	}
	if burstStr != "" {
		fmt.Sscanf(burstStr, "%d", &burst)
	}

	return enabled, rps, burst, nil
}

func (r *RedisStore) SetRateLimit(enabled bool, rps float64, burst int) error {
	if r == nil {
		return fmt.Errorf("Redis store not enabled")
	}

	pipe := r.client.Pipeline()
	pipe.HSet(r.ctx, r.prefix+"rate_limit", "enabled", enabled)
	pipe.HSet(r.ctx, r.prefix+"rate_limit", "rps", rps)
	pipe.HSet(r.ctx, r.prefix+"rate_limit", "burst", burst)
	_, err := pipe.Exec(r.ctx)
	if err != nil {
		return err
	}

	// Publish change notification
	r.publishChange("rate_limit", map[string]interface{}{
		"enabled": enabled,
		"rps":     rps,
		"burst":   burst,
	})
	return nil
}

// WAF IP Operations

func (r *RedisStore) GetBlockedIPs() ([]string, error) {
	if r == nil {
		return nil, fmt.Errorf("Redis store not enabled")
	}

	members := r.client.SMembers(r.ctx, r.prefix+"waf:blocked_ips").Val()
	return members, nil
}

func (r *RedisStore) AddBlockedIPs(ips []string) error {
	if r == nil {
		return fmt.Errorf("Redis store not enabled")
	}

	if len(ips) == 0 {
		return nil
	}

	pipe := r.client.Pipeline()
	for _, ip := range ips {
		pipe.SAdd(r.ctx, r.prefix+"waf:blocked_ips", ip)
	}
	_, err := pipe.Exec(r.ctx)
	if err != nil {
		return err
	}

	// Publish change notification
	r.publishChange("waf_ips", map[string]interface{}{
		"action": "add",
		"ips":    ips,
	})
	return nil
}

func (r *RedisStore) RemoveBlockedIPs(ips []string) error {
	if r == nil {
		return fmt.Errorf("Redis store not enabled")
	}

	if len(ips) == 0 {
		return nil
	}

	pipe := r.client.Pipeline()
	for _, ip := range ips {
		pipe.SRem(r.ctx, r.prefix+"waf:blocked_ips", ip)
	}
	_, err := pipe.Exec(r.ctx)
	if err != nil {
		return err
	}

	// Publish change notification
	r.publishChange("waf_ips", map[string]interface{}{
		"action": "remove",
		"ips":    ips,
	})
	return nil
}

// WAF Pattern Operations

func (r *RedisStore) GetBlockedPatterns() ([]string, error) {
	if r == nil {
		return nil, fmt.Errorf("Redis store not enabled")
	}

	patterns := r.client.LRange(r.ctx, r.prefix+"waf:patterns", 0, -1).Val()
	return patterns, nil
}

func (r *RedisStore) AddBlockedPatterns(patterns []string) error {
	if r == nil {
		return fmt.Errorf("Redis store not enabled")
	}

	if len(patterns) == 0 {
		return nil
	}

	pipe := r.client.Pipeline()
	for _, pattern := range patterns {
		pipe.LPush(r.ctx, r.prefix+"waf:patterns", pattern)
	}
	_, err := pipe.Exec(r.ctx)
	if err != nil {
		return err
	}

	// Publish change notification
	r.publishChange("waf_patterns", map[string]interface{}{
		"action":   "add",
		"patterns": patterns,
	})
	return nil
}

func (r *RedisStore) RemoveBlockedPatterns(patterns []string) error {
	if r == nil {
		return fmt.Errorf("Redis store not enabled")
	}

	if len(patterns) == 0 {
		return nil
	}

	pipe := r.client.Pipeline()
	for _, pattern := range patterns {
		pipe.LRem(r.ctx, r.prefix+"waf:patterns", 0, pattern)
	}
	_, err := pipe.Exec(r.ctx)
	if err != nil {
		return err
	}

	// Publish change notification
	r.publishChange("waf_patterns", map[string]interface{}{
		"action":   "remove",
		"patterns": patterns,
	})
	return nil
}

// Auth Subject Operations

func (r *RedisStore) GetAllowedSubjects() ([]string, error) {
	if r == nil {
		return nil, fmt.Errorf("Redis store not enabled")
	}

	members := r.client.SMembers(r.ctx, r.prefix+"auth:allowed_subjects").Val()
	return members, nil
}

func (r *RedisStore) AddAllowedSubjects(subjects []string) error {
	if r == nil {
		return fmt.Errorf("Redis store not enabled")
	}

	if len(subjects) == 0 {
		return nil
	}

	pipe := r.client.Pipeline()
	for _, subject := range subjects {
		pipe.SAdd(r.ctx, r.prefix+"auth:allowed_subjects", subject)
	}
	_, err := pipe.Exec(r.ctx)
	if err != nil {
		return err
	}

	// Publish change notification
	r.publishChange("auth_subjects", map[string]interface{}{
		"action":   "add",
		"subjects": subjects,
	})
	return nil
}

func (r *RedisStore) RemoveAllowedSubjects(subjects []string) error {
	if r == nil {
		return fmt.Errorf("Redis store not enabled")
	}

	if len(subjects) == 0 {
		return nil
	}

	pipe := r.client.Pipeline()
	for _, subject := range subjects {
		pipe.SRem(r.ctx, r.prefix+"auth:allowed_subjects", subject)
	}
	_, err := pipe.Exec(r.ctx)
	if err != nil {
		return err
	}

	// Publish change notification
	r.publishChange("auth_subjects", map[string]interface{}{
		"action":   "remove",
		"subjects": subjects,
	})
	return nil
}

// publishChange publishes a configuration change notification
func (r *RedisStore) publishChange(changeType string, data interface{}) {
	raw, err := json.Marshal(data)
	if err != nil {
		xlog.Warnf("Failed to marshal config update data: %v", err)
		return
	}
	update := ConfigUpdate{
		Type: changeType,
		Data: raw,
	}
	payload, err := json.Marshal(update)
	if err != nil {
		xlog.Warnf("Failed to marshal config update: %v", err)
		return
	}
	r.client.Publish(r.ctx, r.prefix+"config:changed", payload)
}

// LoadAllFromRedis loads all security configuration from Redis
func (r *RedisStore) LoadAllFromRedis() (*SecurityConfig, error) {
	if r == nil {
		return nil, nil
	}

	cfg := &SecurityConfig{}
	found := false

	if exists, err := r.keyExists(r.prefix + "rate_limit"); err == nil && exists {
		enabled, rps, burst, err := r.GetRateLimit()
		if err != nil {
			return nil, err
		}
		cfg.RateLimit = RateLimitConfig{
			Enabled:           enabled,
			RequestsPerSecond: rps,
			Burst:             burst,
		}
		found = true
	} else if err != nil {
		return nil, err
	}

	if exists, err := r.keyExists(r.prefix + "waf:blocked_ips"); err == nil && exists {
		ips, err := r.GetBlockedIPs()
		if err != nil {
			return nil, err
		}
		cfg.WAF.BlockedIPs = ips
		found = true
	} else if err != nil {
		return nil, err
	}

	if exists, err := r.keyExists(r.prefix + "waf:patterns"); err == nil && exists {
		patterns, err := r.GetBlockedPatterns()
		if err != nil {
			return nil, err
		}
		cfg.WAF.BlockedPatterns = patterns
		found = true
	} else if err != nil {
		return nil, err
	}

	if exists, err := r.keyExists(r.prefix + "auth:allowed_subjects"); err == nil && exists {
		subjects, err := r.GetAllowedSubjects()
		if err != nil {
			return nil, err
		}
		cfg.Auth.AllowedSubjects = subjects
		found = true
	} else if err != nil {
		return nil, err
	}

	if !found {
		return nil, nil
	}

	return cfg, nil
}

// SyncToRedis syncs current config to Redis (for initial setup)
func (r *RedisStore) SyncToRedis(cfg *SecurityConfig) error {
	if r == nil {
		return nil
	}

	// Sync rate limit
	if err := r.SetRateLimit(
		cfg.RateLimit.Enabled,
		cfg.RateLimit.RequestsPerSecond,
		cfg.RateLimit.Burst,
	); err != nil {
		xlog.Warnf("Failed to sync rate limit to Redis: %v", err)
	}

	// Sync WAF IPs
	if len(cfg.WAF.BlockedIPs) > 0 {
		if err := r.AddBlockedIPs(cfg.WAF.BlockedIPs); err != nil {
			xlog.Warnf("Failed to sync WAF IPs to Redis: %v", err)
		}
	}

	// Sync WAF patterns
	if len(cfg.WAF.BlockedPatterns) > 0 {
		if err := r.AddBlockedPatterns(cfg.WAF.BlockedPatterns); err != nil {
			xlog.Warnf("Failed to sync WAF patterns to Redis: %v", err)
		}
	}

	// Sync auth subjects
	if len(cfg.Auth.AllowedSubjects) > 0 {
		if err := r.AddAllowedSubjects(cfg.Auth.AllowedSubjects); err != nil {
			xlog.Warnf("Failed to sync auth subjects to Redis: %v", err)
		}
	}

	return nil
}
