# XDP (eXpress Data Path) - DDoS Protection

## Overview

XDP (eXpress Data Path) is an eBPF-based technology that enables **ultra-fast packet processing at the network driver layer**, before packets enter the kernel network stack. This makes it ideal for **DDoS mitigation** and **early traffic filtering**.

## Why XDP?

### Traditional Network Stack

```
┌─────────────────────────────────────────┐
│  Application (Gateway)                  │  ← Too late, CPU already wasted
└─────────────────┬───────────────────────┘
                  │
┌─────────────────▼───────────────────────┐
│  TCP/IP Stack (Kernel)                  │  ← Expensive processing
└─────────────────┬───────────────────────┘
                  │
┌─────────────────▼───────────────────────┐
│  Network Driver                         │
└─────────────────┬───────────────────────┘
                  │
                  ▼
            Network Interface Card (NIC)
```

**Problem**: Malicious packets consume CPU resources traversing the entire stack.

### XDP Approach

```
            Network Interface Card (NIC)
                  │
┌─────────────────▼───────────────────────┐
│  Network Driver                         │
│  ┌─────────────────────────────────┐   │
│  │  XDP Program (eBPF)             │   │  ← Drop here!
│  │  - IP Blacklist                 │   │
│  │  - Rate Limiting                │   │
│  │  - SYN Flood Protection         │   │
│  └─────────────────────────────────┘   │
└─────────────────┬───────────────────────┘
                  │ (Only legitimate traffic)
┌─────────────────▼───────────────────────┐
│  TCP/IP Stack (Kernel)                  │
└─────────────────┬───────────────────────┘
                  │
┌─────────────────▼───────────────────────┐
│  Application (Gateway)                  │
└─────────────────────────────────────────┘
```

**Benefit**: Malicious packets are dropped at the **earliest possible point**, saving CPU cycles.

## Performance

| Metric | Traditional Firewall | XDP |
|--------|---------------------|-----|
| **Packet Processing** | 1-2 Mpps | **20-30 Mpps** |
| **Latency** | 50-100 μs | **<10 μs** |
| **CPU Usage (DDoS)** | 100% | **<20%** |
| **Drop Location** | Kernel stack | **Driver layer** |

*Mpps = Million packets per second*

## Features Implemented

### 1. IP Blacklist

Block malicious IPs at the driver layer:

```go
xdpMgr, _ := ebpf.NewXDPManager()
xdpMgr.AttachToInterface("eth0")

// Block an attacker
xdpMgr.AddToBlacklist("192.168.1.100")
```

**Use Case**: Block known attackers, botnets, or malicious IPs from threat intelligence feeds.

### 2. Rate Limiting

Limit packets per source IP:

```c
// In XDP program (xdp_filter.c)
#define RATE_LIMIT_THRESHOLD 1000  // Max 1000 packets/sec per IP

if (*pkt_count > RATE_LIMIT_THRESHOLD) {
    return XDP_DROP;  // Drop excessive packets
}
```

**Use Case**: Prevent single-source flooding attacks.

### 3. SYN Flood Protection

Detect and block TCP SYN flood attacks:

```c
// Detect SYN packets
if (tcp->syn && !tcp->ack) {
    if (*pkt_count > 100) {  // 100 SYNs/sec threshold
        // Add to blacklist temporarily
        bpf_map_update_elem(&ip_blacklist, &src_ip, &block, BPF_ANY);
        return XDP_DROP;
    }
}
```

**Use Case**: Protect against SYN flood attacks that exhaust connection tables.

### 4. Statistics

Real-time monitoring of XDP activity:

```go
stats, _ := xdpMgr.GetStats()
fmt.Printf("Total Packets: %d\n", stats.TotalPackets)
fmt.Printf("Dropped (Blacklist): %d\n", stats.DroppedBlacklist)
fmt.Printf("Dropped (Rate Limit): %d\n", stats.DroppedRateLimit)
fmt.Printf("SYN Flood Attempts: %d\n", stats.TCPSynFlood)
```

## Architecture

### XDP Program Flow

```c
SEC("xdp")
int xdp_filter_prog(struct xdp_md *ctx) {
    // 1. Parse Ethernet + IP headers
    struct ethhdr *eth = data;
    struct iphdr *ip = (void *)(eth + 1);
    
    // 2. Check IP blacklist
    if (bpf_map_lookup_elem(&ip_blacklist, &src_ip)) {
        return XDP_DROP;  // Drop immediately
    }
    
    // 3. Rate limiting
    if (*pkt_count > RATE_LIMIT_THRESHOLD) {
        return XDP_DROP;
    }
    
    // 4. SYN flood detection
    if (tcp->syn && !tcp->ack && *pkt_count > 100) {
        bpf_map_update_elem(&ip_blacklist, &src_ip, &block, BPF_ANY);
        return XDP_DROP;
    }
    
    // 5. Pass legitimate traffic
    return XDP_PASS;
}
```

