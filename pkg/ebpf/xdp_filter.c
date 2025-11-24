// SPDX-License-Identifier: GPL-2.0
// XDP (eXpress Data Path) program for early packet filtering and DDoS
// protection This program runs at the network driver layer, before the kernel
// network stack

#include "include/bpf/bpf_endian.h"
#include "include/bpf/bpf_helpers.h"
#include "include/linux/bpf.h"
#include "include/linux/types.h"

// Ethernet header
struct ethhdr {
  __u8 h_dest[6];   // Destination MAC
  __u8 h_source[6]; // Source MAC
  __u16 h_proto;    // Protocol (e.g., 0x0800 for IPv4)
} __attribute__((packed));

// IPv4 header (simplified)
struct iphdr {
  __u8 ihl : 4;     // Header length
  __u8 version : 4; // Version (4 for IPv4)
  __u8 tos;         // Type of service
  __u16 tot_len;    // Total length
  __u16 id;         // Identification
  __u16 frag_off;   // Fragment offset
  __u8 ttl;         // Time to live
  __u8 protocol;    // Protocol (6=TCP, 17=UDP)
  __u16 check;      // Checksum
  __u32 saddr;      // Source IP
  __u32 daddr;      // Destination IP
} __attribute__((packed));

// TCP header (simplified)
struct tcphdr {
  __u16 source; // Source port
  __u16 dest;   // Destination port
  __u32 seq;    // Sequence number
  __u32 ack_seq;
  __u16 res1 : 4, doff : 4, fin : 1, syn : 1, rst : 1, psh : 1, ack : 1,
      urg : 1, ece : 1, cwr : 1;
  __u16 window;
  __u16 check;
  __u16 urg_ptr;
} __attribute__((packed));

// Protocol constants
#define ETH_P_IP 0x0800 // IPv4
#define IPPROTO_TCP 6   // TCP
#define IPPROTO_UDP 17  // UDP

// XDP action codes
#define XDP_ABORTED 0  // Error, drop packet
#define XDP_DROP 1     // Drop packet
#define XDP_PASS 2     // Pass to kernel stack
#define XDP_TX 3       // Transmit from same interface
#define XDP_REDIRECT 4 // Redirect to another interface

// Map: IP blacklist (for DDoS protection)
// Key: IPv4 address (__u32)
// Value: 1 (blocked)
struct {
  __uint(type, BPF_MAP_TYPE_HASH);
  __uint(max_entries, 10000);
  __type(key, __u32);
  __type(value, __u8);
} ip_blacklist SEC(".maps");

// Map: Rate limiting per source IP
// Key: IPv4 address
// Value: packet count in current time window
struct {
  __uint(type, BPF_MAP_TYPE_HASH);
  __uint(max_entries, 65536);
  __type(key, __u32);
  __type(value, __u64);
} rate_limit_map SEC(".maps");

// Map: Statistics
struct {
  __uint(type, BPF_MAP_TYPE_ARRAY);
  __uint(max_entries, 10);
  __type(key, __u32);
  __type(value, __u64);
} stats_map SEC(".maps");

// Statistics indices
#define STAT_TOTAL_PACKETS 0
#define STAT_DROPPED_BLACKLIST 1
#define STAT_DROPPED_RATELIMIT 2
#define STAT_DROPPED_INVALID 3
#define STAT_PASSED 4
#define STAT_TCP_SYN 5
#define STAT_TCP_SYN_FLOOD 6

// Configuration (can be updated from userspace)
struct {
  __uint(type, BPF_MAP_TYPE_ARRAY);
  __uint(max_entries, 1);
  __type(key, __u32);
  __type(value, __u64);
} config_map SEC(".maps");

#define RATE_LIMIT_THRESHOLD 1000 // Max packets per IP per second

// Helper: Update statistics counter
static __always_inline void update_stat(__u32 stat_id) {
  __u64 *count = bpf_map_lookup_elem(&stats_map, &stat_id);
  if (count) {
    __sync_fetch_and_add(count, 1);
  }
}

// Main XDP program
SEC("xdp")
int xdp_filter_prog(struct xdp_md *ctx) {
  // Packet boundaries
  void *data_end = (void *)(long)ctx->data_end;
  void *data = (void *)(long)ctx->data;

  // Update total packet counter
  update_stat(STAT_TOTAL_PACKETS);

  // Parse Ethernet header
  struct ethhdr *eth = data;
  if ((void *)(eth + 1) > data_end) {
    return XDP_DROP; // Packet too short
  }

  // Only process IPv4 packets
  if (bpf_ntohs(eth->h_proto) != ETH_P_IP) {
    return XDP_PASS; // Pass non-IPv4 (e.g., ARP, IPv6)
  }

  // Parse IP header
  struct iphdr *ip = (void *)(eth + 1);
  if ((void *)(ip + 1) > data_end) {
    update_stat(STAT_DROPPED_INVALID);
    return XDP_DROP; // Invalid packet
  }

  __u32 src_ip = ip->saddr;

  // 1. Check IP blacklist (DDoS mitigation)
  __u8 *blocked = bpf_map_lookup_elem(&ip_blacklist, &src_ip);
  if (blocked && *blocked == 1) {
    update_stat(STAT_DROPPED_BLACKLIST);
    return XDP_DROP; // Drop blacklisted IP
  }

  // 2. Rate limiting per source IP
  __u64 *pkt_count = bpf_map_lookup_elem(&rate_limit_map, &src_ip);
  if (pkt_count) {
    if (*pkt_count > RATE_LIMIT_THRESHOLD) {
      update_stat(STAT_DROPPED_RATELIMIT);
      return XDP_DROP; // Rate limit exceeded
    }
    __sync_fetch_and_add(pkt_count, 1);
  } else {
    // First packet from this IP, initialize counter
    __u64 init_count = 1;
    bpf_map_update_elem(&rate_limit_map, &src_ip, &init_count, BPF_ANY);
  }

  // 3. TCP SYN flood protection
  if (ip->protocol == IPPROTO_TCP) {
    struct tcphdr *tcp = (void *)ip + (ip->ihl * 4);
    if ((void *)(tcp + 1) > data_end) {
      update_stat(STAT_DROPPED_INVALID);
      return XDP_DROP;
    }

    // Detect SYN packets
    if (tcp->syn && !tcp->ack) {
      update_stat(STAT_TCP_SYN);

      // Check if this IP is sending too many SYNs
      if (pkt_count && *pkt_count > 100) { // 100 SYNs per second threshold
        update_stat(STAT_TCP_SYN_FLOOD);
        // Add to blacklist temporarily
        __u8 block = 1;
        bpf_map_update_elem(&ip_blacklist, &src_ip, &block, BPF_ANY);
        return XDP_DROP;
      }
    }
  }

  // 4. Pass legitimate traffic to kernel stack
  update_stat(STAT_PASSED);
  return XDP_PASS;
}

char _license[] SEC("license") = "GPL";
