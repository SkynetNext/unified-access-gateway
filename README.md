# Unified Access Gateway (UAG)

[![Go Report Card](https://goreportcard.com/badge/github.com/SkynetNext/unified-access-gateway)](https://goreportcard.com/report/github.com/SkynetNext/unified-access-gateway)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**Unified Access Gateway (UAG)** is a high-performance, cloud-native gateway designed for mixed-protocol environments (HTTP/TCP/WebSocket). It leverages **Go** for concurrency and **eBPF** for kernel-level acceleration.

Built for **SREs** who need visibility, elasticity, and unified governance.

---

## 🚀 Key Features

### 1. Intelligent Protocol Sniffing
*   **Single Port, Multi-Protocol**: Automatically identifies HTTP, TCP, or TLS traffic on the same port using magic byte sniffing.
*   **Dynamic Routing**: Routes traffic based on protocol type, host, or custom binary headers.

### 2. eBPF Kernel Acceleration
*   **SockMap Redirection**: Uses eBPF `BPF_MAP_TYPE_SOCKMAP` to redirect packets between sockets in the kernel, bypassing the TCP/IP stack for maximum throughput.
*   **Zero-Copy Forwarding**: Reduces context switching overhead.

### 3. Kubernetes Native & Elasticity
*   **Graceful Drain Mode**: Supports connection draining for long-lived TCP connections during rolling updates.
*   **HPA Ready**: Exposes custom metrics (QPS, active connections) for Kubernetes Horizontal Pod Autoscaler.

### 4. Full Observability
*   **Prometheus Metrics**: Standard golden signals (Latency, Traffic, Errors).
*   **Async Logging**: High-throughput access logs pushed to **Kafka** asynchronously.

---

## 🛠 Architecture

graph TB
    Client -->|TCP/HTTP| UAG[Unified Access Gateway]
    
    subgraph "Gateway Core"
        Listener[Multi-Protocol Listener] --> Sniffer[Protocol Sniffer]
        Sniffer -->|HTTP| HttpProxy[Reverse Proxy]
        Sniffer -->|TCP| TcpProxy[TCP Stream Proxy]
        
        TcpProxy -.->|Acceleration| eBPF[eBPF SockMap]
    end
    
    HttpProxy --> Backend1[Web Service]
    TcpProxy --> Backend2[Game Server]## 📦 Quick Start

### Prerequisites
*   Go 1.21+
*   Docker & Kubernetes (Optional)

### Build
go build -o uag ./cmd/gateway### Run
# Start gateway on :8080
./uag### Deploy to K8s
kubectl apply -f deploy/---

## 📊 Benchmarks

*(Placeholder for wrk/k6 benchmark results)*

---

## 📝 License

MIT © 2025 SkynetNext