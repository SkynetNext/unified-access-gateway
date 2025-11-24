package core

import (
	"bufio"
	"io"
	"net"
	"time"
)

// ProtocolType 定义协议类型
type ProtocolType int

const (
	ProtocolUnknown ProtocolType = iota
	ProtocolHTTP
	ProtocolTCP // 自定义二进制
	ProtocolTLS
)

// SniffConn 包装 net.Conn，支持 Peek
type SniffConn struct {
	net.Conn
	r *bufio.Reader
}

func NewSniffConn(c net.Conn) *SniffConn {
	return &SniffConn{
		Conn: c,
		r:    bufio.NewReader(c),
	}
}

// Read 实现 io.Reader，优先读缓冲
func (s *SniffConn) Read(p []byte) (int, error) {
	return s.r.Read(p)
}

// Sniff 嗅探协议类型
func (s *SniffConn) Sniff() ProtocolType {
	// 设置读取超时，防止恶意连接 hang 住
	s.Conn.SetReadDeadline(time.Now().Add(time.Millisecond * 500))
	defer s.Conn.SetReadDeadline(time.Time{}) // 清除超时

	// 偷看前 5 个字节
	bytes, err := s.r.Peek(5)
	if err != nil && err != io.EOF {
		return ProtocolUnknown
	}
	
	if len(bytes) < 2 {
		return ProtocolUnknown
	}

	// HTTP 判定: GET, POS, PUT, DEL, HEA...
	// 简单粗暴判断前几个字母
	head := string(bytes)
	if head == "GET " || head == "POST" || head == "HTTP" {
		return ProtocolHTTP
	}
	
	// TLS 判定: 0x16 (Handshake)
	if bytes[0] == 0x16 {
		return ProtocolTLS
	}

	// 默认回退到 TCP (认为是游戏私有协议)
	return ProtocolTCP
}

