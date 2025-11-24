# Multi-stage build for Unified Access Gateway with eBPF support

# Stage 1: Build eBPF programs
FROM ubuntu:22.04 AS ebpf-builder

RUN apt-get update && apt-get install -y \
    clang \
    llvm \
    libbpf-dev \
    linux-headers-generic \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build
COPY pkg/ebpf/sockmap.c .

# Compile eBPF program
RUN clang -O2 -g -Wall -Werror -target bpf -D__TARGET_ARCH_x86_64 \
    -c sockmap.c -o sockmap.o

# Stage 2: Build Go application
FROM golang:1.21-alpine AS go-builder

# Install build dependencies
RUN apk add --no-cache git make

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Copy compiled eBPF object from previous stage
COPY --from=ebpf-builder /build/sockmap.o ./pkg/ebpf/

# Build Go binary (static linking for Alpine)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o uag \
    ./cmd/gateway

# Stage 3: Final runtime image
FROM alpine:3.18

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    libbpf \
    && rm -rf /var/cache/apk/*

WORKDIR /app

# Copy binary from builder
COPY --from=go-builder /build/uag .

# Copy configuration
COPY config/config.yaml ./config/

# Create non-root user
RUN addgroup -g 1000 gateway && \
    adduser -D -u 1000 -G gateway gateway && \
    chown -R gateway:gateway /app

USER gateway

# Expose ports
EXPOSE 8080 9090

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:9090/health || exit 1

ENTRYPOINT ["./uag"]

