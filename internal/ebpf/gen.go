package ebpf

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc $BPF_CLANG -cflags "-O2 -g -target bpf -D__TARGET_ARCH_x86_64 -I$LIBBPF_INCLUDE -I$LINUX_INCLUDE" Monitor bpf/monitor.c
