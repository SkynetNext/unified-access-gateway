package main

import (
	"os"
	"os/signal"
	"syscall"
	
	"hgame-gateway/internal/core"
	"hgame-gateway/pkg/xlog"
)

func main() {
	xlog.Infof("Starting Unified Gateway...")

	// 1. 加载配置 (TODO)
	
	// 2. 初始化监听器 (监听 :8080)
	listener := core.NewListener(":8080")
	
	// 3. 启动服务
	if err := listener.Start(); err != nil {
		xlog.Errorf("Failed to start listener: %v", err)
		os.Exit(1)
	}

	// 4. 优雅停机
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	xlog.Infof("Shutting down server...")
	listener.Stop()
	xlog.Infof("Server exited")
}

