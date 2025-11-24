# 云原生特性详解

## 什么是"云原生"？

**云原生 (Cloud Native)** 不仅仅是"能在 K8s 上运行"，而是：

1. **12-Factor App 原则**：配置外部化、无状态、日志即流
2. **Kubernetes 原生**：健康检查、服务发现、自动扩缩容
3. **可观测性**：Metrics、Logging、Tracing
4. **弹性设计**：优雅降级、自动恢复、故障隔离

## 本项目的云原生特性

### 1. ✅ 配置外部化 (12-Factor)

```go
// internal/config/config.go
func LoadConfig() *Config {
    return &Config{
        Server: ServerConfig{
            ListenAddr: getEnv("GATEWAY_LISTEN_ADDR", ":8080"),  // 环境变量
        },
    }
}
```

**K8s 集成**：
```yaml
# deploy/deployment.yaml
env:
- name: GATEWAY_LISTEN_ADDR
  value: ":8080"
- name: HTTP_BACKEND_URL
  valueFrom:
    configMapKeyRef:
      name: gateway-config
      key: http_backend_url
```

### 2. ✅ Kubernetes 服务发现

```go
// internal/discovery/k8s.go
func (k *K8sServiceDiscovery) ResolveService(serviceName string) (string, error) {
    // 使用 K8s DNS: <service>.<namespace>.svc.cluster.local
    fqdn := fmt.Sprintf("%s.%s.svc.cluster.local", serviceName, k.namespace)
    ips, err := net.LookupIP(fqdn)
    return ips[0].String(), nil
}
```

**使用示例**：
```go
// 自动解析 K8s Service
discovery := discovery.NewK8sServiceDiscovery()
backendAddr, _ := discovery.ResolveServiceWithPort("httpproxy-service", 5000)
// 结果: 10.244.1.5:5000 (Pod IP)
```

### 3. ✅ 健康检查 (Liveness/Readiness)

```go
// internal/core/server.go
func (s *Server) readyHandler(w http.ResponseWriter, r *http.Request) {
    if atomic.LoadInt32(&s.draining) == 1 {
        w.WriteHeader(http.StatusServiceUnavailable)  // 503 = 不接收流量
        return
    }
    w.WriteHeader(http.StatusOK)  // 200 = 就绪
}
```

**K8s 配置**：
```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 9090
  initialDelaySeconds: 10

readinessProbe:
  httpGet:
    path: /ready
    port: 9090
  initialDelaySeconds: 5
```

### 4. ✅ 优雅关闭 (Graceful Shutdown)

```go
// 1. 收到 SIGTERM
signal.Notify(quit, syscall.SIGTERM)

// 2. 进入 Drain Mode
atomic.StoreInt32(&s.draining, 1)  // /ready 返回 503

// 3. K8s 停止路由流量（5-10秒）

// 4. 等待现有连接关闭
time.Sleep(cfg.Lifecycle.DrainWaitTime)  // 最长 1 小时

// 5. 退出
```

**K8s 配置**：
```yaml
spec:
  terminationGracePeriodSeconds: 3600  # 1 小时（游戏长连接）
```

### 5. ✅ 可观测性 (Observability)

#### Metrics (Prometheus)

```go
// internal/middleware/metrics.go
var (
    requestsTotal = promauto.NewCounterVec(...)
    requestDuration = promauto.NewHistogramVec(...)
    activeConnections = promauto.NewGaugeVec(...)
)
```

**暴露端点**：`http://localhost:9090/metrics`

#### Distributed Tracing (OpenTelemetry)

```go
// internal/observability/tracing.go
func InitTracing(serviceName, jaegerEndpoint string) error {
    exp, _ := jaeger.New(jaeger.WithCollectorEndpoint(...))
    tp := trace.NewTracerProvider(trace.WithBatcher(exp))
    otel.SetTracerProvider(tp)
}
```

**使用**：
```go
ctx, span := observability.StartSpan(ctx, "gateway.request")
defer span.End()
```

#### Structured Logging

```go
// pkg/xlog/logger.go
xlog.Infof("Request: method=%s path=%s", r.Method, r.URL.Path)
// 输出: {"level":"info","msg":"Request","method":"GET","path":"/api/users"}
```

### 6. ✅ ConfigMap 热加载

```go
// internal/config/k8s.go
func (w *K8sConfigWatcher) watch() {
    for {
        // 检查 ConfigMap 文件修改时间
        if info.ModTime().After(lastModTime) {
            cfg := LoadConfigFromFile(w.configPath)
            w.onChange(cfg)  // 热更新配置
        }
    }
}
```

**K8s 配置**：
```yaml
volumes:
- name: config
  configMap:
    name: gateway-config
volumeMounts:
- name: config
  mountPath: /etc/config
```

