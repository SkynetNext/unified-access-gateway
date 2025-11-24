# Go 云原生特性详解（新手版）

## 什么是"云原生"？

**简单理解**：云原生 = **为 Kubernetes 而生**，不是"能在 K8s 上运行"，而是"专门为 K8s 设计"。

**传统应用**：写死 IP、写死端口、写死配置，换个环境就报错  
**云原生应用**：自动发现服务、配置外部化、自动扩缩容，换个环境照样跑

---

## 本项目中的 8 大云原生特性

### 1️⃣ **配置外部化** - 不写死配置

#### ❌ 传统写法（不云原生）
```go
// 写死在代码里
server := http.ListenAndServe(":8080", nil)  // 端口写死
backend := "http://192.168.1.100:5000"        // IP 写死
```

#### ✅ 云原生写法（本项目）
```go
// internal/config/config.go
func LoadConfig() *Config {
    return &Config{
        Server: ServerConfig{
            // 从环境变量读取，没有就用默认值
            ListenAddr: getEnv("GATEWAY_LISTEN_ADDR", ":8080"),
        },
        Backends: BackendsConfig{
            HTTP: HTTPBackend{
                // 从环境变量读取后端地址
                TargetURL: getEnv("HTTP_BACKEND_URL", "http://localhost:5000"),
            },
        },
    }
}
```

**好处**：
- 开发环境：`HTTP_BACKEND_URL=http://localhost:5000`
- 测试环境：`HTTP_BACKEND_URL=http://test-backend:5000`
- 生产环境：`HTTP_BACKEND_URL=http://prod-backend:5000`
- **同一份代码，不同环境！**

**K8s 中使用**：
```yaml
# deploy/deployment.yaml
env:
- name: HTTP_BACKEND_URL
  value: "http://httpproxy-service:5000"  # K8s 自动注入
```

---

### 2️⃣ **服务发现** - 不写死 IP

#### ❌ 传统写法
```go
// 写死 IP 地址
backend := "http://192.168.1.100:5000"
```

**问题**：Pod 重启 IP 变了怎么办？多个 Pod 怎么负载均衡？

#### ✅ 云原生写法（本项目）
```go
// internal/discovery/k8s.go
func (k *K8sServiceDiscovery) ResolveService(serviceName string) (string, error) {
    // 使用 K8s DNS 解析服务名
    // 格式：<service>.<namespace>.svc.cluster.local
    fqdn := fmt.Sprintf("%s.%s.svc.cluster.local", serviceName, k.namespace)
    ips, err := net.LookupIP(fqdn)
    return ips[0].String(), nil
}
```

**使用**：
```go
// cmd/gateway/main.go
svcDiscovery := discovery.NewK8sServiceDiscovery()
if httpBackend := os.Getenv("HTTP_BACKEND_SERVICE"); httpBackend != "" {
    // 自动解析 K8s Service 名称
    addr, _ := svcDiscovery.ResolveServiceWithPort(httpBackend, 5000)
    // 结果：10.244.1.5:5000（自动找到 Pod IP）
}
```

**好处**：
- 不用写 IP，写服务名：`httpproxy-service`
- K8s 自动负载均衡（多个 Pod 自动轮询）
- Pod 重启 IP 变了？自动更新！

---

### 3️⃣ **健康检查** - 告诉 K8s 我是否健康

#### 为什么需要？

K8s 需要知道：
- **Liveness**：这个 Pod 还活着吗？（死了就重启）
- **Readiness**：这个 Pod 能接收流量吗？（不能就摘除）

#### ✅ 本项目实现
```go
// internal/core/server.go

// 健康检查：我还活着
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("OK"))
}

// 就绪检查：我能接收流量吗？
func (s *Server) readyHandler(w http.ResponseWriter, r *http.Request) {
    if atomic.LoadInt32(&s.draining) == 1 {
        // 正在关闭，返回 503，告诉 K8s 不要给我流量
        w.WriteHeader(http.StatusServiceUnavailable)
        w.Write([]byte("Draining"))
        return
    }
    // 正常，返回 200，告诉 K8s 可以给我流量
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("Ready"))
}
```

**K8s 配置**：
```yaml
# deploy/deployment.yaml
livenessProbe:
  httpGet:
    path: /health    # 检查是否活着
    port: 9090
  initialDelaySeconds: 15

readinessProbe:
  httpGet:
    path: /ready     # 检查是否就绪
    port: 9090
  initialDelaySeconds: 5
```

