//go:build linux
// +build linux

package ebpf

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"github.com/SkynetNext/unified-access-gateway/pkg/xlog"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -g -Wall -Werror -D__TARGET_ARCH_x86_64" bpf sockmap.c

// SO_COOKIE socket option (Linux-specific)
const SO_COOKIE = 57

// SockMapManager manages eBPF sockmap for socket redirection
type SockMapManager struct {
	objs       *bpfObjects
	cgroupLink link.Link
	enabled    bool
}

// NewSockMapManager creates a new sockmap manager
func NewSockMapManager() (*SockMapManager, error) {
	// Check if eBPF is supported
	if !isEBPFSupported() {
		xlog.Infof("eBPF not supported on this system (insufficient permissions or MEMLOCK limit too low), falling back to userspace proxy")
		xlog.Infof("To enable eBPF: run with CAP_BPF capability or as root, and ensure MEMLOCK limit is sufficient")
		return &SockMapManager{enabled: false}, nil
	}

	// Load pre-compiled eBPF objects
	objs := &bpfObjects{}
	if err := loadBpfObjects(objs, nil); err != nil {
		xlog.Warnf("Failed to load eBPF objects (eBPF programs may not be compiled): %v", err)
		xlog.Infof("Falling back to userspace proxy. To enable eBPF, run: make generate-ebpf")
		return &SockMapManager{enabled: false}, nil
	}

	mgr := &SockMapManager{
		objs:    objs,
		enabled: true,
	}

	xlog.Infof("eBPF SockMap loaded successfully")
	return mgr, nil
}

// findCgroupPath attempts to find the correct cgroup path
// In Kubernetes with systemd cgroup driver, we need to find the root cgroup
// that matches the current process's cgroup hierarchy
func findCgroupPath() string {
	// Read current process cgroup to determine the hierarchy
	// Format: <id>:<controller>:<path>
	// Example: 0::/kubepods.slice/kubepods-burstable.slice/...
	cgroupData, err := os.ReadFile("/proc/self/cgroup")
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(cgroupData)), "\n")
		for _, line := range lines {
			parts := strings.Split(line, ":")
			if len(parts) >= 3 {
				cgroupPath := parts[2]
				// For cgroup v2 (empty controller), path starts with /
				// For cgroup v1 with systemd, path is like /system.slice/...
				if strings.HasPrefix(cgroupPath, "/") {
					// Extract root: for systemd, it's usually /sys/fs/cgroup or /sys/fs/cgroup/systemd
					// For cgroup v2, root is /sys/fs/cgroup
					// For cgroup v1 with systemd, root is /sys/fs/cgroup/systemd
					if strings.Contains(cgroupPath, "kubepods") || strings.Contains(cgroupPath, "system.slice") {
						// Kubernetes Pod: try to find root cgroup
						// Check if cgroup v2 (unified)
						if fd, err := syscall.Open("/sys/fs/cgroup", syscall.O_RDONLY, 0); err == nil {
							syscall.Close(fd)
							return "/sys/fs/cgroup"
						}
						// Fallback to systemd cgroup v1
						if fd, err := syscall.Open("/sys/fs/cgroup/systemd", syscall.O_RDONLY, 0); err == nil {
							syscall.Close(fd)
							return "/sys/fs/cgroup/systemd"
						}
					}
				}
			}
		}
	}

	// Fallback: try common paths
	paths := []string{
		"/sys/fs/cgroup",         // cgroup v2 root (K8s with systemd, unified)
		"/sys/fs/cgroup/unified", // cgroup v2 unified (if separate mount)
		"/sys/fs/cgroup/systemd", // systemd slice (cgroup v1, K8s with systemd)
	}

	for _, path := range paths {
		fd, err := syscall.Open(path, syscall.O_RDONLY, 0)
		if err == nil {
			syscall.Close(fd)
			return path
		}
	}

	// Default fallback
	return "/sys/fs/cgroup"
}

// AttachToCgroup attaches sockops program to cgroup
func (m *SockMapManager) AttachToCgroup(cgroupPath string) error {
	if !m.enabled {
		return errors.New("eBPF not enabled")
	}

	// Auto-detect cgroup path if not specified or default path doesn't work
	if cgroupPath == "" || cgroupPath == "/sys/fs/cgroup" {
		detectedPath := findCgroupPath()
		if detectedPath != cgroupPath {
			xlog.Debugf("Auto-detected cgroup path: %s", detectedPath)
			cgroupPath = detectedPath
		}
	}

	// Open cgroup
	cgroupFd, err := syscall.Open(cgroupPath, syscall.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("opening cgroup %s: %w", cgroupPath, err)
	}
	defer syscall.Close(cgroupFd)

	// Attach sockops program
	l, err := link.AttachCgroup(link.CgroupOptions{
		Path:    cgroupPath,
		Attach:  ebpf.AttachCGroupSockOps,
		Program: m.objs.SockOpsHandler,
	})
	if err != nil {
		return fmt.Errorf("attaching sockops to cgroup: %w", err)
	}

	m.cgroupLink = l
	xlog.Infof("eBPF sockops attached to cgroup: %s", cgroupPath)
	return nil
}

