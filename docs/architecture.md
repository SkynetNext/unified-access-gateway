# Architecture

## Overview

Unified Access Gateway is a cloud-native, multi-protocol gateway designed for high-performance microservices architectures. It provides a single entry point for HTTP, TCP, and WebSocket traffic with automatic protocol detection.

## System Architecture

```mermaid
graph TB
    Client[Client Applications<br>HTTP/TCP/WebSocket] --> LBS[Cloud Load Balancer<br>Aliyun SLB / AWS ALB]
    
    subgraph "Kubernetes Cluster"
        direction TB
        
        subgraph "Unified Access Gateway (Deployment)"
            LBS --> Gateway[**Unified Access Gateway**<br>HPA: 3-20 Pods<br>Port: 8080]
            
            subgraph "Gateway Internal"
                direction TB
                Sniffer[Protocol Sniffer<br>Magic Byte Detection]
                HTTPHandler[HTTP Handler<br>Reverse Proxy]
                TCPHandler[TCP Handler<br>Stream Proxy]
                Middleware[Middleware Stack<br>Metrics/Tracing/Security/Logging]
                
                Gateway --> Sniffer
                Sniffer -->|HTTP/TLS| HTTPHandler
                Sniffer -->|TCP| TCPHandler
                HTTPHandler --> Middleware
                TCPHandler --> Middleware
            end
        end
        
        subgraph "Backend Services"
            direction TB
            HTTPBackend[HTTP Backend<br>HttpProxy Service]
            TCPBackend[TCP Backend<br>GateServer Service]
            
            Middleware -->|HTTP/WebSocket| HTTPBackend
            Middleware -->|TCP Stream| TCPBackend
        end
        
        subgraph "Infrastructure Services"
            direction TB
            Redis[(Redis<br>Config & Session)]
            Prometheus[Prometheus<br>Metrics Collection]
            Jaeger[Jaeger<br>Distributed Tracing]
            
            Gateway -->|Read Config| Redis
            Gateway -->|Metrics| Prometheus
            Gateway -->|Traces| Jaeger
        end
    end
    
    classDef gateway fill:#00ADD8,stroke:#333,stroke-width:3px,color:#fff;
    classDef backend fill:#4CAF50,stroke:#333,stroke-width:2px,color:#fff;
    classDef infra fill:#FF9800,stroke:#333,stroke-width:2px,color:#fff;
    classDef client fill:#9C27B0,stroke:#333,stroke-width:2px,color:#fff;
    
    class Gateway,Sniffer,HTTPHandler,TCPHandler,Middleware gateway;
    class HTTPBackend,TCPBackend backend;
    class Redis,Prometheus,Jaeger infra;
    class Client,LBS client;
```

## Core Components

### 1. Protocol Sniffer

Automatically detects protocol type by inspecting the first few bytes of incoming connections:

- **HTTP**: Detects `GET`, `POST`, `PUT`, `DELETE`, etc.
- **TLS/HTTPS**: Detects TLS handshake (`0x16`)
- **TCP**: Default fallback for unrecognized protocols

### 2. HTTP Handler

- Reverse proxy using `httputil.ReverseProxy`
- Supports HTTP/1.1 and HTTP/2
- WebSocket upgrade support
- Request/response metrics collection

### 3. TCP Handler

- Stream proxy using `io.Copy`
- eBPF SockMap acceleration (optional)
- Connection metrics
- Graceful connection draining

### 4. Middleware Stack

- **Metrics**: Prometheus metrics collection
- **Tracing**: OpenTelemetry distributed tracing
- **Security**: Rate limiting, WAF, authentication
- **Logging**: Structured access logs

## Cloud-Native Features

### 1. Configuration Management

- **Business Config**: Loaded from Redis (read-only)
- **Infrastructure Config**: Environment variables / ConfigMap
- **Hot Reload**: Redis pub/sub for dynamic updates

### 2. Service Discovery

- Kubernetes DNS resolution
- Automatic backend discovery
- Health-aware routing

