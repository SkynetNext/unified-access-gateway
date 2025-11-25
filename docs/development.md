# Development

## Building from Source

### Prerequisites

- Go 1.21+
- Make (optional, for convenience commands)
- Clang 10+ (for eBPF compilation, Linux only)

### Basic Build

```bash
# Clone repository
git clone https://github.com/SkynetNext/unified-access-gateway.git
cd unified-access-gateway

# Download dependencies
go mod download

# Build
go build -o uag ./cmd/gateway

# Run
./uag
```

### Cross-Platform Build

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o uag-linux ./cmd/gateway

# Windows
GOOS=windows GOARCH=amd64 go build -o uag.exe ./cmd/gateway

# macOS
GOOS=darwin GOARCH=amd64 go build -o uag-darwin ./cmd/gateway
```

**Note**: eBPF is Linux-only. Windows/macOS builds automatically disable eBPF.

## eBPF Compilation

### Prerequisites

- Linux kernel 4.18+
- Clang 10+ with BPF target support
- `bpf2go` tool

### Install Dependencies

```bash
# Ubuntu/Debian
sudo apt-get update
sudo apt-get install -y clang llvm

# RHEL/CentOS
sudo yum install -y clang llvm

# Install bpf2go
go install github.com/cilium/ebpf/cmd/bpf2go@latest
export PATH=$PATH:$(go env GOPATH)/bin
```

### Generate eBPF Bindings

```bash
cd pkg/ebpf
export GOPACKAGE=ebpf

# Generate SockMap bindings
bpf2go -cc clang -target bpf -cflags "-O2 -g -Wall" bpf sockmap.c -- -I./include

# Generate XDP bindings
bpf2go -cc clang -target bpf -cflags "-O2 -g -Wall" xdp xdp_filter.c -- -I./include
```

### Build with eBPF

```bash
# After generating eBPF bindings
go build -o uag ./cmd/gateway
```

### Verify eBPF

```bash
# Check if eBPF programs are loaded
bpftool prog list | grep sock
bpftool prog list | grep xdp

# Check maps
bpftool map list | grep sock_map
```

## Vendored Headers

This project vendors all BPF headers in `pkg/ebpf/include/` to avoid system dependencies:

```
pkg/ebpf/include/
├── bpf/
│   ├── bpf_helpers.h
│   └── bpf_endian.h
└── linux/
    ├── bpf.h
    └── types.h
```

**Benefits**:
- No `libbpf-dev` or kernel headers required
- Consistent across environments
- Works on any Linux distro

## Docker Build

### Multi-Stage Build

```dockerfile
# Stage 1: eBPF builder
FROM golang:1.21-alpine AS ebpf-builder
RUN apk add --no-cache clang llvm
WORKDIR /build
COPY pkg/ebpf/ .
RUN export GOPACKAGE=ebpf && \
    bpf2go -cc clang -target bpf -cflags "-O2 -g" bpf sockmap.c -- -I./include && \
    bpf2go -cc clang -target bpf -cflags "-O2 -g" xdp xdp_filter.c -- -I./include

# Stage 2: Go builder
FROM golang:1.21-alpine AS go-builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ebpf-builder /build/*.go ./pkg/ebpf/
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o uag ./cmd/gateway

# Stage 3: Runtime
FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=go-builder /build/uag /usr/local/bin/uag
ENTRYPOINT ["/usr/local/bin/uag"]
```

### Build

```bash
docker build -t unified-access-gateway:latest .
```

## Testing

### Unit Tests

```bash
go test ./...
```

### Integration Tests

```bash
# Requires Redis
REDIS_ADDR=localhost:6379 go test ./internal/config/...
```

### eBPF Tests

```bash
# Requires Linux with eBPF support
sudo go test ./pkg/ebpf/...
```

## Code Quality

### Format

```bash
go fmt ./...
```

### Lint

```bash
# Install golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run
golangci-lint run
```

### Vet

```bash
go vet ./...
```

## CI/CD

### GitHub Actions Example

```yaml
name: Build

on: [push, pull_request]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - name: Install eBPF dependencies
        run: |
          sudo apt-get update
          sudo apt-get install -y clang-18 llvm-18
      - name: Install bpf2go
        run: go install github.com/cilium/ebpf/cmd/bpf2go@latest
      - name: Generate eBPF
        run: |
          cd pkg/ebpf
          export GOPACKAGE=ebpf
          bpf2go -cc clang-18 -target bpf -cflags "-O2 -g" bpf sockmap.c -- -I./include
          bpf2go -cc clang-18 -target bpf -cflags "-O2 -g" xdp xdp_filter.c -- -I./include
      - name: Build
        run: go build -o uag ./cmd/gateway
      - name: Test
        run: go test ./...
```

## Troubleshooting

### eBPF compilation fails

**Error**: `clang: command not found`

**Solution**: Install Clang (see Prerequisites above)

**Error**: `No available targets are compatible with triple "bpf"`

**Solution**: Use Clang 10+ with BPF support. Check with:
```bash
clang --print-targets | grep bpf
```

### Go build fails after eBPF generation

**Error**: `undefined: bpfObjects`

**Solution**: Ensure `bpf2go` generated files are present:
```bash
ls -la pkg/ebpf/*_bpf.go
```

### eBPF program fails to load

**Error**: `permission denied`

**Solution**: Requires `CAP_BPF` or run as root:
```bash
sudo setcap cap_bpf,cap_net_admin+ep ./uag
```

## Project Structure

```
unified-access-gateway/
├── cmd/
│   └── gateway/          # Main entry point
├── internal/
│   ├── config/          # Configuration management
│   ├── core/             # Core server logic
│   ├── discovery/        # Service discovery
│   ├── middleware/       # Middleware stack
│   ├── observability/    # Tracing and metrics
│   ├── protocol/         # Protocol handlers
│   └── security/         # Security features
├── pkg/
│   ├── ebpf/             # eBPF programs and loaders
│   └── xlog/             # Logging utilities
├── deploy/               # Kubernetes manifests
└── docs/                 # Documentation
```

