# 编译指南

## 问题：eBPF 必须在 Linux 上编译吗？

**答案**：**Go 代码可以在 Windows 编译，eBPF 程序需要 Linux 环境编译。**

## 编译策略

### 方案 1：Windows 开发 + Docker 编译 eBPF（推荐）

```bash
# 1. Windows 上开发 Go 代码（eBPF 自动禁用）
go build -o uag.exe ./cmd/gateway
# ✅ 成功！因为有 stub.go，eBPF 自动禁用

# 2. 使用 Docker 编译 eBPF 部分
docker run --rm -v ${PWD}:/workspace -w /workspace \
  golang:1.21-alpine sh -c "
    apk add clang llvm
    go install github.com/cilium/ebpf/cmd/bpf2go@latest
    cd pkg/ebpf
    go generate ./sockmap.go
    go generate ./xdp.go
  "

# 3. 生成的 .o 和 .go 文件在 Windows 上可见
# 4. 最终在 Linux 上构建完整版本
```

### 方案 2：WSL2（Windows Subsystem for Linux）

```bash
# 1. 安装 WSL2
wsl --install -d Ubuntu-22.04

# 2. 在 WSL2 中安装依赖
sudo apt-get update
sudo apt-get install -y clang llvm

# 3. 在 WSL2 中编译
cd /mnt/h/hgame/hgame_proj_svr/unified-access-gateway
make generate-ebpf
make build
```

### 方案 3：CI/CD 自动编译（生产推荐）

```yaml
# .github/workflows/build.yml
name: Build

on: [push, pull_request]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Install dependencies
        run: |
          sudo apt-get update
          sudo apt-get install -y clang llvm
          go install github.com/cilium/ebpf/cmd/bpf2go@latest
      
      - name: Generate eBPF
        run: make generate-ebpf
      
      - name: Build
        run: make build
      
      - name: Upload artifacts
        uses: actions/upload-artifact@v3
        with:
          name: uag-binary
          path: uag
```

### 方案 4：Docker Multi-stage Build（最简单）

```dockerfile
# Dockerfile 已经实现了这个！
# 直接运行：
docker build -t skynet/unified-access-gateway:latest .
```

## 当前代码的跨平台支持

### Windows 编译（无 eBPF）

```powershell
# 直接编译，eBPF 自动禁用
go build -o uag.exe ./cmd/gateway

# 运行（eBPF 功能不可用，但网关正常工作）
.\uag.exe
```

**为什么可以？** 因为有 `pkg/ebpf/stub.go` 和 `pkg/ebpf/xdp_stub.go`：

```go
// +build !linux
// stub.go - 非 Linux 平台的桩实现
func NewSockMapManager() (*SockMapManager, error) {
    return &SockMapManager{enabled: false}, nil  // 自动禁用
}
```

### Linux 编译（完整功能）

```bash
# 1. 生成 eBPF 绑定
make generate-ebpf

# 2. 编译
make build

# 3. 运行（eBPF 功能可用）
./uag
```

## 文件说明

| 文件 | Windows 编译 | Linux 编译 | 说明 |
|------|-------------|-----------|------|
| `sockmap.c` | ❌ 不编译 | ✅ 编译 | eBPF C 程序 |
| `sockmap.go` | ✅ 编译（stub） | ✅ 编译（真实） | Go 加载器 |
| `stub.go` | ✅ 使用 | ❌ 忽略 | Windows 桩实现 |
| `xdp_filter.c` | ❌ 不编译 | ✅ 编译 | XDP C 程序 |
| `xdp.go` | ✅ 编译（stub） | ✅ 编译（真实） | XDP 管理器 |
| `xdp_stub.go` | ✅ 使用 | ❌ 忽略 | Windows 桩实现 |

## 推荐工作流

### 开发阶段（Windows）

```powershell
# 1. 在 Windows 上开发 Go 代码
go build -o uag.exe ./cmd/gateway
go test ./...

# 2. eBPF 部分用 Docker 测试
docker run --rm -v ${PWD}:/workspace -w /workspace \
  golang:1.21-alpine sh -c "cd pkg/ebpf && go generate"
```

### 测试阶段（Linux 或 Docker）

```bash
# 1. 在 Linux 机器或 Docker 中完整编译
make generate-ebpf
make build

# 2. 测试 eBPF 功能
./uag  # 检查日志中是否有 "eBPF SockMap loaded successfully"
```

### 生产部署（CI/CD）

```yaml
# GitHub Actions / GitLab CI 自动编译
# 生成包含 eBPF 的完整二进制文件
```

## 总结

| 场景 | 方案 | eBPF 功能 |
|------|------|-----------|
| **Windows 开发** | 直接编译 | ❌ 禁用（但网关可用） |
| **WSL2** | 在 WSL2 中编译 | ✅ 完整功能 |
| **Docker** | Multi-stage build | ✅ 完整功能 |
| **CI/CD** | Linux runner | ✅ 完整功能 |

**关键点**：
- ✅ Go 代码可以在任何平台编译
- ✅ eBPF 程序必须在 Linux 上编译
- ✅ 有优雅降级（Windows 自动禁用 eBPF）
- ✅ 生产环境用 Docker/CI 编译

