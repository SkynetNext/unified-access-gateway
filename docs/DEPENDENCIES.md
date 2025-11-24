# Dependency Management

## Philosophy

This project follows **zero external system dependencies** for eBPF development:

✅ **No `apt-get install` required** (except Clang)  
✅ **No kernel headers needed**  
✅ **No libbpf-dev package**  
✅ **Works on any Linux distro**  

## How We Achieve This

### 1. Vendored BPF Headers

All BPF headers are **vendored** in `pkg/ebpf/include/`:

```
pkg/ebpf/include/
├── bpf/
│   ├── bpf_helpers.h    # BPF helper functions (from libbpf)
│   └── bpf_endian.h     # Endianness macros
└── linux/
    ├── bpf.h            # BPF UAPI definitions (from kernel)
    └── types.h          # Kernel types (__u32, __u64, etc.)
```

**Source**: Extracted from [libbpf](https://github.com/libbpf/libbpf) and Linux kernel headers.

**License**: 
- `bpf/*.h`: LGPL-2.1 OR BSD-2-Clause
- `linux/*.h`: GPL-2.0 WITH Linux-syscall-note

### 2. Platform-Specific Build Tags

```go
// +build linux
// sockmap.go - Real eBPF implementation (Linux only)

// +build !linux
// stub.go - No-op implementation (Windows/macOS)
```

This allows the gateway to **compile and run on any platform**, with eBPF as an optional optimization on Linux.

## Build Requirements

### Minimal (No eBPF)

```bash
# Only Go is required
go build ./cmd/gateway
```

Gateway runs with userspace proxying (no eBPF).

### Full (With eBPF)

```bash
# Install Clang (only system dependency)
apt-get install clang llvm

# Install bpf2go (Go tool)
go install github.com/cilium/ebpf/cmd/bpf2go@latest

# Generate eBPF bindings
make generate-ebpf

# Build
make build
```

## Go Dependencies

Managed by `go.mod`:

```go
require (
	github.com/cilium/ebpf v0.12.3           // eBPF loader
	github.com/prometheus/client_golang v1.17.0  // Metrics
)
```

**Download**:
```bash
go mod download
```

**Update**:
```bash
go get -u github.com/cilium/ebpf@latest
go mod tidy
```

## C/C++ Dependencies (eBPF)

### Traditional Approach (❌ We Don't Do This)

```bash
# Traditional eBPF projects require:
apt-get install libbpf-dev linux-headers-$(uname -r)
```

**Problems**:
- Different package names per distro (libbpf-dev vs libbpf-devel)
- Kernel headers version mismatch
- Requires root access
- CI/CD complexity

### Our Approach (✅ Vendored Headers)

```c
// sockmap.c
#include "include/linux/types.h"      // Vendored
#include "include/linux/bpf.h"        // Vendored
#include "include/bpf/bpf_helpers.h"  // Vendored
```

**Benefits**:
- Zero system dependencies (only Clang)
- Consistent across environments
- No root access needed
- Simple CI/CD

## Updating Vendored Headers

If you need to update headers (e.g., for new kernel features):

### 1. Download Latest libbpf

```bash
git clone https://github.com/libbpf/libbpf.git /tmp/libbpf
cd /tmp/libbpf
git checkout v1.3.0  # Use stable release
```

### 2. Extract Required Headers

```bash
# Copy BPF helpers
cp /tmp/libbpf/src/bpf_helpers.h pkg/ebpf/include/bpf/
cp /tmp/libbpf/src/bpf_endian.h pkg/ebpf/include/bpf/

# Verify no external dependencies
grep -r '#include' pkg/ebpf/include/bpf/
```

### 3. Extract Kernel UAPI Headers

```bash
# Download kernel source
wget https://cdn.kernel.org/pub/linux/kernel/v6.x/linux-6.6.tar.xz
tar -xf linux-6.6.tar.xz

# Copy UAPI headers (user-space API, stable across versions)
cp linux-6.6/include/uapi/linux/bpf.h pkg/ebpf/include/linux/
cp linux-6.6/include/uapi/linux/types.h pkg/ebpf/include/linux/
```

### 4. Minimize Headers

**Important**: Only include what's needed! Remove unused definitions to keep headers small.

Example: Our `linux/types.h` only includes:
```c
typedef unsigned int __u32;
typedef unsigned long long __u64;
// ... (8 lines total)
```

vs. kernel's `linux/types.h`:
```c
// ... (500+ lines)
```

### 5. Test Compilation

```bash
cd pkg/ebpf
clang -O2 -g -Wall -Werror -target bpf -c sockmap.c -o sockmap.o
```

If successful, commit the updated headers.

## Dependency Versions

### Go Modules

```bash
# List all dependencies
go list -m all

# Check for updates
go list -u -m all

# Update specific module
go get github.com/cilium/ebpf@v0.13.0
go mod tidy
```

### Clang Version

```bash
# Check version
clang --version

# Minimum: Clang 10+
# Recommended: Clang 14+ (better BPF support)
```

### Kernel Requirements

| Feature | Min Kernel | Our Usage |
|---------|-----------|-----------|
| BPF_MAP_TYPE_SOCKHASH | 4.18 | ✅ Used |
| BPF_PROG_TYPE_SK_SKB | 4.14 | ✅ Used |
| BPF_PROG_TYPE_SOCK_OPS | 4.13 | ✅ Used |
| SO_COOKIE socket option | 4.6 | ✅ Used |

**Graceful Fallback**: If kernel < 4.18, eBPF auto-disables, gateway uses userspace proxy.

## CI/CD Dependencies

### GitHub Actions

```yaml
- name: Install Clang
  run: sudo apt-get install -y clang llvm

- name: Install bpf2go
  run: go install github.com/cilium/ebpf/cmd/bpf2go@latest

- name: Generate eBPF
  run: make generate-ebpf
```

### Docker Build

```dockerfile
# Stage 1: eBPF builder
FROM ubuntu:22.04 AS ebpf-builder
RUN apt-get update && apt-get install -y clang llvm
COPY pkg/ebpf/sockmap.c .
RUN clang -O2 -g -target bpf -c sockmap.c -o sockmap.o

# Stage 2: Go builder
FROM golang:1.21-alpine AS go-builder
COPY --from=ebpf-builder /build/sockmap.o ./pkg/ebpf/
RUN go build ./cmd/gateway
```

## Troubleshooting

### Issue: `clang: command not found`

**Solution**:
```bash
# Ubuntu/Debian
sudo apt-get install clang

# RHEL/CentOS
sudo yum install clang

# macOS
brew install llvm
```

### Issue: `bpf2go: command not found`

**Solution**:
```bash
go install github.com/cilium/ebpf/cmd/bpf2go@latest

# Add to PATH
export PATH=$PATH:$(go env GOPATH)/bin
```

### Issue: `unknown type name '__u32'`

**Cause**: Missing vendored header

**Solution**: Verify `pkg/ebpf/include/linux/types.h` exists and is included in `sockmap.c`.

### Issue: `implicit declaration of function 'bpf_map_lookup_elem'`

**Cause**: Missing BPF helpers

**Solution**: Verify `pkg/ebpf/include/bpf/bpf_helpers.h` exists and is included in `sockmap.c`.

## Comparison with Other Projects

| Project | Dependency Management | Our Approach |
|---------|----------------------|--------------|
| **Cilium** | Requires libbpf-dev, kernel headers | ✅ Vendored headers |
| **Katran (Facebook)** | Requires libbpf-dev | ✅ Vendored headers |
| **Envoy** | No eBPF (uses userspace) | ✅ eBPF + fallback |
| **Nginx** | No eBPF | ✅ eBPF + fallback |

## Best Practices

### 1. Pin Dependency Versions

```go
// go.mod
require github.com/cilium/ebpf v0.12.3  // Pin to specific version
```

### 2. Vendor Go Modules (Optional)

```bash
go mod vendor
```

This creates a `vendor/` directory with all Go dependencies.

### 3. Document Header Sources

```c
// pkg/ebpf/include/bpf/bpf_helpers.h
/*
 * Source: https://github.com/libbpf/libbpf/blob/v1.3.0/src/bpf_helpers.h
 * License: LGPL-2.1 OR BSD-2-Clause
 * Modified: Minimal subset for sockmap programs
 */
```

### 4. Test on Multiple Platforms

```bash
# Linux
GOOS=linux go build ./cmd/gateway

# Windows (eBPF disabled)
GOOS=windows go build ./cmd/gateway

# macOS (eBPF disabled)
GOOS=darwin go build ./cmd/gateway
```

## Security Considerations

### Vendored Headers

**Risk**: Outdated headers may have security vulnerabilities.

**Mitigation**:
- Review headers before vendoring
- Update from trusted sources (kernel.org, libbpf GitHub)
- Only include UAPI headers (stable, user-space API)

### Go Dependencies

**Risk**: Vulnerable dependencies (e.g., Prometheus CVE)

**Mitigation**:
```bash
# Check for known vulnerabilities
go list -json -m all | nancy sleuth

# Or use govulncheck
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
```

## License Compliance

### Our Code

- Gateway code: MIT License
- eBPF program (`sockmap.c`): GPL-2.0 (required for kernel code)

### Vendored Headers

- `bpf_helpers.h`, `bpf_endian.h`: LGPL-2.1 OR BSD-2-Clause
- `linux/bpf.h`, `linux/types.h`: GPL-2.0 WITH Linux-syscall-note

**Note**: GPL-2.0 WITH Linux-syscall-note allows use in userspace without GPL contamination.

## Summary

| Aspect | Traditional | Our Approach |
|--------|------------|--------------|
| **System Deps** | libbpf-dev, kernel-headers | ✅ Clang only |
| **Portability** | Linux-specific | ✅ Cross-platform |
| **CI/CD** | Complex setup | ✅ Simple |
| **Root Access** | Required for apt-get | ✅ Not required |
| **Distro Support** | Ubuntu/RHEL specific | ✅ Any distro |

**Result**: A truly portable, zero-dependency eBPF gateway! 🚀