### XDP Action Codes

| Action | Value | Description |
|--------|-------|-------------|
| `XDP_DROP` | 1 | Drop packet (DDoS mitigation) |
| `XDP_PASS` | 2 | Pass to kernel stack (legitimate traffic) |
| `XDP_TX` | 3 | Transmit from same interface (reflection) |
| `XDP_REDIRECT` | 4 | Redirect to another interface (load balancing) |
| `XDP_ABORTED` | 0 | Error, drop packet |

## Deployment

### 1. Load XDP Program

```go
import "github.com/SkynetNext/unified-access-gateway/pkg/ebpf"

func main() {
    // Initialize XDP manager
    xdpMgr, err := ebpf.NewXDPManager()
    if err != nil {
        log.Fatalf("Failed to load XDP: %v", err)
    }
    defer xdpMgr.Close()
    
    // Attach to network interface
    if err := xdpMgr.AttachToInterface("eth0"); err != nil {
        log.Fatalf("Failed to attach XDP: %v", err)
    }
    
    log.Println("XDP protection enabled on eth0")
}
```

### 2. Manage Blacklist

```go
// Add attacker to blacklist
xdpMgr.AddToBlacklist("203.0.113.42")

// Remove from blacklist
xdpMgr.RemoveFromBlacklist("203.0.113.42")
```

### 3. Monitor Statistics

```go
// Periodically check stats
ticker := time.NewTicker(10 * time.Second)
for range ticker.C {
    stats, _ := xdpMgr.GetStats()
    log.Printf("XDP Stats: Total=%d, Dropped=%d, Passed=%d",
        stats.TotalPackets,
        stats.DroppedBlacklist + stats.DroppedRateLimit,
        stats.Passed)
}
```

### 4. Reset Rate Limits

```go
// Reset counters every second (sliding window)
ticker := time.NewTicker(1 * time.Second)
for range ticker.C {
    xdpMgr.ResetRateLimits()
}
```

## Kubernetes Integration

### DaemonSet Deployment

XDP should run on **every node** to protect the entire cluster:

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: xdp-ddos-protection
spec:
  selector:
    matchLabels:
      app: xdp-protection
  template:
    metadata:
      labels:
        app: xdp-protection
    spec:
      hostNetwork: true  # Required for XDP
      containers:
      - name: xdp-agent
        image: skynet/unified-access-gateway:latest
        securityContext:
          privileged: true  # Required for XDP
          capabilities:
            add:
            - NET_ADMIN
            - BPF
        env:
        - name: XDP_INTERFACE
          value: "eth0"  # Or detect automatically
        - name: XDP_RATE_LIMIT
          value: "1000"
```

### Auto-Detect Interface

```go
// Find the default network interface
ifaces, _ := net.Interfaces()
for _, iface := range ifaces {
    if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
        xdpMgr.AttachToInterface(iface.Name)
        break
    }
}
```

## Advanced Use Cases

### 1. Geo-Blocking

Block traffic from specific countries:

```go
// Load IP ranges from GeoIP database
geoIPRanges := loadGeoIPRanges("CN", "RU")  // Block China, Russia
for _, ipRange := range geoIPRanges {
    for ip := ipRange.Start; ip <= ipRange.End; ip++ {
        xdpMgr.AddToBlacklist(intToIP(ip))
    }
}
```

### 2. Dynamic Blacklist from Threat Intel

```go
// Fetch threat intelligence feeds
threats := fetchThreatIntel("https://threatfeed.example.com/ips.txt")
for _, ip := range threats {
    xdpMgr.AddToBlacklist(ip)
}
```

### 3. Application-Layer DDoS (HTTP Flood)

XDP can't parse HTTP (too complex), but can detect patterns:

```c
// Detect small packets (likely HTTP GET floods)
if (ip->tot_len < 100 && tcp->dest == 80) {
    // Likely HTTP flood, apply stricter rate limit
    if (*pkt_count > 50) {
        return XDP_DROP;
    }
}
```

### 4. Load Balancing with XDP_REDIRECT

```c
// Redirect to different backend based on hash
__u32 hash = bpf_get_hash_recalc(ctx);
__u32 backend_idx = hash % NUM_BACKENDS;
return bpf_redirect(backend_ifaces[backend_idx], 0);
```

## Performance Tuning

### 1. XDP Modes

| Mode | Performance | Compatibility | Use Case |
|------|-------------|---------------|----------|
| **Native** | Highest (30 Mpps) | Requires driver support | Production |
| **Offload** | Extreme (100+ Mpps) | SmartNIC only | High-end |
| **Generic** | Lower (5 Mpps) | All drivers | Development |

```go
// Use native mode for best performance
link.AttachXDP(link.XDPOptions{
    Program:   xdpProg,
    Interface: ifaceIndex,
    Flags:     link.XDPDriverMode,  // Native mode
})
```

### 2. CPU Pinning

Pin XDP processing to specific CPUs:

```bash
# Pin IRQs to CPUs 0-3
echo 0-3 > /proc/irq/$(cat /proc/interrupts | grep eth0 | cut -d: -f1)/smp_affinity_list
```

### 3. NIC Tuning

```bash
# Increase ring buffer size
ethtool -G eth0 rx 4096 tx 4096

