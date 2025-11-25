package core

import (
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
	cfg        *config.Config
	listener   *Listener
	draining   int32 // Atomic: 0=Running, 1=Draining
	wg         sync.WaitGroup
	security   *security.Manager
	redisStore *config.RedisStore
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
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			mux := http.NewServeMux()
			mux.Handle("/metrics", promhttp.Handler())
			mux.HandleFunc("/health", s.healthHandler)
			mux.HandleFunc("/ready", s.readyHandler) // K8s Readiness Probe

			xlog.Infof("Metrics server listening on %s", s.cfg.Metrics.ListenAddr)
			if err := http.ListenAndServe(s.cfg.Metrics.ListenAddr, mux); err != nil {
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
	xlog.Infof("Waiting for K8s to deregister endpoints...")
	time.Sleep(5 * time.Second)

	// 3. Stop Listener (Stop accepting new TCP connections)
	s.listener.Stop()

	// 4. Wait for active connections to drain
	// In production, use sync.WaitGroup to track active connections
	// For long-lived gaming connections, this could be hours (configured via DRAIN_WAIT_TIME)
	xlog.Infof("Waiting for active connections to drain (Timeout: %v)...", timeout)
	time.Sleep(5 * time.Second) // Mock wait - in production, wait on WaitGroup with timeout

	// 5. Wait for all goroutines to finish
	s.wg.Wait()
	if s.redisStore != nil {
		if err := s.redisStore.Close(); err != nil {
			xlog.Warnf("Failed to close Redis store: %v", err)
		}
	}
	xlog.Infof("All goroutines finished. Shutdown complete.")
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
