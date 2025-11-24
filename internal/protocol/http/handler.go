package http

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	
	"hgame-gateway/pkg/xlog"
)

type Handler struct {
	proxy *httputil.ReverseProxy
}

func NewHandler() *Handler {
	// 这里的 Backend URL 应该从配置读取，暂时硬编码
	target, _ := url.Parse("http://hgame-httpproxy:8181")
	
	return &Handler{
		proxy: httputil.NewSingleHostReverseProxy(target),
	}
}

// ServeConn 处理单个 HTTP 连接
// 由于 Go 标准库 http.Server 需要 Listener，这里我们用一个小技巧：
// 使用 http.Serve 配合一个自定义的 "OneShotListener" 或者简单地读取 Request 并 Proxy
func (h *Handler) ServeConn(c net.Conn) {
	defer c.Close()

	// 读取请求
	req, err := http.ReadRequest(bufio.NewReader(c))
	if err != nil {
		xlog.Errorf("Failed to read HTTP request: %v", err)
		return
	}

	// 可以在这里做 HTTP 层的路由分发
	// ...

	// 目前简单地作为反向代理，但这有点复杂，因为 ReverseProxy 是 http.Handler
	// 更好的方式是将 sniffConn 作为一个 net.Listener 的 Accept 返回值喂给 http.Server
	// 但为了简化 POC，我们暂时不实现完整的 HTTP Server 逻辑，而是提示架构位置
	
	xlog.Infof("Received HTTP Request: %s %s", req.Method, req.URL.Path)
	
	// 实际项目中，这里应该将 c 注入到一个 channel，由一个运行中的 http.Server (自定义 Listener) 去 Accept
}

