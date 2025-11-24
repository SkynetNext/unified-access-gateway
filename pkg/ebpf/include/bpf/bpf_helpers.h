/* SPDX-License-Identifier: (LGPL-2.1 OR BSD-2-Clause) */
#ifndef __BPF_HELPERS_H
#define __BPF_HELPERS_H

/*
 * Minimal BPF helper definitions for sockmap programs
 * Vendored from libbpf to avoid external dependencies
 * Source: https://github.com/libbpf/libbpf
 */

/* Helper macro to place programs, maps, license in
 * different sections in elf_bpf file. Section names
 * are interpreted by elf_bpf loader
 */
#define SEC(NAME) __attribute__((section(NAME), used))

/* Helper macro to define BPF map attributes */
#define __uint(name, val) int(*name)[val]
#define __type(name, val) typeof(val) *name
#define __array(name, val) typeof(val) *name[]

/* Helper functions to manipulate eBPF maps */
static void *(*bpf_map_lookup_elem)(void *map, const void *key) = (void *)1;
static long (*bpf_map_update_elem)(void *map, const void *key,
                                   const void *value, __u64 flags) = (void *)2;
static long (*bpf_map_delete_elem)(void *map, const void *key) = (void *)3;

/* Socket operations */
static __u64 (*bpf_get_socket_cookie)(void *ctx) = (void *)46;
static long (*bpf_sk_redirect_hash)(void *ctx, void *map, void *key,
                                    __u64 flags) = (void *)72;
static long (*bpf_sock_hash_update)(void *ctx, void *map, void *key,
                                    __u64 flags) = (void *)70;
static __u64 (*bpf_get_socket_cookie_ops)(void *ctx) = (void *)46;

/* BPF map types */
enum bpf_map_type {
  BPF_MAP_TYPE_UNSPEC = 0,
  BPF_MAP_TYPE_HASH = 1,
  BPF_MAP_TYPE_ARRAY = 2,
  BPF_MAP_TYPE_SOCKHASH = 21,
};

/* BPF attach types */
enum bpf_attach_type {
  BPF_CGROUP_INET_INGRESS = 0,
  BPF_CGROUP_INET_EGRESS = 1,
  BPF_CGROUP_SOCK_OPS = 8,
  BPF_SK_SKB_STREAM_PARSER = 14,
  BPF_SK_SKB_STREAM_VERDICT = 15,
};

/* Socket operations */
enum {
  BPF_SOCK_OPS_VOID = 0,
  BPF_SOCK_OPS_TIMEOUT_INIT = 1,
  BPF_SOCK_OPS_RWND_INIT = 2,
  BPF_SOCK_OPS_TCP_CONNECT_CB = 3,
  BPF_SOCK_OPS_ACTIVE_ESTABLISHED_CB = 4,
  BPF_SOCK_OPS_PASSIVE_ESTABLISHED_CB = 5,
  BPF_SOCK_OPS_NEEDS_ECN = 6,
  BPF_SOCK_OPS_BASE_RTT = 7,
  BPF_SOCK_OPS_RTO_CB = 8,
  BPF_SOCK_OPS_RETRANS_CB = 9,
  BPF_SOCK_OPS_STATE_CB = 10,
  BPF_SOCK_OPS_TCP_LISTEN_CB = 11,
  BPF_SOCK_OPS_RTT_CB = 12,
};

/* TCP states */
enum {
  BPF_TCP_ESTABLISHED = 1,
  BPF_TCP_SYN_SENT = 2,
  BPF_TCP_SYN_RECV = 3,
  BPF_TCP_FIN_WAIT1 = 4,
  BPF_TCP_FIN_WAIT2 = 5,
  BPF_TCP_TIME_WAIT = 6,
  BPF_TCP_CLOSE = 7,
  BPF_TCP_CLOSE_WAIT = 8,
  BPF_TCP_LAST_ACK = 9,
  BPF_TCP_LISTEN = 10,
  BPF_TCP_CLOSING = 11,
  BPF_TCP_NEW_SYN_RECV = 12,
  BPF_TCP_MAX_STATES = 13,
};

/* Flags for BPF_MAP_UPDATE_ELEM */
enum {
  BPF_ANY = 0,
  BPF_NOEXIST = 1,
  BPF_EXIST = 2,
  BPF_F_LOCK = 4,
};

/* Flags for BPF_SK_REDIRECT */
enum {
  BPF_F_INGRESS = (1ULL << 0),
};

/* Return codes for SK_SKB programs */
enum {
  SK_DROP = 0,
  SK_PASS = 1,
};

#endif /* __BPF_HELPERS_H */