### 3. Health Probes

- **Liveness**: `/health` - Process health check
- **Readiness**: `/ready` - Service readiness (includes Redis health)

### 4. Graceful Shutdown

1. Receive `SIGTERM` from Kubernetes
2. Mark as draining (`/ready` returns 503)
3. Stop accepting new connections
4. Wait for active connections to drain
5. Shutdown metrics server
6. Exit

### 5. Observability

- **Metrics**: Prometheus-compatible `/metrics` endpoint
- **Tracing**: OpenTelemetry with Jaeger export
- **Logging**: Structured JSON logs to stdout

### 6. Auto-Scaling

- HPA support via CPU and custom metrics
- Readiness probe ensures proper traffic routing
- Horizontal scaling for stateless HTTP traffic

## eBPF Acceleration

### Architecture

```mermaid
graph TB
    subgraph "User Space (Go Gateway)"
        App[Gateway Application]
        SockMapMgr[SockMap Manager]
        XDPMgr[XDP Manager]
    end
    
    subgraph "Kernel Space (eBPF Programs)"
        SockOps[SockOps Program<br>Socket Lifecycle]
        SockMap[(SockMap<br>Socket Redirection)]
        XDPProg[XDP Program<br>Packet Filtering]
        XDPMap[(XDP Maps<br>IP Blacklist/Rate Limit)]
    end
    
    subgraph "Network Stack"
        Driver[Network Driver]
        TCPIP[TCP/IP Stack]
    end
    
    App --> SockMapMgr
    App --> XDPMgr
    SockMapMgr -->|Load & Attach| SockOps
    SockOps --> SockMap
    XDPMgr -->|Load & Attach| XDPProg
    XDPProg --> XDPMap
    
    Driver -->|Early Filter| XDPProg
    XDPProg -->|XDP_PASS| TCPIP
    TCPIP -->|Socket Events| SockOps
    SockOps -->|Redirect| SockMap
    
    classDef userspace fill:#00ADD8,stroke:#333,stroke-width:2px,color:#fff;
    classDef kernel fill:#FF5722,stroke:#333,stroke-width:2px,color:#fff;
    classDef network fill:#4CAF50,stroke:#333,stroke-width:2px,color:#fff;
    
    class App,SockMapMgr,XDPMgr userspace;
    class SockOps,SockMap,XDPProg,XDPMap kernel;
    class Driver,TCPIP network;
```

### SockMap

Kernel-level socket redirection for TCP connections:

- Zero-copy packet forwarding
- Bypasses TCP/IP stack
- 30-50% latency reduction
- Automatic fallback to userspace

### XDP

Early packet filtering at driver layer:

- DDoS protection
- IP blacklisting
- Rate limiting
- SYN flood mitigation

See [Development](development.md) for eBPF compilation details.

## Data Flow

### HTTP Request Flow

```mermaid
sequenceDiagram
    participant C as Client
    participant LBS as Load Balancer
    participant G as Gateway
    participant S as Protocol Sniffer
    participant H as HTTP Handler
    participant M as Middleware
    participant B as Backend Service
    
    C->>LBS: HTTPS Request
    LBS->>G: Forward to Gateway Pod
    G->>S: Inspect first bytes
    S->>H: Detect HTTP/TLS
    H->>M: Apply middleware
    M->>B: Reverse proxy
    B->>M: Response
    M->>H: Add tracing/metrics
    H->>G: Return response
    G->>LBS: Forward response
    LBS->>C: Return to client
```

### TCP Connection Flow

```mermaid
sequenceDiagram
    participant C as Client
    participant LBS as Load Balancer
    participant G as Gateway
    participant S as Protocol Sniffer
    participant T as TCP Handler
    participant E as eBPF SockMap
    participant B as Backend Service
    
    C->>LBS: TCP Connection
    LBS->>G: Forward connection
    G->>S: Inspect first bytes
    S->>T: Detect TCP/Unknown
    T->>E: Register socket pair (if enabled)
    E->>B: Kernel-level redirect
    B->>E: Response packets
    E->>T: Zero-copy forwarding
    T->>G: Stream proxy
    G->>LBS: Forward data
    LBS->>C: Return to client
```

