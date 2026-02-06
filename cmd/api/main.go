package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"rag-platform/internal/app"
	"rag-platform/internal/app/api"
	"rag-platform/pkg/config"
)

func main() {
	cfg, err := config.LoadAPIConfigWithModel()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	bootstrap, err := app.NewBootstrap(cfg)
	if err != nil {
		log.Fatalf("初始化失败: %v", err)
	}

	application, err := api.NewApp(bootstrap)
	if err != nil {
		log.Fatalf("创建 API 应用失败: %v", err)
	}

	addr := ":8080"
	if cfg != nil && cfg.API.Port > 0 {
		addr = fmt.Sprintf(":%d", cfg.API.Port)
	}

	go func() {
		if err := application.Run(addr); err != nil && err != http.ErrServerClosed {
			log.Printf("API 服务异常退出: %v", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := application.Shutdown(ctx); err != nil {
		log.Printf("关闭失败: %v", err)
	}
	log.Println("API 服务已关闭")
}
