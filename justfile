binary := "collect"
db     := "spejder.db"

setup:
    @test -f GeoIP.conf || (echo "Copy GeoIP.conf.example to GeoIP.conf and fill in your credentials" && exit 1)
    mkdir -p assets/geo
    geoipupdate -f GeoIP.conf

generate:
    go generate ./internal/ebpf/...

build: generate
    GOOS=linux GOARCH=amd64 go build -o {{binary}} ./cmd/collect/

run: build
    sudo ./{{binary}}

clean:
    rm -f {{binary}} {{db}}
