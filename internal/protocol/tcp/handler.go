package tcp

import (
	"io"
	"net"
	"os"
	"time"
	
	"github.com/SkynetNext/unified-access-gateway/pkg/xlog"
	"github.com/SkynetNext/unified-access-gateway/internal/middleware"
)

type Handler struct {
	backendAddr string
}

func NewHandler() *Handler {
	addr := os.Getenv("TCP_BACKEND_ADDR")
	if addr == "" {
		addr = "127.0.0.1:9621"
	}
	return &Handler{
		backendAddr: addr,
	}
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

	// Bidirectional Copy
	// In production, consider using io.CopyBuffer for memory optimization
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
