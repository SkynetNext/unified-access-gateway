package tcp

import (
	"io"
	"net"
	"os"
	"time"
	
	"github.com/SkynetNext/unified-access-gateway/pkg/xlog"
	"github.com/SkynetNext/unified-access-gateway/internal/middleware"
	"github.com/SkynetNext/unified-access-gateway/pkg/ebpf"
)

type Handler struct {
	backendAddr   string
	sockMapMgr    *ebpf.SockMapManager
	ebpfEnabled   bool
}

func NewHandler() *Handler {
	addr := os.Getenv("TCP_BACKEND_ADDR")
	if addr == "" {
		addr = "127.0.0.1:9621"
	}
	
	h := &Handler{
		backendAddr: addr,
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
			// Try to attach to cgroup (optional)
			if err := mgr.AttachToCgroup("/sys/fs/cgroup"); err != nil {
				xlog.Infof("eBPF cgroup attachment failed (still using sockmap): %v", err)
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

	// Connect to backend with timeout
	connTimeout := 5 * time.Second
	dst, err := net.DialTimeout("tcp", h.backendAddr, connTimeout)
	if err != nil {
		xlog.Errorf("Failed to dial backend %s: %v", h.backendAddr, err)
		return
	}
	defer dst.Close()

	xlog.Infof("TCP Proxy: %s <-> %s", src.RemoteAddr(), dst.RemoteAddr())

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
	
	go func() {
		// src -> dst (Upstream)
		_, err := io.Copy(dst, src)
		errChan <- err
	}()
	
	go func() {
		// dst -> src (Downstream)
		_, err := io.Copy(src, dst)
		errChan <- err
	}()

	// Wait for any side to close
	<-errChan
}
