package api

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	hertzslog "github.com/hertz-contrib/logger/slog"

	"rag-platform/internal/api/http"
	"rag-platform/internal/api/http/middleware"
	"rag-platform/internal/app"
	"rag-platform/internal/runtime/eino"
)

// App API 应用（装配 HTTP Router、Handler、Middleware；仅依赖 Engine + DocumentService）
type App struct {
	config *app.Bootstrap
	engine *eino.Engine
	router *http.Router
	hertz  *server.Hertz
}

// NewApp 创建 API 应用（由 cmd/api 调用）
func NewApp(bootstrap *app.Bootstrap) (*App, error) {
	engine, err := eino.NewEngine(bootstrap.Config, bootstrap.Logger)
	if err != nil {
		return nil, fmt.Errorf("初始化 eino 引擎失败: %w", err)
	}

	docService := app.NewDocumentService(bootstrap.MetadataStore)
	handler := http.NewHandler(engine, docService)
	mw := middleware.NewMiddleware()
	router := http.NewRouter(handler, mw)

	return &App{
		config: bootstrap,
		engine: engine,
		router: router,
		hertz:  nil,
	}, nil
}

// Run 启动 HTTP 服务，addr 如 ":8080"
func (a *App) Run(addr string) error {
	a.config.Logger.Info("API 服务启动", "addr", addr)

	// 使用 Hertz slog 扩展，与 bootstrap 配置对齐
	output := os.Stdout
	if a.config.Config != nil && a.config.Config.Log.File != "" {
		f, err := os.OpenFile(a.config.Config.Log.File, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("打开日志文件失败: %w", err)
		}
		output = f
	}
	levelVar := &slog.LevelVar{}
	if a.config.Config != nil && a.config.Config.Log.Level != "" {
		switch a.config.Config.Log.Level {
		case "debug":
			levelVar.Set(slog.LevelDebug)
		case "warn":
			levelVar.Set(slog.LevelWarn)
		case "error":
			levelVar.Set(slog.LevelError)
		default:
			levelVar.Set(slog.LevelInfo)
		}
	} else {
		levelVar.Set(slog.LevelInfo)
	}
	hertzLogger := hertzslog.NewLogger(
		hertzslog.WithOutput(output),
		hertzslog.WithLevel(levelVar),
	)
	hlog.SetLogger(hertzLogger)

	a.hertz = a.router.Build(addr)
	return a.hertz.Run()
}

// Shutdown 优雅关闭（传入 ctx 以支持超时，如 cmd 层 WithTimeout）
func (a *App) Shutdown(ctx context.Context) error {
	if a.hertz != nil {
		if err := a.hertz.Shutdown(ctx); err != nil {
			return err
		}
	}
	if err := a.engine.Shutdown(); err != nil {
		return err
	}
	return nil
}
