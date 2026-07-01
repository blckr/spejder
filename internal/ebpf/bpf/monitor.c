// go:build ignore

#include <linux/bpf.h>
#include <linux/in.h>
#include <linux/in6.h>
#include <bpf/bpf_helpers.h>

// SYN_RECV → ESTABLISHED: someone connected to us (incoming)
// SYN_SENT → ESTABLISHED: we connected to someone (outgoing, ignored)
#define TCP_ESTABLISHED 1
#define TCP_SYN_RECV    3
#define AF_INET         2
#define AF_INET6        10

// Event sent to userspace for each new incoming connection.
// src_ip holds 4 bytes for IPv4 (rest zeroed) or 16 bytes for IPv6.
struct connection_event {
    __u8  src_ip[16];
    __u16 dst_port;  // our listening port (22, 80, 443, …)
    __u8  proto;
    __u8  family;    // AF_INET or AF_INET6
};

// Ring buffer map — the queue from kernel to userspace.
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 1 << 24); // 16 MB buffer
} connections SEC(".maps");

// See: cat /sys/kernel/tracing/events/sock/inet_sock_set_state/format
struct inet_sock_set_state_args {
    // First 8 bytes: internal trace metadata, ignored
    __u16 common_type;
    __u8  common_flags;
    __u8  common_preempt_count;
    int   common_pid;

    const void *skaddr;
    int  oldstate;
    int  newstate;
    __u16 sport;
    __u16 dport;
    __u16 family;
    __u16 protocol;
    __u8  saddr[4];
    __u8  daddr[4];
    __u8  saddr_v6[16];
    __u8  daddr_v6[16];
};

SEC("tracepoint/sock/inet_sock_set_state")
int trace_tcp_connect(struct inet_sock_set_state_args *ctx)
{
    if (ctx->newstate != TCP_ESTABLISHED)
        return 0;
    if (ctx->oldstate != TCP_SYN_RECV)
        return 0;
    if (ctx->family != AF_INET && ctx->family != AF_INET6)
        return 0;

    struct connection_event *e = bpf_ringbuf_reserve(&connections, sizeof(*e), 0);
    if (!e)
        return 0;

    e->dst_port = ctx->sport;
    e->proto    = (__u8)ctx->protocol;
    e->family   = (__u8)ctx->family;

    if (ctx->family == AF_INET6) {
        __builtin_memcpy(e->src_ip, ctx->daddr_v6, 16);
    } else {
        __builtin_memcpy(e->src_ip, ctx->daddr, 4);
        __builtin_memset(e->src_ip + 4, 0, 12);
    }

    bpf_ringbuf_submit(e, 0);
    return 0;
}

char __license[] SEC("license") = "GPL";
