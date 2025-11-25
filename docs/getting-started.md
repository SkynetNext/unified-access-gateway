# Getting Started

This guide will help you get Unified Access Gateway up and running.

## Prerequisites

- Go 1.21+ (for building from source)
- Docker (optional, for containerized deployment)
- Kubernetes cluster (for production deployment)

## Installation

### From Source

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

### Docker

```bash
docker build -t unified-access-gateway:latest .
docker run -p 8080:8080 -p 9090:9090 unified-access-gateway:latest
```

### Kubernetes

```bash
kubectl apply -f deploy/namespace.yaml
kubectl apply -f deploy/deployment.yaml
kubectl apply -f deploy/service.yaml
```

## Basic Configuration

The gateway uses environment variables for configuration. See [Configuration](configuration.md) for complete reference.

### Required Settings

```bash
# Business configuration (from Redis)
REDIS_ADDR=10.1.0.8:6379
REDIS_PASSWORD=your-password

# Infrastructure configuration (from env vars)
METRICS_LISTEN_ADDR=:9090
```

### Optional Settings

```bash
# Metrics
METRICS_ENABLED=true

# Tracing
JAEGER_ENDPOINT=http://jaeger:14268/api/traces

# Logging
LOG_LEVEL=info
```

## Verify Installation

### Health Check

```bash
# Liveness probe
curl http://localhost:9090/health
# Expected: 200 OK

# Readiness probe
curl http://localhost:9090/ready
# Expected: 200 OK (or 503 if Redis unavailable)
```

### Metrics

```bash
curl http://localhost:9090/metrics
# Expected: Prometheus metrics output
```

## Next Steps

- [Architecture](architecture.md) - Understand system design
- [Configuration](configuration.md) - Configure the gateway
- [Deployment](deployment.md) - Deploy to production
- [Development](development.md) - Build with eBPF support

