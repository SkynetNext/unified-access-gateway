// SPDX-License-Identifier: GPL-2.0
// eBPF SockMap program for socket redirection
// This program redirects traffic between client and backend sockets at kernel
// level

// Use vendored headers (no external dependencies)
#include "include/bpf/bpf_endian.h"
#include "include/bpf/bpf_helpers.h"
#include "include/linux/bpf.h"
#include "include/linux/types.h"

// Map to store socket file descriptors
// Key: socket cookie - using __u32 instead of __u64 for SOCKHASH compatibility
// Value: socket reference (SOCKHASH stores socket, not fd)
// Note: Some kernels require key_size=4 for SOCKHASH
struct {
  __uint(type, BPF_MAP_TYPE_SOCKHASH);
  __uint(max_entries, 65535);
  __uint(key_size, 4);   // Try 4 bytes (some kernels require this)
  __uint(value_size, 4); // Must be 4 bytes for SOCKHASH
} sock_map SEC(".maps");

// Map to store socket pair relationships
// Key: client socket cookie (__u32 to match sock_map)
// Value: backend socket cookie
struct {
  __uint(type, BPF_MAP_TYPE_HASH);
  __uint(max_entries, 65535);
  __uint(key_size, 4);   // Match sock_map key size
  __uint(value_size, 4); // Match sock_map key size
} sock_pair_map SEC(".maps");

// Parser program: extract socket key (cookie)
SEC("sk_skb/stream_parser")
int sock_stream_parser(struct __sk_buff *skb) {
  // Always accept the packet for verdict program
  return skb->len;
}

// Verdict program: decide where to redirect the packet
SEC("sk_skb/stream_verdict")
int sock_stream_verdict(struct __sk_buff *skb) {
  __u32 key = (__u32)bpf_get_socket_cookie(skb); // Use __u32 to match map
  __u32 *peer_key;

  // Lookup peer socket cookie from pair map
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

// Sockops program: intercept socket operations
SEC("sockops")
int sock_ops_handler(struct bpf_sock_ops *skops) {
  __u32 op = skops->op;
  __u32 key; // Changed from __u64 to __u32 to match map key_size

  switch (op) {
  case BPF_SOCK_OPS_PASSIVE_ESTABLISHED_CB:
  case BPF_SOCK_OPS_ACTIVE_ESTABLISHED_CB:
    // Socket established, add to sockmap
    // Use lower 32 bits of cookie as key
    key = (__u32)bpf_get_socket_cookie_ops(skops);
    bpf_sock_hash_update(skops, &sock_map, &key, BPF_NOEXIST);
    break;

  case BPF_SOCK_OPS_STATE_CB:
    // Socket state changed (e.g., closed)
    if (skops->args[1] == BPF_TCP_CLOSE) {
      key = (__u32)bpf_get_socket_cookie_ops(skops);
      // Remove from maps (cleanup)
      bpf_map_delete_elem(&sock_map, &key);
      bpf_map_delete_elem(&sock_pair_map, &key);
    }
    break;
  }

  return 1;
}

char _license[] SEC("license") = "GPL";