### 7. ✅ 服务网格就绪 (Service Mesh Ready)

```go
// internal/middleware/cloudnative.go
func ServiceMeshMiddleware(next http.Handler) http.Handler {
    // 传播 Istio/Linkerd trace context
    if traceID := r.Header.Get("X-B3-TraceId"); traceID != "" {
        w.Header().Set("X-B3-TraceId", traceID)
    }
}
```

**Istio 集成**：
```yaml
# 自动注入 sidecar
apiVersion: v1
kind: Pod
metadata:
  annotations:
    sidecar.istio.io/inject: "true"
```

### 8. ✅ 自动扩缩容 (HPA)

```yaml
# deploy/hpa.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
spec:
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        averageUtilization: 70
  - type: Pods
    pods:
      metric:
        name: gateway_requests_per_second
      target:
        averageValue: "1000"
```

### 9. ✅ 资源限制

```yaml
resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 2000m
    memory: 2Gi
```

### 10. ✅ 多环境支持

```go
// 自动检测运行环境
if discovery.IsRunningInK8s() {
    // K8s 模式：使用 Service Discovery
    addr, _ := discovery.ResolveService("backend-service")
} else {
    // 本地模式：使用 localhost
    addr = "localhost:5000"
}
```

## 云原生架构图

```
┌─────────────────────────────────────────────────────────┐
│              Kubernetes Cluster                         │
│                                                         │
│  ┌─────────────────────────────────────────────────┐  │
│  │  Unified Access Gateway (Deployment)            │  │
│  │  ┌───────────────────────────────────────────┐ │  │
│  │  │  Pod (3 replicas)                         │ │  │
│  │  │  ┌─────────────────────────────────────┐  │ │  │
│  │  │  │  Gateway Container                   │  │ │  │
│  │  │  │  - Liveness: /health                 │  │ │  │
│  │  │  │  - Readiness: /ready                 │  │ │  │
│  │  │  │  - Metrics: /metrics                 │  │ │  │
│  │  │  │  - Tracing: OpenTelemetry            │  │ │  │
│  │  │  └─────────────────────────────────────┘  │ │  │
│  │  └───────────────────────────────────────────┘ │  │
│  └─────────────────────────────────────────────────┘  │
│                        │                                │
│  ┌─────────────────────▼────────────────────────────┐  │
│  │  Service (ClusterIP)                             │  │
│  │  - DNS: gateway-service.namespace.svc.cluster.local│ │
│  └──────────────────────────────────────────────────┘  │
│                        │                                │
│  ┌─────────────────────▼────────────────────────────┐  │
│  │  HPA (Horizontal Pod Autoscaler)                 │  │
│  │  - CPU: 70%                                       │  │
│  │  - QPS: 1000 req/s                                │  │
│  └──────────────────────────────────────────────────┘  │
│                                                         │
│  ┌─────────────────────────────────────────────────┐  │
│  │  ConfigMap                                       │  │
│  │  - gateway-config.yaml                          │  │
│  └─────────────────────────────────────────────────┘  │
│                                                         │
│  ┌─────────────────────────────────────────────────┐  │
│  │  Prometheus (Metrics)                           │  │
│  │  - Scrapes /metrics                              │  │
│  └─────────────────────────────────────────────────┘  │
│                                                         │
│  ┌─────────────────────────────────────────────────┐  │
│  │  Jaeger (Tracing)                                │  │
│  │  - Receives OpenTelemetry traces                 │  │
│  └─────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

## 对比：传统 vs 云原生

| 特性 | 传统应用 | 云原生应用 |
|------|---------|-----------|
| **配置** | 硬编码/配置文件 | 环境变量/ConfigMap |
| **服务发现** | 静态 IP/配置文件 | K8s DNS/Service |
| **健康检查** | 无/简单 ping | Liveness/Readiness Probes |
| **扩缩容** | 手动 | HPA 自动 |
| **日志** | 文件 | 标准输出/结构化 |
| **监控** | 无/简单 | Prometheus + Grafana |
| **追踪** | 无 | OpenTelemetry + Jaeger |
| **部署** | 手动/脚本 | K8s Deployment |
| **优雅关闭** | 无 | SIGTERM + Drain Mode |

## 总结

**本项目的云原生特性**：

1. ✅ **12-Factor App**：配置外部化、无状态、日志即流
2. ✅ **K8s 原生**：服务发现、健康检查、自动扩缩容
3. ✅ **可观测性**：Prometheus Metrics、OpenTelemetry Tracing
4. ✅ **弹性设计**：优雅关闭、自动恢复、故障隔离
5. ✅ **服务网格就绪**：Istio/Linkerd 兼容
6. ✅ **多环境支持**：K8s/本地自动切换

**这不是"能在 K8s 上运行"，而是"为 K8s 而生"！** 🚀

