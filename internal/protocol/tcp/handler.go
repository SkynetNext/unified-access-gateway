package tcp

import (
	"io"
	"net"
	"time"

	"github.com/SkynetNext/unified-access-gateway/internal/config"
	"github.com/SkynetNext/unified-access-gateway/internal/middleware"
	"github.com/SkynetNext/unified-access-gateway/internal/security"
	"github.com/SkynetNext/unified-access-gateway/pkg/ebpf"
	"github.com/SkynetNext/unified-access-gateway/pkg/xlog"
)

type Handler struct {
	backendAddr string
	sockMapMgr  *ebpf.SockMapManager
	ebpfEnabled bool
	security    *security.Manager
}

func NewHandler(cfg *config.Config, sec *security.Manager) *Handler {
	addr := cfg.Backends.TCP.TargetAddr
	if addr == "" {
		// Business config MUST be loaded from Redis, no fallback
		xlog.Errorf("CRITICAL: backends.tcp.target_addr is not configured (must be set in Redis)")
		return nil
	}

	h := &Handler{
		backendAddr: addr,
		security:    sec,
	}

	// Try to initialize eBPF SockMap (optional, graceful fallback)
	mgr, err := ebpf.NewSockMapManager()
	if err != nil {
		xlog.Infof("eBPF SockMap initialization failed (falling back to userspace): %v", err)
		h.ebpfEnabled = false
	} else {
		h.sockMapMgr = mgr
		h.ebpfEnabled = mgr.IsEnabled()
		if h.ebpfEnabled {
			xlog.Infof("eBPF SockMap acceleration enabled")
			// Try to attach to cgroup (optional, improves performance)
			// Empty string triggers auto-detection
			if err := mgr.AttachToCgroup(""); err != nil {
				xlog.Infof("eBPF cgroup attachment failed (sockmap still works, but may have reduced performance): %v", err)
			}
		}
	}

	return h
}

func (h *Handler) Handle(src net.Conn) {
	// Metrics: Track active connections
	middleware.IncActiveConnections("tcp")
	defer middleware.DecActiveConnections("tcp")
	defer src.Close()

	// Track connection start time and bytes for metrics
	startTime := time.Now()
	var bytesIn, bytesOut int64

	// Connect to backend with timeout
	connTimeout := 5 * time.Second
	dst, err := net.DialTimeout("tcp", h.backendAddr, connTimeout)
	if err != nil {
		xlog.Errorf("Failed to dial backend %s: %v", h.backendAddr, err)
		if h.security != nil {
			h.security.AuditTCP(src.RemoteAddr().String(), h.backendAddr, false, err.Error())
		}
		// Record failed connection metrics
		middleware.RecordUpstreamRequest(h.backendAddr, "connection_failed", 0)
		return
	}
	defer dst.Close()

	xlog.Infof("TCP Proxy: %s <-> %s", src.RemoteAddr(), dst.RemoteAddr())
	if h.security != nil {
		h.security.AuditTCP(src.RemoteAddr().String(), h.backendAddr, true, "")
	}

	// Register socket pair for eBPF redirection (if enabled)
	if h.ebpfEnabled {
		if err := h.sockMapMgr.RegisterSocketPair(src, dst); err != nil {
			xlog.Debugf("Failed to register socket pair in eBPF: %v", err)
		} else {
			xlog.Debugf("Socket pair registered in eBPF SockMap")
			defer h.sockMapMgr.UnregisterSocketPair(src, dst)
		}
	}

	// Bidirectional Copy (userspace fallback + eBPF acceleration)
	// Even with eBPF, we need this for initial packets and fallback
	// eBPF will handle most packets at kernel level after registration
	errChan := make(chan error, 2)
	bytesChan := make(chan struct{ in, out int64 }, 2)

	go func() {
		// src -> dst (Upstream)
		n, err := io.Copy(dst, src)
		bytesChan <- struct{ in, out int64 }{in: n, out: 0}
		errChan <- err
	}()

	go func() {
		// dst -> src (Downstream)
		n, err := io.Copy(src, dst)
		bytesChan <- struct{ in, out int64 }{in: 0, out: n}
		errChan <- err
	}()

	// Wait for any side to close
	<-errChan

	// Collect bytes transferred from both directions
	for i := 0; i < 2; i++ {
		select {
		case b := <-bytesChan:
			bytesIn += b.in
			bytesOut += b.out
		default:
		}
	}

	// Record TCP metrics
	duration := time.Since(startTime)
	middleware.RecordTCPMetrics(h.backendAddr, duration.Seconds(), bytesIn, bytesOut)
	middleware.RecordConnectionDuration("tcp", duration.Seconds())

	// Record successful upstream request
	middleware.RecordUpstreamRequest(h.backendAddr, "success", duration.Seconds())
}