# Enable hardware offloading
ethtool -K eth0 gro on lro on
```

## Monitoring & Observability

### Prometheus Metrics

```go
// Export XDP stats to Prometheus
xdpTotalPackets := prometheus.NewGauge(prometheus.GaugeOpts{
    Name: "xdp_total_packets",
    Help: "Total packets processed by XDP",
})

xdpDropped := prometheus.NewGaugeVec(prometheus.GaugeOpts{
    Name: "xdp_dropped_packets",
    Help: "Packets dropped by XDP",
}, []string{"reason"})

// Update periodically
stats, _ := xdpMgr.GetStats()
xdpTotalPackets.Set(float64(stats.TotalPackets))
xdpDropped.WithLabelValues("blacklist").Set(float64(stats.DroppedBlacklist))
xdpDropped.WithLabelValues("ratelimit").Set(float64(stats.DroppedRateLimit))
```

### Grafana Dashboard

```json
{
  "title": "XDP DDoS Protection",
  "panels": [
    {
      "title": "Packet Drop Rate",
      "targets": [
        {
          "expr": "rate(xdp_dropped_packets[1m])"
        }
      ]
    },
    {
      "title": "Top Blocked IPs",
      "targets": [
        {
          "expr": "topk(10, xdp_blacklist_hits)"
        }
      ]
    }
  ]
}
```

## Limitations

### What XDP Can Do

✅ Drop packets based on IP/port  
✅ Rate limiting per source IP  
✅ SYN flood protection  
✅ Simple pattern matching  
✅ Packet redirection  

### What XDP Cannot Do

❌ **Deep packet inspection** (too complex for kernel)  
❌ **TLS decryption** (requires userspace)  
❌ **HTTP parsing** (use application layer)  
❌ **Stateful connection tracking** (limited memory)  

**Solution**: Combine XDP (L3/L4) with application-layer filtering (L7).

## Troubleshooting

### Issue: XDP program fails to load

**Error**: `permission denied`

**Solution**: Requires `CAP_NET_ADMIN` and `CAP_BPF`:
```bash
# Run as root or with capabilities
sudo setcap cap_net_admin,cap_bpf+ep ./uag
```

### Issue: XDP not attaching to interface

**Error**: `operation not supported`

**Solution**: Driver doesn't support native XDP, use generic mode:
```go
link.AttachXDP(link.XDPOptions{
    Flags: link.XDPGenericMode,  // Fallback mode
})
```

### Issue: High CPU usage with XDP

**Cause**: XDP program is too complex

**Solution**: Simplify logic, avoid loops:
```c
// BAD: Loop in XDP
for (int i = 0; i < 100; i++) { ... }

// GOOD: Direct lookup
bpf_map_lookup_elem(&blacklist, &ip);
```

## References

- [Linux XDP Documentation](https://www.kernel.org/doc/html/latest/networking/af_xdp.html)
- [Cilium XDP Tutorial](https://docs.cilium.io/en/stable/bpf/)
- [Cloudflare's XDP DDoS Protection](https://blog.cloudflare.com/l4drop-xdp-ebpf-based-ddos-mitigations/)
- [Facebook Katran Load Balancer](https://github.com/facebookincubator/katran)

## Next Steps

- [eBPF SockMap](EBPF.md) - Kernel-level socket redirection
- [Build Guide](BUILD.md) - Compile XDP programs
- [Deployment](../deploy/) - Kubernetes manifests