// RegisterSocketPair registers a client-backend socket pair for redirection
func (m *SockMapManager) RegisterSocketPair(clientConn, backendConn net.Conn) error {
	if !m.enabled {
		return nil // Silently skip if eBPF not enabled
	}

	// Extract socket cookies
	clientCookie, err := getSocketCookie(clientConn)
	if err != nil {
		return fmt.Errorf("getting client socket cookie: %w", err)
	}

	backendCookie, err := getSocketCookie(backendConn)
	if err != nil {
		return fmt.Errorf("getting backend socket cookie: %w", err)
	}

	// Update sock_pair_map: client -> backend
	if err := m.objs.SockPairMap.Update(clientCookie, backendCookie, ebpf.UpdateAny); err != nil {
		return fmt.Errorf("updating sock_pair_map (client->backend): %w", err)
	}

	// Update sock_pair_map: backend -> client (bidirectional)
	if err := m.objs.SockPairMap.Update(backendCookie, clientCookie, ebpf.UpdateAny); err != nil {
		return fmt.Errorf("updating sock_pair_map (backend->client): %w", err)
	}

	xlog.Debugf("Registered socket pair: client=%d <-> backend=%d", clientCookie, backendCookie)
	return nil
}

// UnregisterSocketPair removes a socket pair from the map
func (m *SockMapManager) UnregisterSocketPair(clientConn, backendConn net.Conn) error {
	if !m.enabled {
		return nil
	}

	clientCookie, _ := getSocketCookie(clientConn)
	backendCookie, _ := getSocketCookie(backendConn)

	m.objs.SockPairMap.Delete(&clientCookie)
	m.objs.SockPairMap.Delete(&backendCookie)

	return nil
}

// Close cleans up eBPF resources
func (m *SockMapManager) Close() error {
	if !m.enabled {
		return nil
	}

	if m.cgroupLink != nil {
		m.cgroupLink.Close()
	}

	if m.objs != nil {
		m.objs.Close()
	}

	xlog.Infof("eBPF SockMap manager closed")
	return nil
}

// IsEnabled returns whether eBPF acceleration is enabled
func (m *SockMapManager) IsEnabled() bool {
	return m.enabled
}

// getSocketCookie extracts the kernel socket cookie from a net.Conn
func getSocketCookie(conn net.Conn) (uint64, error) {
	// Get raw file descriptor
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return 0, errors.New("not a TCP connection")
	}

	rawConn, err := tcpConn.SyscallConn()
	if err != nil {
		return 0, err
	}

	var cookie uint64
	var sockErr error

	err = rawConn.Control(func(fd uintptr) {
		// Use SO_COOKIE socket option to get kernel cookie
		// This requires Linux kernel 4.6+
		var val uint64
		valLen := uint32(unsafe.Sizeof(val))
		_, _, errno := syscall.Syscall6(
			syscall.SYS_GETSOCKOPT,
			fd,
			uintptr(syscall.SOL_SOCKET),
			uintptr(SO_COOKIE),
			uintptr(unsafe.Pointer(&val)),
			uintptr(unsafe.Pointer(&valLen)),
			0,
		)
		if errno != 0 {
			sockErr = errno
			return
		}
		cookie = val
	})

	if err != nil {
		return 0, err
	}
	if sockErr != nil {
		return 0, sockErr
	}

	return cookie, nil
}

// isEBPFSupported checks if the system supports eBPF
func isEBPFSupported() bool {
	// Try to create a simple eBPF map to test support
	spec := &ebpf.MapSpec{
		Type:       ebpf.Hash,
		KeySize:    4,
		ValueSize:  4,
		MaxEntries: 1,
	}

	m, err := ebpf.NewMap(spec)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "operation not permitted") {
			xlog.Debugf("eBPF map creation failed: %v", err)
			xlog.Debugf("Hint: Need CAP_BPF or CAP_SYS_ADMIN capability, or run as root")
			xlog.Debugf("Hint: If MEMLOCK error, increase limit: ulimit -l unlimited")
		} else {
			xlog.Debugf("eBPF map creation test failed: %v", err)
		}
		return false
	}
	m.Close()

	return true
}
