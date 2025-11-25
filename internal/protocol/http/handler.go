package http

import (
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/SkynetNext/unified-access-gateway/internal/config"
	"github.com/SkynetNext/unified-access-gateway/internal/middleware"
	"github.com/SkynetNext/unified-access-gateway/internal/security"
	"github.com/SkynetNext/unified-access-gateway/pkg/xlog"
)

type Handler struct {
	proxy    *httputil.ReverseProxy
	backend  string
	security *security.Manager
}

func NewHandler(cfg *config.Config, sec *security.Manager) *Handler {
	backend := cfg.Backends.HTTP.TargetURL
	if backend == "" {
		// Business config MUST be loaded from Redis, no fallback
		xlog.Errorf("CRITICAL: backends.http.target_url is not configured (must be set in Redis)")
		return nil
	}

	target, err := url.Parse(backend)
	if err != nil {
		xlog.Errorf("CRITICAL: Invalid backend URL: %s, error: %v", backend, err)
		return nil
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
		proxy:    proxy,
		backend:  backend,
		security: sec,
	}
}

func (h *Handler) ServeConn(c net.Conn) {
	// Metrics: Inc active connection
	middleware.IncActiveConnections("http")
	defer middleware.DecActiveConnections("http")

	start := time.Now()

	// Create a OneShotListener for this connection
	l := &oneShotListener{c: c}

	// Wrap handler to record metrics and security controls
	wrappedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var denyErr error
		denyStatus := http.StatusForbidden
		if h.security != nil {
			if err := h.security.AuthorizeHTTP(r); err != nil {
				http.Error(w, err.Error(), http.StatusUnauthorized)
				denyStatus = http.StatusUnauthorized
				denyErr = err
			} else if err := h.security.ApplyWAF(r); err != nil {
				http.Error(w, "blocked by WAF", http.StatusForbidden)
				denyErr = err
			}
			if denyErr != nil {
				h.security.AuditHTTP(r, denyStatus, 0, denyErr)
				return
			}
		}

		recorder := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		h.proxy.ServeHTTP(recorder, r)

		duration := time.Since(start)
		if h.security != nil {
			h.security.AuditHTTP(r, recorder.statusCode, duration, nil)
		}
	})

	server := &http.Server{
		Handler:      middleware.K8sProbeMiddleware(middleware.CloudNativeMiddleware(wrappedHandler)),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	if err := server.Serve(l); err != nil && err != ErrListenerClosed {
		xlog.Debugf("HTTP Serve finished: %v", err)
	}
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.statusCode = code
	sr.ResponseWriter.WriteHeader(code)
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
