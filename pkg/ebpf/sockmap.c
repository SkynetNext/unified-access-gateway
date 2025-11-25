// SPDX-License-Identifier: GPL-2.0
// eBPF SockMap program for socket redirection
// This program redirects traffic between client and backend sockets at kernel
// level

// Use vendored headers (no external dependencies)
#include "include/bpf/bpf_endian.h"
#include "include/bpf/bpf_helpers.h"
#include "include/linux/bpf.h"
#include "include/linux/types.h"

// Socket key structure (following Cilium's approach)
// Using 5-tuple: src_ip, dst_ip, src_port, dst_port, family
struct sock_key {
  __u32 sip4;  // Source IPv4
  __u32 dip4;  // Destination IPv4
  __u32 sport; // Source port
  __u32 dport; // Destination port
  __u8 family; // Address family (AF_INET)
  __u8 pad1;
  __u16 pad2;
} __attribute__((packed));

// Map to store socket file descriptors
// Following Cilium's design: using 5-tuple as key
// Reference:
// https://github.com/cilium/cilium/blob/v1.13/bpf/sockops/bpf_sockops.c
struct {
  __uint(type, BPF_MAP_TYPE_SOCKHASH);
  __uint(max_entries, 65535);
  __type(key, struct sock_key);
  __type(value, int); // Cilium uses int, not __u64
} sock_map SEC(".maps");

// Map to store socket pair relationships
// Key: client socket key (5-tuple)
// Value: backend socket key (5-tuple)
struct {
  __uint(type, BPF_MAP_TYPE_HASH);
  __uint(max_entries, 65535);
  __type(key, struct sock_key);
  __type(value, struct sock_key);
} sock_pair_map SEC(".maps");

// Parser program: extract socket key (cookie)
SEC("sk_skb/stream_parser")
int sock_stream_parser(struct __sk_buff *skb) {
  // Always accept the packet for verdict program
  return skb->len;
}

// Helper: extract socket key from skb
static __always_inline void sk_extract_key(struct __sk_buff *skb,
                                           struct sock_key *key) {
  key->sip4 = skb->remote_ip4;
  key->dip4 = skb->local_ip4;
  key->sport = bpf_ntohl(skb->remote_port);
  key->dport = skb->local_port >> 16;
  key->family = skb->family;
}

// Verdict program: decide where to redirect the packet
SEC("sk_skb/stream_verdict")
int sock_stream_verdict(struct __sk_buff *skb) {
  struct sock_key key = {};
  struct sock_key *peer_key;

  // Extract 5-tuple from skb
  sk_extract_key(skb, &key);

  // Lookup peer socket key from pair map
  peer_key = bpf_map_lookup_elem(&sock_pair_map, &key);
  if (!peer_key) {
    // No peer found, pass to userspace
    return SK_PASS;
  }

  // Redirect to peer socket (kernel-level forwarding)
  long ret = bpf_sk_redirect_hash(skb, &sock_map, peer_key, BPF_F_INGRESS);
  if (ret == SK_PASS) {
    // Redirect succeeded
    return SK_PASS;
  }

  // Redirect failed, pass to userspace
  return SK_PASS;
}

// Helper: extract socket key from sock_ops
static __always_inline void sk_extract_key_ops(struct bpf_sock_ops *skops,
                                               struct sock_key *key) {
  key->sip4 = skops->remote_ip4;
  key->dip4 = skops->local_ip4;
  key->sport = skops->remote_port;
  key->dport = bpf_ntohl(skops->local_port);
  key->family = skops->family;
}

// Sockops program: intercept socket operations (following Cilium's approach)
SEC("sockops")
int sock_ops_handler(struct bpf_sock_ops *skops) {
  __u32 op = skops->op;
  struct sock_key key = {};

  switch (op) {
  case BPF_SOCK_OPS_PASSIVE_ESTABLISHED_CB:
  case BPF_SOCK_OPS_ACTIVE_ESTABLISHED_CB:
    // Only handle IPv4 TCP connections
    if (skops->family != AF_INET) {
      break;
    }
    // Socket established, add to sockmap using 5-tuple as key
    sk_extract_key_ops(skops, &key);
    bpf_sock_hash_update(skops, &sock_map, &key, BPF_NOEXIST);
    break;

  case BPF_SOCK_OPS_STATE_CB:
    // Socket state changed (e.g., closed)
    if (skops->args[1] == BPF_TCP_CLOSE && skops->family == AF_INET) {
      sk_extract_key_ops(skops, &key);
      // Remove from maps (cleanup)
      bpf_map_delete_elem(&sock_map, &key);
      bpf_map_delete_elem(&sock_pair_map, &key);
    }
    break;
  }

  return 0; // Cilium returns 0, not 1
}

char _license[] SEC("license") = "GPL";
