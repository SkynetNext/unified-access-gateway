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
// Key: socket cookie (unique identifier)
// Value: socket (not fd, SOCKHASH stores socket references)
// Note: SOCKHASH value_size must be sizeof(int) for socket reference
struct {
  __uint(type, BPF_MAP_TYPE_SOCKHASH);
  __uint(max_entries, 65535);
  __uint(key_size, sizeof(__u64)); // socket cookie
  __uint(value_size, sizeof(int)); // socket reference (required for SOCKHASH)
} sock_map SEC(".maps");

// Map to store socket pair relationships
// Key: client socket cookie
// Value: backend socket cookie
struct {
  __uint(type, BPF_MAP_TYPE_HASH);
  __uint(max_entries, 65535);
  __uint(key_size, sizeof(__u64));
  __uint(value_size, sizeof(__u64));
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
  __u64 cookie = bpf_get_socket_cookie(skb);
  __u64 *peer_cookie;

  // Lookup peer socket cookie from pair map
  peer_cookie = bpf_map_lookup_elem(&sock_pair_map, &cookie);
  if (!peer_cookie) {
    // No peer found, pass to userspace
    return SK_PASS;
  }

  // Redirect to peer socket (kernel-level forwarding)
  long ret = bpf_sk_redirect_hash(skb, &sock_map, peer_cookie, BPF_F_INGRESS);
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
  __u64 cookie;

  switch (op) {
  case BPF_SOCK_OPS_PASSIVE_ESTABLISHED_CB:
  case BPF_SOCK_OPS_ACTIVE_ESTABLISHED_CB:
    // Socket established, add to sockmap
    cookie = bpf_get_socket_cookie_ops(skops);
    bpf_sock_hash_update(skops, &sock_map, &cookie, BPF_NOEXIST);
    break;

  case BPF_SOCK_OPS_STATE_CB:
    // Socket state changed (e.g., closed)
    if (skops->args[1] == BPF_TCP_CLOSE) {
      cookie = bpf_get_socket_cookie_ops(skops);
      // Remove from maps (cleanup)
      bpf_map_delete_elem(&sock_map, &cookie);
      bpf_map_delete_elem(&sock_pair_map, &cookie);
    }
    break;
  }

  return 1;
}

char _license[] SEC("license") = "GPL";