**效果**：
- Pod 挂了 → Liveness 失败 → K8s 自动重启
- Pod 启动中 → Readiness 失败 → K8s 不给流量
- Pod 关闭中 → Readiness 返回 503 → K8s 停止给流量

---

### 4️⃣ **优雅关闭** - 不中断用户连接

#### 问题场景

游戏网关有长连接（玩家在线），如果直接 `kill`：
- 玩家掉线！
- 数据丢失！

#### ✅ 本项目实现
```go
// cmd/gateway/main.go
// 7. 等待关闭信号（K8s 发 SIGTERM）
quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
sig := <-quit

// 8. 优雅关闭
server.GracefulShutdown(cfg.Lifecycle.ShutdownTimeout)
```

```go
// internal/core/server.go
func (s *Server) GracefulShutdown(timeout time.Duration) {
    // 1. 标记为"正在关闭"
    atomic.StoreInt32(&s.draining, 1)  // /ready 返回 503
    
    // 2. 等待 K8s 停止给流量（5-10秒）
    time.Sleep(5 * time.Second)
    
    // 3. 停止接收新连接
    s.listener.Stop()
    
    // 4. 等待现有连接关闭（最长 1 小时，给游戏玩家时间）
    time.Sleep(timeout)
    
    // 5. 退出
}
```

**K8s 配置**：
```yaml
# deploy/deployment.yaml
spec:
  terminationGracePeriodSeconds: 3600  # 给 1 小时时间关闭
```

**效果**：
1. K8s 发 `SIGTERM` → 程序收到信号
2. `/ready` 返回 503 → K8s 停止给新流量
3. 等待现有连接关闭 → 玩家不掉线
4. 超时后强制关闭 → 保证能退出

---

### 5️⃣ **Metrics 暴露** - 让 Prometheus 监控我

#### 为什么需要？

K8s 需要监控：
- CPU 使用率（自动扩缩容）
- 请求量（QPS）
- 错误率
- 延迟

#### ✅ 本项目实现
```go
// internal/core/server.go
func (s *Server) Start() {
    mux := http.NewServeMux()
    mux.Handle("/metrics", promhttp.Handler())  // 暴露 Prometheus 指标
    http.ListenAndServe(s.cfg.Metrics.ListenAddr, mux)
}
```

```go
// internal/middleware/metrics.go
var (
    // 总请求数
    requestsTotal = promauto.NewCounterVec(...)
    // 请求延迟
    requestDuration = promauto.NewHistogramVec(...)
    // 活跃连接数
    activeConnections = promauto.NewGaugeVec(...)
)
```

**访问**：
```bash
curl http://localhost:9090/metrics
# 输出：
# gateway_requests_total{protocol="http",status="200"} 12345
# gateway_request_duration_seconds{protocol="http"} 0.001
# gateway_active_connections{protocol="tcp"} 1000
```

**K8s 配置**：
```yaml
# deploy/deployment.yaml
annotations:
  prometheus.io/scrape: "true"  # Prometheus 自动抓取
  prometheus.io/port: "9090"
```

**效果**：
- Prometheus 自动抓取 `/metrics`
- Grafana 自动画图
- HPA 根据 CPU/QPS 自动扩缩容

---

### 6️⃣ **分布式追踪** - 追踪请求经过哪些服务

#### 为什么需要？

微服务架构：
```
用户 → 网关 → 服务A → 服务B → 数据库
```

出错了，是哪个服务的问题？

#### ✅ 本项目实现
```go
// cmd/gateway/main.go
// 2. 初始化分布式追踪
jaegerEndpoint := os.Getenv("JAEGER_ENDPOINT")
if jaegerEndpoint != "" {
    observability.InitTracing("unified-access-gateway", jaegerEndpoint)
}
```

```go
// internal/middleware/cloudnative.go
func CloudNativeMiddleware(next http.Handler) http.Handler {
    // 1. 提取追踪上下文（从上游服务）
    ctx := observability.ExtractTraceContext(r.Context(), r)
    
    // 2. 创建新的 span（记录这个请求）
    ctx, span := observability.StartSpan(ctx, "gateway.request")
    defer span.End()
    
    // 3. 记录请求信息
    span.SetAttributes(
        attribute.String("http.method", r.Method),
        attribute.String("http.path", r.URL.Path),
    )
    
    // 4. 传递给下游服务
    observability.InjectTraceContext(ctx, r)
}
```

**效果**：
- 每个请求有唯一 TraceID
- 可以看到请求经过：网关 → 服务A → 服务B
- 出错了，直接定位到具体服务

