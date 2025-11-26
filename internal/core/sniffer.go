package core

import (
	"bufio"
	"io"
	"net"
	"strings"
	"time"

	"github.com/SkynetNext/unified-access-gateway/pkg/xlog"
)

// ProtocolType enum
type ProtocolType int

const (
	ProtocolUnknown ProtocolType = iota
	ProtocolHTTP
	ProtocolTCP // Custom Binary Protocol
	ProtocolTLS
)

// SniffConn wraps net.Conn with Peek support
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

// Read implements io.Reader, favoring buffer
func (s *SniffConn) Read(p []byte) (int, error) {
	return s.r.Read(p)
}

// Unwrap returns the underlying net.Conn for eBPF socket cookie extraction
// This implements the ebpf.UnwrappableConn interface (implicitly, no import needed)
func (s *SniffConn) Unwrap() net.Conn {
	return s.Conn
}

// Sniff detects protocol type
func (s *SniffConn) Sniff() ProtocolType {
	// Set read deadline to prevent hanging on malicious connections
	s.Conn.SetReadDeadline(time.Now().Add(time.Millisecond * 500))
	defer s.Conn.SetReadDeadline(time.Time{}) // Clear deadline

	// Peek first 5 bytes
	bytes, err := s.r.Peek(5)
	if err != nil && err != io.EOF {
		return ProtocolUnknown
	}

	if len(bytes) < 2 {
		return ProtocolUnknown
	}

	// HTTP detection: GET, POST, PUT, DELETE, HEAD...
	// Check first 3-4 bytes for HTTP methods
	head := string(bytes)
	if strings.HasPrefix(head, "GET") || strings.HasPrefix(head, "POST") ||
		strings.HasPrefix(head, "PUT ") || strings.HasPrefix(head, "DELE") ||
		strings.HasPrefix(head, "HEAD") || strings.HasPrefix(head, "HTTP") {
		return ProtocolHTTP
	}

	// TLS detection: 0x16 (Handshake)
	if bytes[0] == 0x16 {
		return ProtocolTLS
	}

	// Default fallback to TCP (Assuming custom game protocol)
	xlog.Debugf("[SNIFF] %s -> TCP, peek: hex=%x ascii=%q string=%q", s.Conn.RemoteAddr(), bytes, bytes, head)
	return ProtocolTCP
}
