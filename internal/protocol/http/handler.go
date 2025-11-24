package http

import (
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"

	"github.com/SkynetNext/unified-access-gateway/pkg/xlog"
	"github.com/SkynetNext/unified-access-gateway/internal/middleware"
)

type Handler struct {
	proxy   *httputil.ReverseProxy
	backend string
}

func NewHandler() *Handler {
	backend := os.Getenv("HTTP_BACKEND_URL")
	if backend == "" {
		backend = "http://127.0.0.1:8181"
	}

	target, err := url.Parse(backend)
	if err != nil {
		xlog.Errorf("Invalid backend URL: %s, error: %v", backend, err)
	}

	// Custom Director to support Metrics and Header modification
	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		// Add X-Forwarded-For or other headers here
		req.Header.Set("X-Gateway-ID", "uag-v1")
	}

	// Custom ModifyResponse to record Status Code (Optional)
	proxy.ModifyResponse = func(resp *http.Response) error {
		// Log status code here for Access Log
		return nil
	}

	return &Handler{
		proxy:   proxy,
		backend: backend,
	}
}

func (h *Handler) ServeConn(c net.Conn) {
	// Metrics: Inc active connection
	middleware.IncActiveConnections("http")
	defer middleware.DecActiveConnections("http")

	start := time.Now()
	
	// Create a OneShotListener for this connection
	l := &oneShotListener{c: c}
	
	// Wrap handler to record metrics
	wrappedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Start processing
		h.proxy.ServeHTTP(w, r)
		
		// Metrics: Record duration
		duration := time.Since(start).Seconds()
		middleware.RecordMetrics("http", "200", duration) // Simplified: assume 200, use ResponseWriter wrapper for real status
	})

	server := &http.Server{
		Handler:      wrappedHandler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	
	if err := server.Serve(l); err != nil && err != ErrListenerClosed {
		xlog.Debugf("HTTP Serve finished: %v", err)
	}
}

// oneShotListener is a helper struct
type oneShotListener struct {
	c    net.Conn
	done bool
}

var ErrListenerClosed = net.ErrClosed

func (l *oneShotListener) Accept() (net.Conn, error) {
	if l.done {
		return nil, ErrListenerClosed
	}
	l.done = true
	return l.c, nil
}

func (l *oneShotListener) Close() error {
	return nil
}

func (l *oneShotListener) Addr() net.Addr {
	return l.c.LocalAddr()
}
