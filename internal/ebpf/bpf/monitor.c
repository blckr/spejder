// go:build ignore

#include <linux/bpf.h>
#include <linux/in.h>
#include <linux/in6.h>
#include <bpf/bpf_helpers.h>

// See tcp_states.h
#define TCP_ESTABLISHED 1
#define TCP_SYN_SENT    2
#define TCP_SYN_RECV    3
#define TCP_CLOSE       7
#define AF_INET         2
#define AF_INET6        10


// Event sent to userspace for each new incoming connection.
// src_ip holds 4 bytes for IPv4 (rest zeroed) or 16 bytes for IPv6.
struct connection_event {
    __u64 socket_id;
    __u8  local_ip[16];
    __u8  remote_ip[16];
    __u16 local_port;
    __u16 remote_port;
    __u8  event_type;    // 1 = established, 2 = closed
    __u8  old_state;     // state we transitioned from
    __u8  family;        // AF_INET or AF_INET6
    __u8  pad;
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
int trace_tcp_connect(struct inet_sock_set_state_args *ctx) {

    if (!(ctx->family == AF_INET || ctx->family == AF_INET6)) {
        return 0;
    }

    // 1 = established, 2 = closed
    __u8 event_type = 0;
    if (ctx->newstate == TCP_ESTABLISHED) {
        if (ctx->oldstate == TCP_SYN_RECV || ctx->oldstate == TCP_SYN_SENT) {
            event_type = 1; //start
        }
    } else if (ctx->newstate == TCP_CLOSE) {
        event_type = 2;
    }

    if (event_type == 0) {
        return 0;
    }

    struct connection_event *event = bpf_ringbuf_reserve(&connections, sizeof(*event), 0);
    if (!event) {
        return 0;
    }

    event->socket_id = (__u64) ctx->skaddr;
    event->local_port = ctx->sport;
    event->remote_port = ctx->dport;
    event->event_type = event_type;
    event->old_state = (__u8) ctx->oldstate;
    event->family = (__u8) ctx->family;

    if (ctx->family == AF_INET6) { //ipv6
        __builtin_memcpy(event->local_ip, ctx->saddr_v6, 16);
        __builtin_memcpy(event->remote_ip, ctx->daddr_v6, 16);
    } else { //ipv4
        __builtin_memcpy(event->local_ip, ctx->saddr, 4);
        __builtin_memset(event->local_ip + 4, 0, 12);

        __builtin_memcpy(event->remote_ip, ctx->daddr, 4);
        __builtin_memset(event->remote_ip + 4, 0, 12);
    }

    bpf_ringbuf_submit(event, 0);
    return 0;


}

char __license[] SEC("license") = "GPL";
