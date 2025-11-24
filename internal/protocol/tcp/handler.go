package tcp

import (
	"io"
	"net"
	
	"hgame-gateway/pkg/xlog"
)

type Handler struct {
	backendAddr string
}

func NewHandler() *Handler {
	// 这里的 Backend 应该从配置读取
	return &Handler{
		backendAddr: "hgame-game-gateway:9621",
	}
}

func (h *Handler) Handle(src net.Conn) {
	defer src.Close()

	// 连接后端
	dst, err := net.Dial("tcp", h.backendAddr)
	if err != nil {
		xlog.Errorf("Failed to dial backend %s: %v", h.backendAddr, err)
		return
	}
	defer dst.Close()

	// 双向拷贝
	go io.Copy(dst, src)
	io.Copy(src, dst)
}

