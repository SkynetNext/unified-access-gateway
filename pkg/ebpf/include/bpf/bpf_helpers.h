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
  BPF_MAP_TYPE_SOCKHASH = 18,
};

/* BPF attach types (sync with linux/bpf.h) */
enum bpf_attach_type {
  BPF_CGROUP_INET_INGRESS = 0,
  BPF_CGROUP_INET_EGRESS = 1,
  BPF_CGROUP_INET_SOCK_CREATE = 2,
  BPF_CGROUP_SOCK_OPS = 3,
  BPF_SK_SKB_STREAM_PARSER = 4,
  BPF_SK_SKB_STREAM_VERDICT = 5,
  BPF_CGROUP_DEVICE = 6,
  BPF_SK_MSG_VERDICT = 7,
  BPF_CGROUP_INET4_BIND = 8,
  BPF_CGROUP_INET6_BIND = 9,
  BPF_CGROUP_INET4_CONNECT = 10,
  BPF_CGROUP_INET6_CONNECT = 11,
  BPF_CGROUP_INET4_POST_BIND = 12,
  BPF_CGROUP_INET6_POST_BIND = 13,
  BPF_CGROUP_UDP4_SENDMSG = 14,
  BPF_CGROUP_UDP6_SENDMSG = 15,
  BPF_LIRC_MODE2 = 16,
  BPF_FLOW_DISSECTOR = 17,
  BPF_CGROUP_SYSCTL = 18,
  BPF_CGROUP_UDP4_RECVMSG = 19,
  BPF_CGROUP_UDP6_RECVMSG = 20,
  BPF_CGROUP_GETSOCKOPT = 21,
  BPF_CGROUP_SETSOCKOPT = 22,
  BPF_TRACE_RAW_TP = 23,
  BPF_TRACE_FENTRY = 24,
  BPF_TRACE_FEXIT = 25,
  BPF_MODIFY_RETURN = 26,
  BPF_LSM_MAC = 27,
  BPF_TRACE_ITER = 28,
  BPF_CGROUP_INET4_GETPEERNAME = 29,
  BPF_CGROUP_INET6_GETPEERNAME = 30,
  BPF_CGROUP_INET4_GETSOCKNAME = 31,
  BPF_CGROUP_INET6_GETSOCKNAME = 32,
  BPF_XDP_DEVMAP = 33,
  BPF_CGROUP_INET_SOCK_RELEASE = 34,
  BPF_XDP_CPUMAP = 35,
  BPF_SK_LOOKUP = 36,
  BPF_XDP = 37,
  BPF_SK_SKB_VERDICT = 38,
  BPF_SK_REUSEPORT_SELECT = 39,
  BPF_SK_REUSEPORT_SELECT_OR_MIGRATE = 40,
  BPF_PERF_EVENT = 41,
  BPF_TRACE_KPROBE_MULTI = 42,
  BPF_LSM_CGROUP = 43,
  BPF_STRUCT_OPS = 44,
  BPF_NETFILTER = 45,
  BPF_TCX_INGRESS = 46,
  BPF_TCX_EGRESS = 47,
  BPF_TRACE_UPROBE_MULTI = 48,
  BPF_CGROUP_UNIX_CONNECT = 49,
  BPF_CGROUP_UNIX_SENDMSG = 50,
  BPF_CGROUP_UNIX_RECVMSG = 51,
  BPF_CGROUP_UNIX_GETPEERNAME = 52,
  BPF_CGROUP_UNIX_GETSOCKNAME = 53,
  BPF_NETKIT_PRIMARY = 54,
  BPF_NETKIT_PEER = 55,
  BPF_TRACE_KPROBE_SESSION = 56,
  __MAX_BPF_ATTACH_TYPE = 57,
};

/* Socket operations (sync with linux/bpf.h) */
enum {
  BPF_SOCK_OPS_VOID = 0,
  BPF_SOCK_OPS_TIMEOUT_INIT,
  BPF_SOCK_OPS_RWND_INIT,
  BPF_SOCK_OPS_TCP_CONNECT_CB,
  BPF_SOCK_OPS_ACTIVE_ESTABLISHED_CB,
  BPF_SOCK_OPS_PASSIVE_ESTABLISHED_CB,
  BPF_SOCK_OPS_NEEDS_ECN,
  BPF_SOCK_OPS_BASE_RTT,
  BPF_SOCK_OPS_RTO_CB,
  BPF_SOCK_OPS_RETRANS_CB,
  BPF_SOCK_OPS_STATE_CB,
  BPF_SOCK_OPS_TCP_LISTEN_CB,
  BPF_SOCK_OPS_RTT_CB,
  BPF_SOCK_OPS_PARSE_HDR_OPT_CB,
  BPF_SOCK_OPS_HDR_OPT_LEN_CB,
  BPF_SOCK_OPS_WRITE_HDR_OPT_CB,
};

/* TCP states */
enum {
	BPF_TCP_ESTABLISHED = 1,
	BPF_TCP_SYN_SENT,
	BPF_TCP_SYN_RECV,
	BPF_TCP_FIN_WAIT1,
	BPF_TCP_FIN_WAIT2,
	BPF_TCP_TIME_WAIT,
	BPF_TCP_CLOSE,
	BPF_TCP_CLOSE_WAIT,
	BPF_TCP_LAST_ACK,
	BPF_TCP_LISTEN,
	BPF_TCP_CLOSING,	/* Now a valid state */
	BPF_TCP_NEW_SYN_RECV,

	BPF_TCP_MAX_STATES	/* Leave at the end! */
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
enum sk_action {
	SK_DROP = 0,
	SK_PASS,
};

#endif /* __BPF_HELPERS_H */
