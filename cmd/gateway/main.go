package main

import (
	"os"
	"os/signal"
	"syscall"
	
	"github.com/SkynetNext/unified-access-gateway/internal/config"
	"github.com/SkynetNext/unified-access-gateway/internal/core"
	"github.com/SkynetNext/unified-access-gateway/pkg/xlog"
)

func main() {
	xlog.Infof("Starting Unified Access Gateway (UAG)...")

	// 1. Load Configuration (env vars override defaults)
	cfg := config.LoadConfig()
	xlog.Infof("Config loaded: listen=%s, metrics=%s", cfg.Server.ListenAddr, cfg.Metrics.ListenAddr)

	// 2. Initialize Server with configuration
	server := core.NewServer(cfg)
	
	// 3. Start Server (Non-blocking)
	server.Start()

	// 4. Wait for Shutdown Signal (SIGINT/SIGTERM from K8s)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	
	xlog.Infof("Received signal: %v. Initiating graceful shutdown...", sig)

	// 5. Execute Graceful Shutdown (Drain Mode)
	server.GracefulShutdown(cfg.Lifecycle.ShutdownTimeout)
	
	xlog.Infof("Server exited successfully.")
}
