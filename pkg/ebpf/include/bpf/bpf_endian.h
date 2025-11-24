/* SPDX-License-Identifier: (LGPL-2.1 OR BSD-2-Clause) */
#ifndef __BPF_ENDIAN_H
#define __BPF_ENDIAN_H

/*
 * Minimal endianness conversion helpers
 * Vendored from libbpf
 */

#include <linux/types.h>

/* Byte swap macros */
#define ___bpf_swab16(x)                                                       \
  ((__u16)((((__u16)(x) & (__u16)0x00ffU) << 8) |                              \
           (((__u16)(x) & (__u16)0xff00U) >> 8)))

#define ___bpf_swab32(x)                                                       \
  ((__u32)((((__u32)(x) & (__u32)0x000000ffUL) << 24) |                        \
           (((__u32)(x) & (__u32)0x0000ff00UL) << 8) |                         \
           (((__u32)(x) & (__u32)0x00ff0000UL) >> 8) |                         \
           (((__u32)(x) & (__u32)0xff000000UL) >> 24)))

#define ___bpf_swab64(x)                                                       \
  ((__u64)((((__u64)(x) & (__u64)0x00000000000000ffULL) << 56) |               \
           (((__u64)(x) & (__u64)0x000000000000ff00ULL) << 40) |               \
           (((__u64)(x) & (__u64)0x0000000000ff0000ULL) << 24) |               \
           (((__u64)(x) & (__u64)0x00000000ff000000ULL) << 8) |                \
           (((__u64)(x) & (__u64)0x000000ff00000000ULL) >> 8) |                \
           (((__u64)(x) & (__u64)0x0000ff0000000000ULL) >> 24) |               \
           (((__u64)(x) & (__u64)0x00ff000000000000ULL) >> 40) |               \
           (((__u64)(x) & (__u64)0xff00000000000000ULL) >> 56)))

/* Endianness detection */
#if __BYTE_ORDER__ == __ORDER_LITTLE_ENDIAN__
#define __bpf_ntohs(x) ___bpf_swab16(x)
#define __bpf_htons(x) ___bpf_swab16(x)
#define __bpf_ntohl(x) ___bpf_swab32(x)
#define __bpf_htonl(x) ___bpf_swab32(x)
#define __bpf_be64_to_cpu(x) ___bpf_swab64(x)
#define __bpf_cpu_to_be64(x) ___bpf_swab64(x)
#elif __BYTE_ORDER__ == __ORDER_BIG_ENDIAN__
#define __bpf_ntohs(x) (x)
#define __bpf_htons(x) (x)
#define __bpf_ntohl(x) (x)
#define __bpf_htonl(x) (x)
#define __bpf_be64_to_cpu(x) (x)
#define __bpf_cpu_to_be64(x) (x)
#else
#error "Unsupported endianness"
#endif

#define bpf_ntohs(x) __bpf_ntohs(x)
#define bpf_htons(x) __bpf_htons(x)
#define bpf_ntohl(x) __bpf_ntohl(x)
#define bpf_htonl(x) __bpf_htonl(x)
#define bpf_be64_to_cpu(x) __bpf_be64_to_cpu(x)
#define bpf_cpu_to_be64(x) __bpf_cpu_to_be64(x)

#endif /* __BPF_ENDIAN_H */
