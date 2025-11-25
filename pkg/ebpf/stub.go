//go:build !linux
// +build !linux

package ebpf

import (
	"errors"
	"net"
)

// Stub implementation for non-Linux platforms
// eBPF is Linux-only, so we provide a no-op implementation for Windows/macOS

type bpfObjects struct{}

func loadBpfObjects(objs *bpfObjects, opts interface{}) error {
	return errors.New("eBPF not supported on this platform")
}

func (o *bpfObjects) Close() error {
	return nil
}

// SockMapManager stub for non-Linux platforms
type SockMapManager struct {
	enabled bool
}

// NewSockMapManager returns a disabled manager on non-Linux platforms
func NewSockMapManager() (*SockMapManager, error) {
	return &SockMapManager{enabled: false}, nil
}

// AttachToCgroup is a no-op on non-Linux platforms
func (m *SockMapManager) AttachToCgroup(cgroupPath string) error {
	return errors.New("eBPF not supported on this platform")
}

// RegisterSocketPair is a no-op on non-Linux platforms
func (m *SockMapManager) RegisterSocketPair(clientConn, backendConn net.Conn) error {
	return nil // Silently skip
}

// UnregisterSocketPair is a no-op on non-Linux platforms
func (m *SockMapManager) UnregisterSocketPair(clientConn, backendConn net.Conn) error {
	return nil
}

// Close is a no-op on non-Linux platforms
func (m *SockMapManager) Close() error {
	return nil
}

// IsEnabled always returns false on non-Linux platforms
func (m *SockMapManager) IsEnabled() bool {
	return false
}

