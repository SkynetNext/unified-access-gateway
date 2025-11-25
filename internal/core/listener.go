package core

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/SkynetNext/unified-access-gateway/internal/config"
	httpproxy "github.com/SkynetNext/unified-access-gateway/internal/protocol/http"
	tcpproxy "github.com/SkynetNext/unified-access-gateway/internal/protocol/tcp"
	"github.com/SkynetNext/unified-access-gateway/internal/security"
	"github.com/SkynetNext/unified-access-gateway/pkg/xlog"
)

type Listener struct {
	address  string
	listener net.Listener

	cfg      *config.Config
	security *security.Manager

	httpHandler *httpproxy.Handler
	tcpHandler  *tcpproxy.Handler
}

func NewListener(cfg *config.Config, sec *security.Manager) *Listener {
	l := &Listener{
		address:  cfg.Server.ListenAddr,
		cfg:      cfg,
		security: sec,
	}

	// Create handlers (may return nil if config is missing)
	l.httpHandler = httpproxy.NewHandler(cfg, sec)
	l.tcpHandler = tcpproxy.NewHandler(cfg, sec)

	return l
}

func (l *Listener) Start() error {
	// Check if handlers are properly initialized
	if l.httpHandler == nil && l.tcpHandler == nil {
		xlog.Errorf("CRITICAL: No handlers available. Check business config in Redis.")
		return fmt.Errorf("no handlers available")
	}

	if l.address == "" {
		xlog.Errorf("CRITICAL: server.listen_addr is not configured")
		return fmt.Errorf("listen address not configured")
	}

	var err error
	l.listener, err = net.Listen("tcp", l.address)
	if err != nil {
		return err
	}

	xlog.Infof("Gateway listening on %s", l.address)

	go l.acceptLoop()
	return nil
}

func (l *Listener) Stop() {
	if l.listener != nil {
		l.listener.Close()
	}
}

func (l *Listener) acceptLoop() {
	for {
		conn, err := l.listener.Accept()
		if err != nil {
			// Check if listener was closed (normal shutdown during graceful shutdown)
			errStr := err.Error()
			if strings.Contains(errStr, "use of closed network connection") ||
				strings.Contains(errStr, "operation on closed") {
				// Listener was closed, exit gracefully (this is expected during shutdown)
				xlog.Infof("Listener closed, exiting accept loop")
				return
			}
			
			// Check for temporary errors (network issues, can retry)
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Temporary() {
				xlog.Warnf("Temporary accept error: %v", err)
				continue
			}
			
			// Other permanent errors
			xlog.Errorf("Accept error: %v", err)
			return
		}

		go l.handleConn(conn)
	}
}

func (l *Listener) handleConn(c net.Conn) {
	if l.security != nil {
		if err := l.security.CheckConnection(c.RemoteAddr()); err != nil {
			xlog.Warnf("Connection %s rejected: %v", c.RemoteAddr(), err)
			l.security.AuditTCP(c.RemoteAddr().String(), "", false, err.Error())
			c.Close()
			return
		}
	}
	// 1. Wrap connection (Support Peek)
	sniffConn := NewSniffConn(c)

	// 2. Sniff protocol (Magic Bytes)
	proto := sniffConn.Sniff()

	// 3. Dispatch
	switch proto {
	case ProtocolHTTP:
		if l.httpHandler == nil {
			xlog.Warnf("Conn %s -> HTTP but handler not configured, closing", c.RemoteAddr())
			c.Close()
			return
		}
		xlog.Debugf("Conn %s -> HTTP", c.RemoteAddr())
		l.httpHandler.ServeConn(sniffConn)

	case ProtocolTCP:
		if l.tcpHandler == nil {
			xlog.Warnf("Conn %s -> TCP but handler not configured, closing", c.RemoteAddr())
			c.Close()
			return
		}
		xlog.Debugf("Conn %s -> TCP", c.RemoteAddr())
		l.tcpHandler.Handle(sniffConn)

	default:
		xlog.Warnf("Conn %s -> Unknown Protocol, closing", c.RemoteAddr())
		c.Close()
	}
}