## Kubernetes Deployment Architecture

```mermaid
graph TB
    subgraph "External"
        Internet[Internet Traffic]
        LBS[Cloud Load Balancer<br>Aliyun SLB / AWS ALB]
    end
    
    subgraph "Kubernetes Cluster (uag namespace)"
        direction TB
        
        subgraph "Gateway Deployment"
            HPA[HorizontalPodAutoscaler<br>CPU: 70%<br>QPS: 1000 req/s]
            Service[Service<br>ClusterIP<br>Port: 8080]
            Gateway1[Gateway Pod 1<br>Metrics: :9090]
            Gateway2[Gateway Pod 2<br>Metrics: :9090]
            Gateway3[Gateway Pod N<br>Metrics: :9090]
            
            HPA --> Service
            Service --> Gateway1
            Service --> Gateway2
            Service --> Gateway3
        end
        
        subgraph "Configuration & State"
            Redis[(Redis<br>Business Config<br>Security Config)]
            ConfigMap[ConfigMap<br>Infrastructure Config]
            Secret[Secret<br>Redis Password]
        end
        
        subgraph "Observability"
            Prometheus[Prometheus<br>Scrapes /metrics]
            Jaeger[Jaeger<br>Receives traces]
            Grafana[Grafana<br>Dashboards]
        end
        
        Gateway1 --> Redis
        Gateway2 --> Redis
        Gateway3 --> Redis
        Gateway1 --> ConfigMap
        Gateway2 --> ConfigMap
        Gateway3 --> ConfigMap
        Gateway1 --> Secret
        Gateway2 --> Secret
        Gateway3 --> Secret
        
        Gateway1 -->|Metrics| Prometheus
        Gateway2 -->|Metrics| Prometheus
        Gateway3 -->|Metrics| Prometheus
        Gateway1 -->|Traces| Jaeger
        Gateway2 -->|Traces| Jaeger
        Gateway3 -->|Traces| Jaeger
        Prometheus --> Grafana
        Jaeger --> Grafana
    end
    
    subgraph "Backend Services"
        HTTPBackend[HttpProxy Service<br>Deployment]
        TCPBackend[GateServer Service<br>StatefulSet]
    end
    
    Internet --> LBS
    LBS --> Service
    Service --> HTTPBackend
    Service --> TCPBackend
    
    classDef gateway fill:#00ADD8,stroke:#333,stroke-width:3px,color:#fff;
    classDef k8s fill:#326ce5,stroke:#fff,stroke-width:2px,color:#fff;
    classDef obs fill:#FF9800,stroke:#333,stroke-width:2px,color:#fff;
    classDef backend fill:#4CAF50,stroke:#333,stroke-width:2px,color:#fff;
    
    class Gateway1,Gateway2,Gateway3 gateway;
    class HPA,Service,ConfigMap,Secret,Redis k8s;
    class Prometheus,Jaeger,Grafana obs;
    class HTTPBackend,TCPBackend backend;
```

## Configuration Sources

| Type | Source | Priority | Fallback |
|------|--------|----------|----------|
| Business | Redis | Required | None (gateway exits) |
| Security | Redis | Required | Defaults |
| Infrastructure | Env Vars | Optional | Defaults |

## Performance Characteristics

- **Stateless**: HTTP requests can be load-balanced across pods
- **Stateful**: TCP connections require graceful drain during shutdown
- **Horizontal Scaling**: Suitable for HTTP traffic via HPA
- **Vertical Scaling**: TCP connections benefit from more resources per pod

## Security Model

- **Authentication**: TLS client certificate verification
- **Rate Limiting**: Token bucket per instance
- **WAF**: IP blacklist and pattern matching
- **Audit Logging**: All allow/deny decisions logged

See [Configuration](configuration.md) for security settings.

