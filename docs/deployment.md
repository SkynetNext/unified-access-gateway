# Deployment

## Kubernetes

### Prerequisites

- Kubernetes 1.20+
- Redis instance (for business configuration)
- Prometheus (optional, for metrics)

### Deploy

```bash
# Create namespace
kubectl apply -f deploy/namespace.yaml

# Deploy gateway
kubectl apply -f deploy/deployment.yaml

# Create service
kubectl apply -f deploy/service.yaml

# Optional: Deploy HPA
kubectl apply -f deploy/hpa.yaml
```

### Verify

```bash
# Check pods
kubectl get pods -n uag

# Check logs
kubectl logs -f deployment/uag -n uag

# Check service
kubectl get svc -n uag

# Port forward for testing
kubectl port-forward svc/uag 8080:8080 -n uag
```

### Initialize Redis Configuration

```bash
# Create Redis init job
kubectl apply -f deploy/redis-init-job.yaml

# Or manually
kubectl run -it --rm redis-init --image=redis:7-alpine -n uag -- \
  redis-cli -h 10.1.0.8 -a $REDIS_PASSWORD \
  HSET gateway:business:config \
  server.listen_addr ":8080" \
  backends.http.target_url "http://httpproxy:5000" \
  backends.tcp.target_addr "gateserver:6000"
```

## Docker

### Build

```bash
docker build -t unified-access-gateway:latest .
```

### Run

```bash
docker run -d \
  --name uag \
  -p 8080:8080 \
  -p 9090:9090 \
  -e REDIS_ADDR=10.1.0.8:6379 \
  -e REDIS_PASSWORD=your-password \
  -e METRICS_LISTEN_ADDR=:9090 \
  unified-access-gateway:latest
```

### With eBPF Support

```bash
docker run -d \
  --name uag \
  --privileged \
  -p 8080:8080 \
  -p 9090:9090 \
  -v /sys/fs/cgroup:/sys/fs/cgroup:ro \
  -e REDIS_ADDR=10.1.0.8:6379 \
  unified-access-gateway:latest
```

**Note**: `--privileged` is only needed for eBPF. Without it, the gateway falls back to userspace.

## Production Considerations

### Resource Limits

```yaml
resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 2000m
    memory: 2Gi
```

### Graceful Shutdown

```yaml
spec:
  terminationGracePeriodSeconds: 3600  # 1 hour for long connections
```

### Health Probes

```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 9090
  initialDelaySeconds: 15
  periodSeconds: 10

readinessProbe:
  httpGet:
    path: /ready
    port: 9090
  initialDelaySeconds: 5
  periodSeconds: 5
```

### Horizontal Pod Autoscaling

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: uag-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: uag
  minReplicas: 3
  maxReplicas: 20
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        averageUtilization: 70
```

## Monitoring

### Prometheus

Gateway exposes metrics at `/metrics` endpoint. Configure Prometheus to scrape:

```yaml
scrape_configs:
- job_name: 'uag'
  kubernetes_sd_configs:
  - role: pod
    namespaces:
      names:
      - uag
  relabel_configs:
  - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
    action: keep
    regex: true
```

### Grafana Dashboard

Import dashboard from `deploy/grafana-dashboard.json` or create custom dashboard using metrics:

- `gateway_requests_total`
- `gateway_request_duration_seconds`
- `gateway_active_connections`

## Security

### Network Policies

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: uag-netpol
spec:
  podSelector:
    matchLabels:
      app: unified-access-gateway
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector: {}
    ports:
    - protocol: TCP
      port: 8080
  egress:
  - to:
    - namespaceSelector: {}
    ports:
    - protocol: TCP
      port: 6379  # Redis
```

### Secrets Management

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: uag-secrets
type: Opaque
stringData:
  redis-password: your-password
```

## Troubleshooting

### Pods not starting

```bash
# Check events
kubectl describe pod <pod-name> -n uag

# Check logs
kubectl logs <pod-name> -n uag
```

### Image pull errors

```bash
# Ensure imagePullSecrets is configured
kubectl get deployment uag -n uag -o yaml | grep imagePullSecrets
```

### Redis connection issues

```bash
# Test Redis connectivity
kubectl run -it --rm redis-test --image=redis:7-alpine -n uag -- \
  redis-cli -h 10.1.0.8 -a $REDIS_PASSWORD PING
```

