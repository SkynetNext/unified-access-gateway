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

	// 3. Load Configuration (env vars or ConfigMap)
	cfg := config.LoadConfig()
	if discovery.IsRunningInK8s() {
		// Try to load from ConfigMap first
		if cmCfg := config.LoadConfigFromConfigMap(); cmCfg != nil {
			cfg = cmCfg
			xlog.Infof("Config loaded from ConfigMap")
		}
	}
	xlog.Infof("Config loaded: listen=%s, metrics=%s", cfg.Server.ListenAddr, cfg.Metrics.ListenAddr)

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

	// 5. Initialize Redis config store if enabled
	var redisStore *config.RedisStore
	if cfg.Security.Redis.Enabled {
		store, err := config.NewRedisStore(&cfg.Security.Redis)
		if err != nil {
			xlog.Errorf("Failed to initialize Redis config store: %v", err)
		} else {
			redisStore = store
		}
	}

	// 6. Initialize Server with configuration
	server := core.NewServer(cfg, redisStore)

	// 6. Start Server (Non-blocking)
	server.Start()

	// 7. Wait for Shutdown Signal (SIGINT/SIGTERM from K8s)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	xlog.Infof("Received signal: %v. Initiating graceful shutdown...", sig)

	// 8. Execute Graceful Shutdown (Drain Mode)
	server.GracefulShutdown(cfg.Lifecycle.ShutdownTimeout)

	xlog.Infof("Server exited successfully.")
}
