package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/SkynetNext/unified-access-gateway/internal/config"
	"github.com/SkynetNext/unified-access-gateway/internal/core"
	"github.com/SkynetNext/unified-access-gateway/internal/discovery"
	"github.com/SkynetNext/unified-access-gateway/internal/observability"
	"github.com/SkynetNext/unified-access-gateway/pkg/xlog"
)

func main() {
	xlog.Infof("Starting Unified Access Gateway (UAG)...")

	// 1. Check if running in Kubernetes
	if discovery.IsRunningInK8s() {
		xlog.Infof("Running in Kubernetes: Pod=%s, Namespace=%s, Node=%s",
			discovery.GetPodName(),
			os.Getenv("POD_NAMESPACE"),
			discovery.GetNodeName())
	}

	// 2. Initialize Distributed Tracing (OpenTelemetry)
	jaegerEndpoint := os.Getenv("JAEGER_ENDPOINT")
	if jaegerEndpoint != "" {
		if err := observability.InitTracing("unified-access-gateway", jaegerEndpoint); err != nil {
			xlog.Errorf("Failed to initialize tracing: %v", err)
		} else {
			xlog.Infof("Distributed tracing enabled: %s", jaegerEndpoint)
		}
	}

	// 3. Load Infrastructure Configuration (env vars or ConfigMap)
	// Infrastructure config: Metrics, Redis connection settings
	cfg := config.LoadConfig()
	if discovery.IsRunningInK8s() {
		// Try to load infrastructure config from ConfigMap
		if cmCfg := config.LoadConfigFromConfigMap(); cmCfg != nil {
			cfg = cmCfg
			xlog.Infof("Infrastructure config loaded from ConfigMap")
		}
	}
	xlog.Infof("Infrastructure config loaded: metrics=%s, redis=%v", cfg.Metrics.ListenAddr, cfg.Security.Redis.Enabled)

	// 4. Initialize Service Discovery (K8s DNS)
	svcDiscovery := discovery.NewK8sServiceDiscovery()
	if discovery.IsRunningInK8s() {
		// Resolve backend services using K8s DNS
		if httpBackend := os.Getenv("HTTP_BACKEND_SERVICE"); httpBackend != "" {
			addr, err := svcDiscovery.ResolveServiceWithPort(httpBackend, 5000)
			if err == nil {
				os.Setenv("HTTP_BACKEND_URL", "http://"+addr)
				xlog.Infof("Resolved HTTP backend: %s -> %s", httpBackend, addr)
			}
		}
		if tcpBackend := os.Getenv("TCP_BACKEND_SERVICE"); tcpBackend != "" {
			addr, err := svcDiscovery.ResolveServiceWithPort(tcpBackend, 6000)
			if err == nil {
				os.Setenv("TCP_BACKEND_ADDR", addr)
				xlog.Infof("Resolved TCP backend: %s -> %s", tcpBackend, addr)
			}
		}
	}

	// 5. Initialize Redis config store (REQUIRED for business config)
	var redisStore *config.RedisStore
	if cfg.Security.Redis.Enabled {
		store, err := config.NewRedisStore(&cfg.Security.Redis)
		if err != nil {
			xlog.Errorf("CRITICAL: Failed to connect to Redis: %v", err)
			xlog.Errorf("Gateway cannot start without Redis. Business config is unavailable.")
			os.Exit(1)
		}
		redisStore = store

		// 6. Load Business Configuration from Redis (READ-ONLY)
		businessCfg, err := redisStore.LoadBusinessConfig()
		if err != nil {
			xlog.Errorf("CRITICAL: Failed to load business config from Redis: %v", err)
			xlog.Errorf("Gateway cannot start. Please configure business config in Redis first.")
			os.Exit(1)
		}

		// Apply business config to main config
		cfg.Server = businessCfg.Server
		cfg.Backends = businessCfg.Backends
		cfg.Lifecycle = businessCfg.Lifecycle
		xlog.Infof("Business config loaded from Redis: listen=%s, http_backend=%s, tcp_backend=%s",
			cfg.Server.ListenAddr, cfg.Backends.HTTP.TargetURL, cfg.Backends.TCP.TargetAddr)

		// 7. Load Security Configuration from Redis (READ-ONLY)
		securityCfg, err := redisStore.LoadSecurityConfig()
		if err != nil {
			xlog.Warnf("Failed to load security config from Redis: %v (using defaults)", err)
		} else {
			cfg.Security.Auth = securityCfg.Auth
			cfg.Security.RateLimit = securityCfg.RateLimit
			cfg.Security.WAF = securityCfg.WAF
			xlog.Infof("Security config loaded from Redis: rate_limit=%v, waf=%v",
				cfg.Security.RateLimit.Enabled, cfg.Security.WAF.Enabled)
		}
	} else {
		xlog.Errorf("CRITICAL: Redis is disabled. Gateway requires Redis for business config.")
		os.Exit(1)
	}

	// 8. Initialize Server with configuration
	server := core.NewServer(cfg, redisStore)

	// 9. Start Server (Non-blocking)
	server.Start()

	// 10. Wait for Shutdown Signal (SIGINT/SIGTERM from K8s)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	xlog.Infof("Received signal: %v. Initiating graceful shutdown...", sig)

	// 11. Execute Graceful Shutdown (Drain Mode)
	server.GracefulShutdown(cfg.Lifecycle.ShutdownTimeout)

	xlog.Infof("Server exited successfully.")
}
