// Package ebpf provides eBPF-based socket acceleration using SockMap.
//
// # Overview
//
// This package implements kernel-level socket redirection using eBPF SockMap,
// which allows bypassing the TCP/IP stack for proxied connections, significantly
// reducing latency and CPU overhead.
//
// # Architecture
//
//	┌─────────────────────────────────────────────────────────┐
//	│                    User Space (Go)                      │
//	│  ┌──────────┐                          ┌──────────┐     │
//	│  │ Client   │                          │ Backend  │     │
//	│  │ Socket   │                          │ Socket   │     │
//	│  └────┬─────┘                          └─────┬────┘     │
//	│       │                                      │          │
//	│       │  Register Pair (cookie1, cookie2)    │          │
//	│       └──────────────┬───────────────────────┘          │
//	└───────────────────────┼──────────────────────────────────┘
//	                        │
//	┌───────────────────────┼──────────────────────────────────┐
//	│               Kernel Space (eBPF)                        │
//	│                       ▼                                  │
//	│  ┌─────────────────────────────────────────────────┐    │
//	│  │           sock_pair_map (BPF_MAP_TYPE_HASH)     │    │
//	│  │   Key: socket_cookie  Value: peer_cookie        │    │
//	│  └─────────────────────────────────────────────────┘    │
//	│                       │                                  │
//	│                       ▼                                  │
//	│  ┌─────────────────────────────────────────────────┐    │
//	│  │        sock_map (BPF_MAP_TYPE_SOCKHASH)         │    │
//	│  │   Key: socket_cookie  Value: socket_fd          │    │
//	│  └─────────────────────────────────────────────────┘    │
//	│                       │                                  │
//	│                       ▼                                  │
//	│  ┌─────────────────────────────────────────────────┐    │
//	│  │         sk_skb/stream_verdict (BPF Program)     │    │
//	│  │  - Intercept packets                            │    │
//	│  │  - Lookup peer socket                           │    │
//	│  │  - Redirect to peer (bpf_sk_redirect_hash)      │    │
//	│  └─────────────────────────────────────────────────┘    │
//	└──────────────────────────────────────────────────────────┘
//
// # Performance Benefits
//
//   - Latency Reduction: ~30-50% (bypassing TCP/IP stack)
//   - CPU Reduction: ~40-60% (kernel-level forwarding)
//   - Zero-Copy: Data moves directly between sockets
//
// # Requirements
//
//   - Linux Kernel 4.14+ (for SockMap)
//   - Linux Kernel 4.18+ (for SOCKHASH)
//   - CAP_BPF or CAP_SYS_ADMIN capability
//   - Cgroup v2 mounted (for sockops attachment)
//
// # Build Requirements
//
//	apt-get install clang llvm libbpf-dev linux-headers-$(uname -r)
//	go install github.com/cilium/ebpf/cmd/bpf2go@latest
//
// # Usage
//
//	// Initialize manager
//	mgr, err := ebpf.NewSockMapManager()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer mgr.Close()
//
//	// Attach to cgroup (required for sockops)
//	if err := mgr.AttachToCgroup("/sys/fs/cgroup/unified"); err != nil {
//	    log.Fatal(err)
//	}
//
//	// Register socket pair for redirection
//	clientConn, _ := listener.Accept()
//	backendConn, _ := net.Dial("tcp", "backend:8080")
//	mgr.RegisterSocketPair(clientConn, backendConn)
//
//	// Now packets are redirected at kernel level!
//	// Still need userspace io.Copy as fallback for first packets
//
// # Limitations
//
//   - Only works for TCP connections
//   - Requires root or CAP_BPF capability
//   - Cgroup v2 required (not available in all environments)
//   - Not compatible with TLS termination at gateway (encrypted data)
//
// # Fallback Strategy
//
// The implementation gracefully falls back to userspace proxying if:
//   - eBPF is not supported on the system
//   - Insufficient permissions
//   - Cgroup attachment fails
//
// This ensures the gateway works everywhere, with eBPF as an optimization.
package ebpf

