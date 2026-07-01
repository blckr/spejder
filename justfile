daemon := "collect"
tui    := "tui"
db     := "spejder.db"

# Show available recipes
default:
    @just --list

# Download/update GeoIP databases (run once before first start)
setup:
    @test -f GeoIP.conf || (echo "Copy GeoIP.conf.example to GeoIP.conf and fill in your credentials" && exit 1)
    mkdir -p assets/geo
    geoipupdate -f GeoIP.conf

# Compile eBPF C code → Go bindings (requires nix devshell for BPF_CLANG etc.)
ebpf:
    @test -n "$BPF_CLANG"       || (echo "BPF_CLANG not set — run 'nix develop' first" && exit 1)
    @test -n "$LIBBPF_INCLUDE"  || (echo "LIBBPF_INCLUDE not set — run 'nix develop' first" && exit 1)
    @test -n "$LINUX_INCLUDE"   || (echo "LINUX_INCLUDE not set — run 'nix develop' first" && exit 1)
    go generate ./internal/ebpf/...

# Build monitoring daemon (without recompiling eBPF)
build-daemon:
    GOOS=linux GOARCH=amd64 go build -o {{daemon}} ./cmd/collect/

# Build TUI
build-tui:
    GOOS=linux GOARCH=amd64 go build -o {{tui}} ./cmd/tui/

# Build everything including eBPF (full build)
build: ebpf build-daemon build-tui

# Start monitoring daemon (requires root)
run: build-daemon
    sudo ./{{daemon}}

# Start TUI
run-tui: build-tui
    ./{{tui}}

# Remove build artifacts and database
clean:
    rm -f {{daemon}} {{tui}} {{db}}
