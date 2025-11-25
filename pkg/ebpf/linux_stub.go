//go:build linux && !ebpf
// +build linux,!ebpf

package ebpf

import (
	"errors"
	"net"
)

// Stub implementation for Linux without eBPF support
// Used when building on Linux but without -tags ebpf

type bpfObjects struct{}

func loadBpfObjects(objs *bpfObjects, opts interface{}) error {
	return errors.New("eBPF not enabled (build without -tags ebpf)")
}

func (o *bpfObjects) Close() error {
	return nil
}

// SockMapManager stub for Linux without eBPF
type SockMapManager struct {
	enabled bool
}

// NewSockMapManager returns a disabled manager when eBPF is not enabled
func NewSockMapManager() (*SockMapManager, error) {
	return &SockMapManager{enabled: false}, nil
}

// AttachToCgroup is a no-op when eBPF is not enabled
func (m *SockMapManager) AttachToCgroup(cgroupPath string) error {
	return errors.New("eBPF not enabled (build with -tags ebpf)")
}

// RegisterSocketPair is a no-op when eBPF is not enabled
func (m *SockMapManager) RegisterSocketPair(clientConn, backendConn net.Conn) error {
	return nil // Silently skip
}

// UnregisterSocketPair is a no-op when eBPF is not enabled
func (m *SockMapManager) UnregisterSocketPair(clientConn, backendConn net.Conn) error {
	return nil
}

// Close is a no-op when eBPF is not enabled
func (m *SockMapManager) Close() error {
	return nil
}

// IsEnabled always returns false when eBPF is not enabled
func (m *SockMapManager) IsEnabled() bool {
	return false
}

// XDPManager stub for Linux without eBPF
type XDPManager struct {
	enabled bool
}

// XDPStats stub
type XDPStats struct {
	TotalPackets      uint64
	DroppedBlacklist  uint64
	DroppedRateLimit  uint64
	DroppedInvalid    uint64
	Passed            uint64
	TCPSyn            uint64
	TCPSynFlood       uint64
}

// NewXDPManager returns a disabled manager when eBPF is not enabled
func NewXDPManager() (*XDPManager, error) {
	return &XDPManager{enabled: false}, nil
}

// AttachToInterface is a no-op when eBPF is not enabled
func (m *XDPManager) AttachToInterface(ifaceName string) error {
	return errors.New("XDP not enabled (build with -tags ebpf)")
}

// AddToBlacklist is a no-op when eBPF is not enabled
func (m *XDPManager) AddToBlacklist(ipAddr string) error {
	return nil
}

// RemoveFromBlacklist is a no-op when eBPF is not enabled
func (m *XDPManager) RemoveFromBlacklist(ipAddr string) error {
	return nil
}

// GetStats returns empty stats when eBPF is not enabled
func (m *XDPManager) GetStats() (*XDPStats, error) {
	return &XDPStats{}, nil
}

// ResetRateLimits is a no-op when eBPF is not enabled
func (m *XDPManager) ResetRateLimits() error {
	return nil
}

// Close is a no-op when eBPF is not enabled
func (m *XDPManager) Close() error {
	return nil
}

// IsEnabled always returns false when eBPF is not enabled
func (m *XDPManager) IsEnabled() bool {
	return false
}