---

### 7️⃣ **自动扩缩容 (HPA)** - 根据负载自动增减 Pod

#### 为什么需要？

- 白天流量大 → 需要 10 个 Pod
- 晚上流量小 → 只需要 3 个 Pod
- 手动调整？太累！

#### ✅ 本项目支持
```yaml
# deploy/deployment.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
spec:
  minReplicas: 3      # 最少 3 个
  maxReplicas: 20     # 最多 20 个
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        averageUtilization: 60  # CPU > 60% 就扩容
```

**效果**：
- CPU > 60% → 自动增加 Pod
- CPU < 30% → 自动减少 Pod
- **完全自动化！**

---

### 8️⃣ **多环境自动切换** - 一套代码，多个环境

#### ✅ 本项目实现
```go
// cmd/gateway/main.go
// 1. 检查是否在 K8s 中
if discovery.IsRunningInK8s() {
    // K8s 模式：使用服务发现
    xlog.Infof("Running in Kubernetes: Pod=%s", discovery.GetPodName())
    addr, _ := svcDiscovery.ResolveService("backend-service")
} else {
    // 本地模式：使用 localhost
    addr = "localhost:5000"
}
```

**效果**：
- 本地开发：自动用 `localhost`
- K8s 部署：自动用服务发现
- **一套代码，自动适配！**

---

## 总结对比

| 特性 | 传统应用 | 云原生应用（本项目） |
|------|---------|---------------------|
| **配置** | 写死在代码 | ✅ 环境变量/ConfigMap |
| **服务地址** | 写死 IP | ✅ K8s DNS 服务发现 |
| **健康检查** | 无 | ✅ `/health` + `/ready` |
| **关闭** | 直接 kill | ✅ 优雅关闭（Drain Mode） |
| **监控** | 无/手动 | ✅ Prometheus Metrics |
| **追踪** | 无 | ✅ OpenTelemetry |
| **扩缩容** | 手动 | ✅ HPA 自动 |
| **多环境** | 改代码 | ✅ 自动适配 |

---

## 关键代码位置

| 云原生特性 | 代码文件 | 关键函数 |
|-----------|---------|---------|
| **配置外部化** | `internal/config/config.go` | `LoadConfig()` |
| **服务发现** | `internal/discovery/k8s.go` | `ResolveService()` |
| **健康检查** | `internal/core/server.go` | `healthHandler()`, `readyHandler()` |
| **优雅关闭** | `internal/core/server.go` | `GracefulShutdown()` |
| **Metrics** | `internal/middleware/metrics.go` | `RecordMetrics()` |
| **追踪** | `internal/observability/tracing.go` | `InitTracing()` |
| **主入口** | `cmd/gateway/main.go` | `main()` |

---

## 新手常见问题

### Q1: 为什么不用配置文件？

**A**: 配置文件需要：
- 不同环境不同文件（dev.yaml, prod.yaml）
- 部署时复制文件
- 修改配置要重新打包

**环境变量**：
- 一套代码，环境变量不同
- K8s 自动注入
- 修改配置不用重新打包

### Q2: 服务发现有什么用？

**A**: 传统方式：
```
网关 → 192.168.1.100:5000  // 写死 IP
```

**问题**：
- Pod 重启 IP 变了 → 连不上
- 多个 Pod 怎么负载均衡？

**服务发现**：
```
网关 → httpproxy-service:5000  // 写服务名
```

**好处**：
- K8s 自动解析到 Pod IP
- 多个 Pod 自动负载均衡
- Pod 重启自动更新

### Q3: 优雅关闭为什么重要？

**A**: 直接 `kill`：
- 正在处理的请求 → 失败
- 长连接（游戏） → 玩家掉线
- 数据可能丢失

**优雅关闭**：
1. 停止接收新请求
2. 等待现有请求完成
3. 等待长连接关闭
4. 然后退出

**结果**：用户无感知！

---

## 下一步学习

1. **Kubernetes 基础**：Pod、Service、Deployment
2. **Prometheus**：Metrics 收集和查询
3. **OpenTelemetry**：分布式追踪
4. **HPA**：自动扩缩容原理

**推荐资源**：
- [Kubernetes 官方文档](https://kubernetes.io/docs/)
- [12-Factor App](https://12factor.net/)
- [Prometheus 文档](https://prometheus.io/docs/)

---

**记住**：云原生不是"能在 K8s 上运行"，而是"为 K8s 而生"！🚀

