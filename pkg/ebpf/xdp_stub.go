// +build !linux

package ebpf

import "errors"

// XDPManager stub for non-Linux platforms
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

// NewXDPManager returns a disabled manager on non-Linux platforms
func NewXDPManager() (*XDPManager, error) {
	return &XDPManager{enabled: false}, nil
}

// AttachToInterface is a no-op on non-Linux platforms
func (m *XDPManager) AttachToInterface(ifaceName string) error {
	return errors.New("XDP not supported on this platform")
}

// AddToBlacklist is a no-op on non-Linux platforms
func (m *XDPManager) AddToBlacklist(ipAddr string) error {
	return nil
}

// RemoveFromBlacklist is a no-op on non-Linux platforms
func (m *XDPManager) RemoveFromBlacklist(ipAddr string) error {
	return nil
}

// GetStats returns empty stats on non-Linux platforms
func (m *XDPManager) GetStats() (*XDPStats, error) {
	return &XDPStats{}, nil
}

// ResetRateLimits is a no-op on non-Linux platforms
func (m *XDPManager) ResetRateLimits() error {
	return nil
}

// Close is a no-op on non-Linux platforms
func (m *XDPManager) Close() error {
	return nil
}

// IsEnabled always returns false on non-Linux platforms
func (m *XDPManager) IsEnabled() bool {
	return false
}

