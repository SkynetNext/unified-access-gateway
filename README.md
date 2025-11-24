# Unified Access Gateway (UAG)

[![Go Version](https://img.shields.io/badge/Go-1.21+-blue.svg)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

A **production-grade, cloud-native unified access gateway** designed for high-performance microservices architectures. Built with Go, it provides a single entry point for multiple protocols (HTTP/TCP/WebSocket) with advanced features like graceful shutdown, eBPF acceleration readiness, and comprehensive observability.

---

## 🚀 Features

### Core Capabilities
- **Multi-Protocol Support**: HTTP/1.1, HTTP/2, TCP, WebSocket on a single port
- **Protocol Sniffing**: Automatic protocol detection via magic byte inspection
- **Reverse Proxy**: High-performance forwarding to backend services
- **Graceful Shutdown**: Zero-downtime deployments with drain mode for long-lived connections

### Cloud Native
- **Kubernetes Native**: HPA support, readiness/liveness probes
- **Observability**: Prometheus metrics, structured logging (Kafka-ready)
- **Configuration**: Environment variable overrides for 12-factor apps
- **GitOps Ready**: Declarative K8s manifests included

### Performance
- **eBPF Ready**: Architecture designed for SockMap acceleration (Cilium integration)
- **Connection Pooling**: Efficient backend connection management
- **Zero-Copy**: Optimized TCP proxying with `io.Copy`

---

## 📐 Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Client Traffic                         │
│                  (HTTP/TCP/WebSocket)                       │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
           ┌───────────────────────┐
           │  Alibaba Cloud LBS    │
           │   (Load Balancer)     │
           └───────────┬───────────┘
                       │
                       ▼
    ┌──────────────────────────────────────────┐
    │   Unified Access Gateway (K8s Pods)      │
    │  ┌────────────────────────────────────┐  │
    │  │  Protocol Sniffer (Magic Bytes)    │  │
    │  └──────────┬─────────────────────────┘  │
    │             │                             │
    │    ┌────────┴────────┐                    │
    │    │                 │                    │
    │    ▼                 ▼                    │
    │  ┌─────┐         ┌─────┐                 │
    │  │HTTP │         │ TCP │                 │
    │  │Proxy│         │Proxy│                 │
    │  └──┬──┘         └──┬──┘                 │
    │     │               │                     │
    │     │  Middleware:  │                     │
    │     │  - Metrics    │                     │
    │     │  - Logging    │                     │
    │     │  - RateLimit  │                     │
    └─────┼───────────────┼─────────────────────┘
          │               │
          ▼               ▼
    ┌─────────┐     ┌──────────┐
    │HttpProxy│     │GateServer│
    │ (C# API)│     │(C# Game) │
    └─────────┘     └──────────┘
```

---

## 🛠️ Quick Start

### Prerequisites
- Go 1.21+
- Docker (optional, for containerized deployment)
- Kubernetes cluster (for production deployment)

### Local Development

```bash
# 1. Clone the repository
git clone https://github.com/SkynetNext/unified-access-gateway.git
cd unified-access-gateway

# 2. Install dependencies
go mod download

# 3. Set environment variables (optional, defaults are provided)
export GATEWAY_LISTEN_ADDR=":8080"
export METRICS_LISTEN_ADDR=":9090"
export HTTP_BACKEND_URL="http://localhost:5000"
export TCP_BACKEND_ADDR="localhost:6000"

# 4. Build
go build -o uag ./cmd/gateway

# 5. Run
./uag
```

### Docker Build

```bash
docker build -t skynet/unified-access-gateway:latest .
docker run -p 8080:8080 -p 9090:9090 \
  -e HTTP_BACKEND_URL="http://backend:5000" \
  skynet/unified-access-gateway:latest
```

### Kubernetes Deployment

```bash
kubectl apply -f deploy/deployment.yaml
kubectl apply -f deploy/service.yaml
kubectl apply -f deploy/hpa.yaml
```

---

## ⚙️ Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `GATEWAY_LISTEN_ADDR` | `:8080` | Main gateway listen address |
| `METRICS_LISTEN_ADDR` | `:9090` | Prometheus metrics endpoint |
| `HTTP_BACKEND_URL` | `http://localhost:5000` | HTTP backend service URL |
| `TCP_BACKEND_ADDR` | `localhost:6000` | TCP backend service address |
| `SHUTDOWN_TIMEOUT` | `60s` | Max time for graceful shutdown |
| `DRAIN_WAIT_TIME` | `3600s` | Max time to drain long-lived connections |
| `METRICS_ENABLED` | `true` | Enable/disable metrics server |

### Configuration File

See `config/config.yaml` for YAML-based configuration (environment variables take precedence).

---

## 📊 Observability

### Prometheus Metrics

Available at `http://localhost:9090/metrics`:

- `gateway_requests_total{protocol, status}` - Total requests
- `gateway_request_duration_seconds{protocol}` - Request latency histogram
- `gateway_active_connections{protocol}` - Current active connections

### Health Checks

- **Liveness**: `GET /health` - Always returns 200 (process is alive)
- **Readiness**: `GET /ready` - Returns 503 when in drain mode (K8s stops routing traffic)

---

## 🎯 Production Deployment

### Graceful Shutdown for Long-Lived Connections

The gateway implements a **drain mode** to handle TCP/WebSocket connections gracefully:

1. **Signal Reception**: On `SIGTERM` (K8s pod termination), the gateway:
   - Marks itself as "draining" (readiness probe returns 503)
   - Stops accepting new connections
   - Keeps existing connections alive

2. **K8s Integration**:
   - `terminationGracePeriodSeconds: 3600` (1 hour for gaming sessions)
   - Readiness probe ensures no new traffic is routed to draining pods

3. **Connection Draining**:
   - Waits for clients to naturally disconnect (e.g., game session ends)
   - Configurable timeout via `DRAIN_WAIT_TIME`

### Horizontal Pod Autoscaling (HPA)

```yaml
# deploy/hpa.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: uag-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: unified-access-gateway
  minReplicas: 3
  maxReplicas: 20
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Pods
    pods:
      metric:
        name: gateway_requests_per_second
      target:
        type: AverageValue
        averageValue: "1000"
```

---

## 🔧 Advanced Features

### eBPF Acceleration (Roadmap)

The gateway is architected to support eBPF SockMap redirection for kernel-level network acceleration:

- **SockMap**: Direct socket-to-socket forwarding, bypassing TCP/IP stack
- **XDP**: Early packet filtering for DDoS protection
- **Cilium Integration**: Leverages existing cluster CNI

*Implementation pending - architecture supports future integration.*

### Protocol Inspection (Roadmap)

For custom binary protocols (e.g., `StructPacket`):

- Parse protocol headers (MsgID, UserID)
- Consistent hashing for stateful routing
- Connection affinity for game sessions

---

## 📈 Benchmarks

*Benchmarks pending - run `wrk` for HTTP and custom Go scripts for TCP load testing.*

Expected performance (based on similar Go proxies):
- **HTTP**: 50k+ req/s (single instance, 4 cores)
- **TCP**: 100k+ concurrent connections
- **Latency**: <1ms P99 (local network)

---

## 🤝 Contributing

Contributions are welcome! Please follow these guidelines:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

---

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

## 🙏 Acknowledgments

- Inspired by production gateways at Tencent, ByteDance, and OKX
- Built with best practices from SRE and cloud-native communities
- Special thanks to the Go, Kubernetes, and Prometheus ecosystems

---

## 📞 Contact

For questions or support, please open an issue on GitHub.

**Project Link**: [https://github.com/SkynetNext/unified-access-gateway](https://github.com/SkynetNext/unified-access-gateway)
