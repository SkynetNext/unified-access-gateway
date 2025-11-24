package core

import (
	"hgame-gateway/internal/protocol/http"
	"hgame-gateway/internal/protocol/tcp"
	"hgame-gateway/pkg/xlog"
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
			xlog.Errorf("Accept error: %v", err)
			continue
		}

		go l.handleConn(conn)
	}
}

func (l *Listener) handleConn(c net.Conn) {
	// 1. 包装连接
	sniffConn := NewSniffConn(c)

	// 2. 嗅探协议
	proto := sniffConn.Sniff()

	// 3. 分发处理
	switch proto {
	case ProtocolHTTP:
		xlog.Debugf("New connection identified as HTTP from %s", c.RemoteAddr())
		// 交给 HTTP Handler 处理
		// 注意：http.Serve 需要一个 Listener，这里我们用单个连接模拟或者直接处理
		// 简化起见，这里假设 httpHandler 有一个处理单连接的方法
		go l.httpHandler.ServeConn(sniffConn)
		
	case ProtocolTCP:
		xlog.Debugf("New connection identified as TCP from %s", c.RemoteAddr())
		// 交给 TCP Handler 处理
		go l.tcpHandler.Handle(sniffConn)
		
	default:
		xlog.Warnf("Unknown protocol from %s, closing", c.RemoteAddr())
		c.Close()
	}
}

