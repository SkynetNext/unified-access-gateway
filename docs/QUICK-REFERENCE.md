# 云原生特性快速参考

## 一句话总结

**云原生 = 不写死配置、不写死 IP、告诉 K8s 我是否健康、优雅关闭、自动监控**

---

## 8 大特性速查表

### 1. 配置外部化
```go
// ❌ 不云原生
port := ":8080"  // 写死

// ✅ 云原生
port := os.Getenv("GATEWAY_LISTEN_ADDR")  // 从环境变量读
```
**文件**：`internal/config/config.go`

---

### 2. 服务发现
```go
// ❌ 不云原生
backend := "http://192.168.1.100:5000"  // 写死 IP

// ✅ 云原生
backend := discovery.ResolveService("httpproxy-service")  // 用服务名
```
**文件**：`internal/discovery/k8s.go`

---

### 3. 健康检查
```go
// ✅ 云原生
func readyHandler(w http.ResponseWriter, r *http.Request) {
    if draining {
        w.WriteHeader(503)  // 告诉 K8s：别给我流量
    } else {
        w.WriteHeader(200)  // 告诉 K8s：可以给我流量
    }
}
```
**文件**：`internal/core/server.go` (第 87 行)

---

### 4. 优雅关闭
```go
// ✅ 云原生
signal.Notify(quit, syscall.SIGTERM)  // 监听 K8s 关闭信号
<-quit
server.GracefulShutdown()  // 优雅关闭
```
**文件**：`cmd/gateway/main.go` (第 74-81 行)

---

### 5. Metrics 暴露
```go
// ✅ 云原生
mux.Handle("/metrics", promhttp.Handler())  // Prometheus 自动抓取
```
**文件**：`internal/core/server.go` (第 35 行)

---

### 6. 分布式追踪
```go
// ✅ 云原生
ctx, span := observability.StartSpan(ctx, "gateway.request")
defer span.End()
```
**文件**：`internal/observability/tracing.go`

---

### 7. 自动扩缩容
```yaml
# ✅ 云原生
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
spec:
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        averageUtilization: 60  # CPU > 60% 自动扩容
```
**文件**：`deploy/deployment.yaml` (第 83-110 行)

---

### 8. 多环境适配
```go
// ✅ 云原生
if discovery.IsRunningInK8s() {
    // K8s 模式
} else {
    // 本地模式
}
```
**文件**：`cmd/gateway/main.go` (第 19-24 行)

---

## 代码位置速查

| 特性 | 文件 | 行数 |
|------|------|------|
| 配置加载 | `internal/config/config.go` | 51-76 |
| 服务发现 | `internal/discovery/k8s.go` | 38-63 |
| 健康检查 | `internal/core/server.go` | 81-96 |
| 优雅关闭 | `internal/core/server.go` | 56-79 |
| Metrics | `internal/middleware/metrics.go` | 全部 |
| 追踪 | `internal/observability/tracing.go` | 全部 |
| 主入口 | `cmd/gateway/main.go` | 15-84 |

---

## K8s 配置速查

| 特性 | K8s 资源 | 配置位置 |
|------|---------|---------|
| 环境变量 | Deployment | `spec.template.spec.containers[0].env` |
| 健康检查 | Deployment | `spec.template.spec.containers[0].livenessProbe` |
| 就绪检查 | Deployment | `spec.template.spec.containers[0].readinessProbe` |
| 优雅关闭 | Deployment | `spec.template.spec.terminationGracePeriodSeconds` |
| Metrics | Deployment | `metadata.annotations.prometheus.io/scrape` |
| 自动扩缩容 | HPA | `deploy/deployment.yaml` (第 83 行) |

---

## 运行效果

### 本地运行
```bash
./uag.exe
# 输出：
# Starting Unified Access Gateway (UAG)...
# Config loaded: listen=:8080, metrics=:9090
```

### K8s 运行
```bash
kubectl logs <pod-name>
# 输出：
# Starting Unified Access Gateway (UAG)...
# Running in Kubernetes: Pod=gateway-abc123, Namespace=hgame
# Resolved HTTP backend: httpproxy-service -> 10.244.1.5:5000
# Config loaded from ConfigMap
```

---

## 验证清单

- [ ] 配置从环境变量读取（不是写死）
- [ ] 服务地址用服务名（不是写死 IP）
- [ ] 有 `/health` 和 `/ready` 端点
- [ ] 监听 `SIGTERM` 信号
- [ ] 有 `/metrics` 端点
- [ ] K8s Deployment 配置了 probes
- [ ] K8s Deployment 配置了 `terminationGracePeriodSeconds`
- [ ] 有 HPA 配置（可选）

---

**记住**：云原生 = 为 K8s 而生，不是"能在 K8s 上运行"！

