package healthcheck

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/SkynetNext/unified-access-gateway/internal/config"
	"github.com/SkynetNext/unified-access-gateway/internal/middleware"
	"github.com/SkynetNext/unified-access-gateway/pkg/xlog"
)

// UpstreamHealthChecker periodically checks the health of upstream backends
type UpstreamHealthChecker struct {
	cfg        *config.Config
	httpClient *http.Client
	tcpTimeout time.Duration
	interval   time.Duration
	stopChan   chan struct{}
	wg         sync.WaitGroup
	mu         sync.RWMutex
	healthMap  map[string]bool // upstream -> healthy
}

// NewUpstreamHealthChecker creates a new health checker
func NewUpstreamHealthChecker(cfg *config.Config) *UpstreamHealthChecker {
	return &UpstreamHealthChecker{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		tcpTimeout: 5 * time.Second,
		interval:   30 * time.Second, // Check every 30 seconds
		stopChan:   make(chan struct{}),
		healthMap:  make(map[string]bool),
	}
}

// Start begins periodic health checking
func (c *UpstreamHealthChecker) Start() {
	c.wg.Add(1)
	go c.run()
	xlog.Infof("Upstream health checker started (interval: %v)", c.interval)
}

// Stop stops the health checker
func (c *UpstreamHealthChecker) Stop() {
	close(c.stopChan)
	c.wg.Wait()
	xlog.Infof("Upstream health checker stopped")
}

// IsHealthy returns the health status of an upstream
func (c *UpstreamHealthChecker) IsHealthy(upstream string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.healthMap[upstream]
}

// run performs periodic health checks
func (c *UpstreamHealthChecker) run() {
	defer c.wg.Done()

	// Initial check
	c.checkAll()

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.checkAll()
		case <-c.stopChan:
			return
		}
	}
}

// checkAll checks all configured upstreams
func (c *UpstreamHealthChecker) checkAll() {
	// Check HTTP backend
	if c.cfg.Backends.HTTP.TargetURL != "" {
		healthy := c.checkHTTP(c.cfg.Backends.HTTP.TargetURL)
		c.updateHealth(c.cfg.Backends.HTTP.TargetURL, healthy)
	}

	// Check TCP backend
	if c.cfg.Backends.TCP.TargetAddr != "" {
		healthy := c.checkTCP(c.cfg.Backends.TCP.TargetAddr)
		c.updateHealth(c.cfg.Backends.TCP.TargetAddr, healthy)
	}
}

// checkHTTP checks HTTP backend health
func (c *UpstreamHealthChecker) checkHTTP(url string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		xlog.Debugf("Health check: failed to create HTTP request for %s: %v", url, err)
		return false
	}

	// Try to hit a health endpoint, or just check if the connection works
	// If the URL doesn't have a path, try /health or / as fallback
	resp, err := c.httpClient.Do(req)
	if err != nil {
		xlog.Debugf("Health check: HTTP backend %s is unhealthy: %v", url, err)
		return false
	}
	resp.Body.Close()

	// Consider 2xx and 3xx as healthy
	healthy := resp.StatusCode >= 200 && resp.StatusCode < 400
	if !healthy {
		xlog.Debugf("Health check: HTTP backend %s returned status %d", url, resp.StatusCode)
	}
	return healthy
}

// checkTCP checks TCP backend health
func (c *UpstreamHealthChecker) checkTCP(addr string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), c.tcpTimeout)
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		xlog.Debugf("Health check: TCP backend %s is unhealthy: %v", addr, err)
		return false
	}
	conn.Close()
	return true
}

// updateHealth updates the health status and metrics
func (c *UpstreamHealthChecker) updateHealth(upstream string, healthy bool) {
	c.mu.Lock()
	oldHealthy := c.healthMap[upstream]
	c.healthMap[upstream] = healthy
	c.mu.Unlock()

	// Update Prometheus metric
	middleware.SetUpstreamHealth(upstream, healthy)

	// Log status changes
	if oldHealthy != healthy {
		if healthy {
			xlog.Infof("Upstream %s is now healthy", upstream)
		} else {
			xlog.Warnf("Upstream %s is now unhealthy", upstream)
		}
	}
}
