# Unified Access Gateway

[![Go Version](https://img.shields.io/badge/Go-1.21+-blue.svg)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

A production-grade, cloud-native unified access gateway for high-performance microservices. Provides a single entry point for HTTP, TCP, and WebSocket traffic with eBPF acceleration, graceful shutdown, and comprehensive observability.

## Features

- **Multi-Protocol**: HTTP/1.1, HTTP/2, TCP, WebSocket on a single port
- **Protocol Sniffing**: Automatic detection via magic byte inspection
- **Cloud Native**: Kubernetes-native with HPA, health probes, service discovery
- **eBPF Acceleration**: Kernel-level SockMap and XDP for zero-copy proxying and DDoS protection
- **Observability**: Prometheus metrics, OpenTelemetry tracing, structured logging
- **Graceful Shutdown**: Zero-downtime deployments with drain mode for long-lived connections

## Quick Start

```bash
# Clone
git clone https://github.com/SkynetNext/unified-access-gateway.git
cd unified-access-gateway

# Build
go build -o uag ./cmd/gateway

# Run
./uag
```

See [Getting Started](docs/getting-started.md) for detailed instructions.

## Documentation

- [Getting Started](docs/getting-started.md) - Installation and quick start
- [Architecture](docs/architecture.md) - System design and cloud-native features
- [Configuration](docs/configuration.md) - Configuration reference
- [Deployment](docs/deployment.md) - Kubernetes and Docker deployment
- [Development](docs/development.md) - Building from source and eBPF compilation
- [Troubleshooting](docs/troubleshooting.md) - Common issues and solutions

## Architecture

```
Client → Load Balancer → Gateway (Protocol Sniffer)
                              ├─ HTTP Handler → HTTP Backend
                              └─ TCP Handler → TCP Backend
```

The gateway automatically detects protocol type and routes traffic accordingly. Supports graceful shutdown, eBPF acceleration, and full observability.

## Performance

- **HTTP**: 50k+ req/s (single instance, 4 cores)
- **TCP**: 100k+ concurrent connections
- **Latency**: <1ms P99 (local network)
- **eBPF SockMap**: 30-50% latency reduction
- **XDP**: 20-30 Mpps packet processing

## Requirements

- Go 1.21+ (for building)
- Linux Kernel 4.18+ (for eBPF, optional)
- Kubernetes 1.20+ (for production deployment)

## License

MIT License - see [LICENSE](LICENSE) for details.

## Contributing

Contributions welcome! Please see our [Development Guide](docs/development.md) for details.
