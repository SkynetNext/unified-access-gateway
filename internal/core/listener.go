package core

import (
	"github.com/SkynetNext/unified-access-gateway/internal/protocol/http"
	"github.com/SkynetNext/unified-access-gateway/internal/protocol/tcp"
	"github.com/SkynetNext/unified-access-gateway/pkg/xlog"
	"net"
)

type Listener struct {
	address     string
	listener    net.Listener
	httpHandler *http.Handler
	tcpHandler  *tcp.Handler
}

func NewListener(addr string) *Listener {
	return &Listener{
		address:     addr,
		httpHandler: http.NewHandler(),
		tcpHandler:  tcp.NewHandler(),
	}
}

func (l *Listener) Start() error {
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
			// Log error but don't crash, could be just a closed listener
			xlog.Errorf("Accept error: %v", err)
			continue
		}

		go l.handleConn(conn)
	}
}

func (l *Listener) handleConn(c net.Conn) {
	// 1. Wrap connection (Support Peek)
	sniffConn := NewSniffConn(c)

	// 2. Sniff protocol (Magic Bytes)
	proto := sniffConn.Sniff()

	// 3. Dispatch
	switch proto {
	case ProtocolHTTP:
		xlog.Debugf("Conn %s -> HTTP", c.RemoteAddr())
		l.httpHandler.ServeConn(sniffConn)
		
	case ProtocolTCP:
		xlog.Debugf("Conn %s -> TCP", c.RemoteAddr())
		l.tcpHandler.Handle(sniffConn)
		
	default:
		xlog.Warnf("Conn %s -> Unknown Protocol, closing", c.RemoteAddr())
		c.Close()
	}
}
