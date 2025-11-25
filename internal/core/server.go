package core

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SkynetNext/unified-access-gateway/internal/config"
	"github.com/SkynetNext/unified-access-gateway/internal/security"
	"github.com/SkynetNext/unified-access-gateway/pkg/xlog"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Server struct {
	cfg          *config.Config
	listener     *Listener
	draining     int32 // Atomic: 0=Running, 1=Draining
	wg           sync.WaitGroup
	security     *security.Manager
	redisStore   *config.RedisStore
	metricsServer *http.Server // For graceful shutdown
}

func NewServer(cfg *config.Config, store *config.RedisStore) *Server {
	sec := security.NewManager(cfg, store)
	return &Server{
		cfg:        cfg,
		listener:   NewListener(cfg, sec),
		security:   sec,
		redisStore: store,
	}
}

func (s *Server) Start() {
	// 1. Start Metrics Server (if enabled)
	if s.cfg.Metrics.Enabled {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		mux.HandleFunc("/health", s.healthHandler)
		mux.HandleFunc("/ready", s.readyHandler) // K8s Readiness Probe

		s.metricsServer = &http.Server{
			Addr:    s.cfg.Metrics.ListenAddr,
			Handler: mux,
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			xlog.Infof("Metrics server listening on %s", s.cfg.Metrics.ListenAddr)
			if err := s.metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				xlog.Errorf("Metrics server error: %v", err)
			}
		}()
	}

	// 2. Start Business Listener
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.listener.Start(); err != nil {
			xlog.Errorf("Failed to start listener: %v", err)
		}
	}()
}

// GracefulShutdown handles the shutdown process
func (s *Server) GracefulShutdown(timeout time.Duration) {
	xlog.Infof("Entering Drain Mode...")

	// 1. Mark as Draining
	// This causes /ready to return 503, prompting K8s to remove this pod from endpoints
	atomic.StoreInt32(&s.draining, 1)

	// 2. Wait for K8s endpoints propagation (usually 5-10s)
	// Use shorter wait if timeout is small
	// NOTE: Metrics server stays running during this time for K8s probes
	k8sWaitTime := 5 * time.Second
	if timeout < 10*time.Second {
		k8sWaitTime = 2 * time.Second // Shorter wait for quick shutdowns
	}
	xlog.Infof("Waiting for K8s to deregister endpoints (%v)...", k8sWaitTime)
	xlog.Infof("Metrics server remains available for /health and /ready probes during shutdown")
	time.Sleep(k8sWaitTime)

	// 3. Stop Listener (Stop accepting new TCP connections)
	// Metrics server still running for monitoring and probes
	s.listener.Stop()

	// 4. Wait for active connections to drain
	// Calculate remaining time for connection drain
	// Metrics server remains available for monitoring and probes during this time
	remainingTime := timeout - k8sWaitTime
	if remainingTime < 0 {
		remainingTime = 0
	}
	
	if remainingTime > 0 {
		xlog.Infof("Waiting for active connections to drain (Timeout: %v)...", remainingTime)
		xlog.Infof("Metrics server remains available for /health and /ready probes during drain")
		time.Sleep(remainingTime)
	} else {
		xlog.Infof("No time remaining for connection drain")
	}

	// 5. Stop Metrics Server (graceful shutdown) - LAST to close
	// This allows monitoring and probes to work during entire shutdown process
	// After this, metrics server goroutine will complete, and s.wg.Wait() can finish
	if s.metricsServer != nil {
		xlog.Infof("Shutting down metrics server (last step)...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.metricsServer.Shutdown(ctx); err != nil {
			xlog.Warnf("Metrics server shutdown error: %v", err)
		}
	}

	// 6. Wait for all goroutines to finish
	// Listener goroutine already finished (acceptLoop exited after Stop())
	// Metrics server goroutine will finish after Shutdown()
	xlog.Infof("Waiting for all goroutines to finish...")
	s.wg.Wait()

	// 7. Close Redis store (final cleanup)
	// All services are stopped, now close external connections
	if s.redisStore != nil {
		if err := s.redisStore.Close(); err != nil {
			xlog.Warnf("Failed to close Redis store: %v", err)
		}
	}
	xlog.Infof("Shutdown complete.")
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// readyHandler for K8s Readiness Probe
// Returns 503 if:
// 1. Gateway is in drain mode (shutting down)
// 2. Redis is enabled but unavailable (business config cannot be loaded)
func (s *Server) readyHandler(w http.ResponseWriter, r *http.Request) {
	// Check 1: Drain mode
	if atomic.LoadInt32(&s.draining) == 1 {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Draining"))
		return
	}

	// Check 2: Redis health (if enabled)
	if s.cfg.Security.Redis.Enabled && s.redisStore != nil {
		if err := s.redisStore.CheckHealth(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Redis Unavailable: " + err.Error()))
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Ready"))
}
