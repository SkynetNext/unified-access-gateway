# Build Guide

## Prerequisites

### Required Tools

| Tool | Version | Purpose | Installation |
|------|---------|---------|--------------|
| **Go** | 1.21+ | Build gateway | [go.dev](https://go.dev/dl/) |
| **Clang** | 10+ | Compile eBPF | `apt install clang` |
| **Make** | Any | Build automation | `apt install make` |

### Optional Tools

| Tool | Purpose | Installation |
|------|---------|--------------|
| **Docker** | Container build | [docker.com](https://docs.docker.com/get-docker/) |
| **kubectl** | K8s deployment | [kubernetes.io](https://kubernetes.io/docs/tasks/tools/) |

---

## Quick Start (No eBPF)

If you just want to test the gateway **without eBPF acceleration**:

```bash
# 1. Clone repository
git clone https://github.com/SkynetNext/unified-access-gateway.git
cd unified-access-gateway

# 2. Download Go dependencies
go mod download

# 3. Build (eBPF will auto-disable if not available)
go build -o uag ./cmd/gateway

# 4. Run
./uag
```

The gateway will automatically fall back to userspace proxying.

---

## Full Build (With eBPF)

### Step 1: Install Clang

#### Ubuntu/Debian
```bash
sudo apt-get update
sudo apt-get install -y clang llvm
```

#### RHEL/CentOS
```bash
sudo yum install -y clang llvm
```

#### macOS
```bash
brew install llvm
```

#### Windows (WSL2)
```bash
# Use WSL2 Ubuntu and follow Ubuntu instructions
wsl --install -d Ubuntu-22.04
```

### Step 2: Install bpf2go

```bash
go install github.com/cilium/ebpf/cmd/bpf2go@latest
```

### Step 3: Generate eBPF Bindings

```bash
make generate-ebpf
```

This will:
1. Compile `pkg/ebpf/sockmap.c` to eBPF bytecode
2. Generate `pkg/ebpf/bpf_bpfel.go` (little-endian)
3. Generate `pkg/ebpf/bpf_bpfeb.go` (big-endian)

### Step 4: Build Gateway

```bash
make build
```

---

## Vendored Headers (Zero Dependencies)

This project **vendors all BPF headers** in `pkg/ebpf/include/`, so you don't need to install `libbpf-dev` or kernel headers!

```
pkg/ebpf/include/
├── bpf/
│   ├── bpf_helpers.h    # BPF helper functions
│   └── bpf_endian.h     # Endianness conversion
└── linux/
    ├── bpf.h            # BPF UAPI definitions
    └── types.h          # Kernel types
```

**Benefits**:
- ✅ No system dependencies
- ✅ Consistent across environments
- ✅ Works on any Linux distro
- ✅ Easy CI/CD integration

---

## Build Targets

### Local Development

```bash
# Build for current platform
make build

# Build for Linux (from macOS/Windows)
make build-linux

# Build for Windows
make build-windows
```

### eBPF Development

```bash
# Regenerate eBPF bindings after modifying sockmap.c
make generate-ebpf

# Clean generated files
make clean
```

### Testing

```bash
# Run unit tests
make test

# Generate coverage report
make test-coverage
```

### Code Quality

```bash
# Format Go code
make fmt

# Run linters (requires golangci-lint)
make lint
```

---

## Docker Build

### Standard Build

```bash
make docker-build
```

This creates a multi-stage image:
1. **Stage 1**: Compile eBPF program with Clang
2. **Stage 2**: Build Go binary
3. **Stage 3**: Minimal Alpine runtime image

### Run Container

```bash
make docker-run
```

Or manually:

```bash
docker run -d \
  --name uag \
  --privileged \
  -p 8080:8080 \
  -p 9090:9090 \
  -v /sys/fs/cgroup:/sys/fs/cgroup:ro \
  -e HTTP_BACKEND_URL="http://backend:5000" \
  -e TCP_BACKEND_ADDR="backend:6000" \
  skynet/unified-access-gateway:latest
```

**Note**: `--privileged` is only needed for eBPF. Without it, the gateway falls back to userspace.

---

## Kubernetes Deployment

### Prerequisites

- Kubernetes 1.20+
- Nodes with Linux Kernel 4.18+ (for eBPF)

### Deploy

```bash
make k8s-deploy
```

Or manually:

```bash
kubectl apply -f deploy/deployment.yaml
kubectl apply -f deploy/service.yaml
kubectl apply -f deploy/hpa.yaml
```

### Verify

```bash
# Check pods
kubectl get pods -l app=unified-access-gateway

# Check logs
kubectl logs -f deployment/unified-access-gateway

# Check metrics
kubectl port-forward svc/unified-access-gateway 9090:9090
curl http://localhost:9090/metrics
```

---

## Troubleshooting

### Issue: `clang: command not found`

**Solution**: Install Clang (see Step 1 above)

### Issue: `bpf2go: command not found`

**Solution**:
```bash
go install github.com/cilium/ebpf/cmd/bpf2go@latest
# Ensure $GOPATH/bin is in your PATH
export PATH=$PATH:$(go env GOPATH)/bin
```

### Issue: eBPF compilation fails with "unknown type name"

**Solution**: This project uses vendored headers, so this shouldn't happen. If it does:
1. Check that `pkg/ebpf/include/` exists
2. Verify `sockmap.c` includes are correct:
   ```c
   #include "include/linux/types.h"
   #include "include/linux/bpf.h"
   ```

### Issue: Gateway runs but eBPF not enabled

**Check logs**:
```bash
./uag 2>&1 | grep -i ebpf
```

**Common causes**:
- Kernel < 4.18
- Missing `CAP_BPF` capability
- Cgroup v2 not mounted

**Solution**: The gateway works fine without eBPF! It's just an optimization.

---

## Cross-Compilation

### Linux → Windows

```bash
GOOS=windows GOARCH=amd64 go build -o uag.exe ./cmd/gateway
```

**Note**: eBPF is Linux-only. Windows builds will auto-disable eBPF.

### macOS → Linux

```bash
GOOS=linux GOARCH=amd64 go build -o uag ./cmd/gateway
```

### ARM64 (for Raspberry Pi, AWS Graviton)

```bash
GOOS=linux GOARCH=arm64 go build -o uag ./cmd/gateway
```

---

## CI/CD Integration

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
      
      - name: Install Clang
        run: sudo apt-get install -y clang llvm
      
      - name: Install bpf2go
        run: go install github.com/cilium/ebpf/cmd/bpf2go@latest
      
      - name: Generate eBPF
        run: make generate-ebpf
      
      - name: Build
        run: make build
      
      - name: Test
        run: make test
```

### GitLab CI Example

```yaml
build:
  image: golang:1.21
  before_script:
    - apt-get update && apt-get install -y clang llvm
    - go install github.com/cilium/ebpf/cmd/bpf2go@latest
  script:
    - make generate-ebpf
    - make build
    - make test
```

---

## Performance Tuning

### Build Optimizations

```bash
# Smaller binary (strip debug symbols)
go build -ldflags="-s -w" -o uag ./cmd/gateway

# Static linking (for Alpine/scratch containers)
CGO_ENABLED=0 go build -o uag ./cmd/gateway

# Aggressive optimization
go build -ldflags="-s -w" -gcflags="-l=4" -o uag ./cmd/gateway
```

### eBPF Optimizations

```bash
# Enable eBPF verifier logs (for debugging)
clang -O2 -g -target bpf -c sockmap.c -o sockmap.o

# Check eBPF instructions count
llvm-objdump -d sockmap.o | wc -l
```

---

## Next Steps

- [Configuration Guide](../config/config.yaml)
- [eBPF Deep Dive](EBPF.md)
- [Deployment Guide](../deploy/README.md)
- [Performance Benchmarks](BENCHMARKS.md)

