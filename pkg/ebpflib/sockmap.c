// +build ignore

#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

struct {
    __uint(type, BPF_MAP_TYPE_SOCKMAP);
    __uint(max_entries, 65535);
    __type(key, __u32);
    __type(value, __u64);
} sock_ops_map SEC(".maps");

SEC("sk_msg")
int bpf_redir(struct sk_msg_md *msg)
{
    // 获取目标 Socket 的 Key (这里简化处理，实际需要根据路由表查找)
    __u32 key = 0; // 假设 key 0 是目标

    // 直接重定向到目标 Socket
    return bpf_msg_redirect_map(msg, &sock_ops_map, key, 0);
}

char _license[] SEC("license") = "GPL";

