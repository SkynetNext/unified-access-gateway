// +build linux

package ebpf

import (
	"fmt"
	"net"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/SkynetNext/unified-access-gateway/pkg/xlog"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -g -Wall -Werror -D__TARGET_ARCH_x86_64" xdp xdp_filter.c

// XDPManager manages XDP programs for early packet filtering
type XDPManager struct {
	objs    *xdpObjects
	link    link.Link
	enabled bool
}

// XDPStats represents XDP statistics
type XDPStats struct {
	TotalPackets      uint64
	DroppedBlacklist  uint64
	DroppedRateLimit  uint64
	DroppedInvalid    uint64
	Passed            uint64
	TCPSyn            uint64
	TCPSynFlood       uint64
}

// NewXDPManager creates a new XDP manager
func NewXDPManager() (*XDPManager, error) {
	// Check if XDP is supported
	if !isXDPSupported() {
		xlog.Infof("XDP not supported on this system")
		return &XDPManager{enabled: false}, nil
	}

	// Load XDP objects
	objs := &xdpObjects{}
	if err := loadXdpObjects(objs, nil); err != nil {
		return nil, fmt.Errorf("loading XDP objects: %w", err)
	}

	mgr := &XDPManager{
		objs:    objs,
		enabled: true,
	}

	xlog.Infof("XDP program loaded successfully")
	return mgr, nil
}

// AttachToInterface attaches XDP program to a network interface
func (m *XDPManager) AttachToInterface(ifaceName string) error {
	if !m.enabled {
		return fmt.Errorf("XDP not enabled")
	}

	// Get interface
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return fmt.Errorf("getting interface %s: %w", ifaceName, err)
	}

	// Attach XDP program
	l, err := link.AttachXDP(link.XDPOptions{
		Program:   m.objs.XdpFilterProg,
		Interface: iface.Index,
		Flags:     link.XDPGenericMode, // Use generic mode for compatibility
	})
	if err != nil {
		return fmt.Errorf("attaching XDP to interface %s: %w", ifaceName, err)
	}

	m.link = l
	xlog.Infof("XDP program attached to interface: %s", ifaceName)
	return nil
}

// AddToBlacklist adds an IP address to the blacklist
func (m *XDPManager) AddToBlacklist(ipAddr string) error {
	if !m.enabled {
		return nil
	}

	ip := net.ParseIP(ipAddr)
	if ip == nil {
		return fmt.Errorf("invalid IP address: %s", ipAddr)
	}

	// Convert to IPv4 uint32 (network byte order)
	ipv4 := ip.To4()
	if ipv4 == nil {
		return fmt.Errorf("not an IPv4 address: %s", ipAddr)
	}

	ipInt := uint32(ipv4[0])<<24 | uint32(ipv4[1])<<16 | uint32(ipv4[2])<<8 | uint32(ipv4[3])
	blocked := uint8(1)

	if err := m.objs.IpBlacklist.Update(&ipInt, &blocked, ebpf.UpdateAny); err != nil {
		return fmt.Errorf("updating blacklist: %w", err)
	}

	xlog.Infof("Added IP to blacklist: %s", ipAddr)
	return nil
}

// RemoveFromBlacklist removes an IP address from the blacklist
func (m *XDPManager) RemoveFromBlacklist(ipAddr string) error {
	if !m.enabled {
		return nil
	}

	ip := net.ParseIP(ipAddr)
	if ip == nil {
		return fmt.Errorf("invalid IP address: %s", ipAddr)
	}

	ipv4 := ip.To4()
	if ipv4 == nil {
		return fmt.Errorf("not an IPv4 address: %s", ipAddr)
	}

	ipInt := uint32(ipv4[0])<<24 | uint32(ipv4[1])<<16 | uint32(ipv4[2])<<8 | uint32(ipv4[3])

	if err := m.objs.IpBlacklist.Delete(&ipInt); err != nil {
		return fmt.Errorf("removing from blacklist: %w", err)
	}

	xlog.Infof("Removed IP from blacklist: %s", ipAddr)
	return nil
}

// GetStats retrieves XDP statistics
func (m *XDPManager) GetStats() (*XDPStats, error) {
	if !m.enabled {
		return &XDPStats{}, nil
	}

	stats := &XDPStats{}

	// Read statistics from map
	var key uint32
	var val uint64

	key = 0 // STAT_TOTAL_PACKETS
	if err := m.objs.StatsMap.Lookup(&key, &val); err == nil {
		stats.TotalPackets = val
	}

	key = 1 // STAT_DROPPED_BLACKLIST
	if err := m.objs.StatsMap.Lookup(&key, &val); err == nil {
		stats.DroppedBlacklist = val
	}

	key = 2 // STAT_DROPPED_RATELIMIT
	if err := m.objs.StatsMap.Lookup(&key, &val); err == nil {
		stats.DroppedRateLimit = val
	}

	key = 3 // STAT_DROPPED_INVALID
	if err := m.objs.StatsMap.Lookup(&key, &val); err == nil {
		stats.DroppedInvalid = val
	}

	key = 4 // STAT_PASSED
	if err := m.objs.StatsMap.Lookup(&key, &val); err == nil {
		stats.Passed = val
	}

	key = 5 // STAT_TCP_SYN
	if err := m.objs.StatsMap.Lookup(&key, &val); err == nil {
		stats.TCPSyn = val
	}

	key = 6 // STAT_TCP_SYN_FLOOD
	if err := m.objs.StatsMap.Lookup(&key, &val); err == nil {
		stats.TCPSynFlood = val
	}

	return stats, nil
}

// ResetRateLimits clears the rate limit counters (should be called periodically)
func (m *XDPManager) ResetRateLimits() error {
	if !m.enabled {
		return nil
	}

	// Clear the rate limit map
	// Note: In production, use a time-based sliding window instead
	var key uint32
	var val uint64

	// Iterate and delete all entries (simplified approach)
	iter := m.objs.RateLimitMap.Iterate()
	for iter.Next(&key, &val) {
		m.objs.RateLimitMap.Delete(&key)
	}

	xlog.Debugf("Reset rate limit counters")
	return nil
}

// Close cleans up XDP resources
func (m *XDPManager) Close() error {
	if !m.enabled {
		return nil
	}

	if m.link != nil {
		m.link.Close()
	}

	if m.objs != nil {
		m.objs.Close()
	}

	xlog.Infof("XDP manager closed")
	return nil
}

// IsEnabled returns whether XDP is enabled
func (m *XDPManager) IsEnabled() bool {
	return m.enabled
}

// isXDPSupported checks if the system supports XDP
func isXDPSupported() bool {
	// Try to create a simple XDP program to test support
	// In production, check kernel version (>= 4.8)
	spec := &ebpf.ProgramSpec{
		Type: ebpf.XDP,
		Instructions: []ebpf.Instruction{
			// XDP_PASS
			ebpf.LoadImm(ebpf.R0, 2, ebpf.DWord),
			ebpf.Return(),
		},
		License: "GPL",
	}

	prog, err := ebpf.NewProgram(spec)
	if err != nil {
		return false
	}
	prog.Close()

	return true
}

