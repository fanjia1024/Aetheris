package api

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	hertzslog "github.com/hertz-contrib/logger/slog"
	"github.com/hertz-contrib/obs-opentelemetry/provider"
	hertztracing "github.com/hertz-contrib/obs-opentelemetry/tracing"

	"rag-platform/internal/api/http"
	"rag-platform/internal/api/http/middleware"
	"rag-platform/internal/app"
	"rag-platform/internal/pipeline/ingest"
	"rag-platform/internal/pipeline/query"
	"rag-platform/internal/runtime/eino"
	"rag-platform/internal/storage/vector"
)

// otelProviderShutdown 用于优雅关闭时关闭 OpenTelemetry provider
type otelProviderShutdown interface {
	Shutdown(ctx context.Context) error
}

// App API 应用（装配 HTTP Router、Handler、Middleware；仅依赖 Engine + DocumentService）
type App struct {
	config       *app.Bootstrap
	engine       *eino.Engine
	router       *http.Router
	hertz        *server.Hertz
	otelProvider otelProviderShutdown
}

// NewApp 创建 API 应用（由 cmd/api 调用）
func NewApp(bootstrap *app.Bootstrap) (*App, error) {
	engine, err := eino.NewEngine(bootstrap.Config, bootstrap.Logger)
	if err != nil {
		return nil, fmt.Errorf("初始化 eino 引擎失败: %w", err)
	}

	// 装配并注册 query_pipeline（Retriever + Generator + queryEmbedder）
	if bootstrap.Config != nil && bootstrap.VectorStore != nil {
		llmClient, errLLM := app.NewLLMClientFromConfig(bootstrap.Config)
		queryEmbedder, errEmb := app.NewQueryEmbedderFromConfig(bootstrap.Config)
		if errLLM == nil && errEmb == nil && llmClient != nil && queryEmbedder != nil {
			retriever := query.NewRetriever(bootstrap.VectorStore, "default", 10, 0.3)
			generator := query.NewGenerator(llmClient, 4096, 0.1)
			qwf := eino.NewQueryWorkflowExecutor(retriever, generator, queryEmbedder, bootstrap.Logger)
			if err := engine.RegisterWorkflow("query_pipeline", qwf); err != nil {
				bootstrap.Logger.Info("注册 query_pipeline 失败，将使用占位实现", "error", err)
			}
		}
	}

	// 装配并注册 ingest_pipeline（loader → parser → splitter → embedding → indexer）
	if bootstrap.Config != nil && bootstrap.VectorStore != nil && bootstrap.MetadataStore != nil {
		ingestEmbedder, errEmb := app.NewQueryEmbedderFromConfig(bootstrap.Config)
		if errEmb == nil && ingestEmbedder != nil {
			docEmbedding := ingest.NewDocumentEmbedding(ingestEmbedder, 4)
			docIndexer := ingest.NewDocumentIndexer(bootstrap.VectorStore, bootstrap.MetadataStore, 4, 100)
			if err := vector.EnsureIndex(context.Background(), bootstrap.VectorStore, "default", ingestEmbedder.Dimension(), "cosine"); err != nil {
				bootstrap.Logger.Info("创建 default 向量索引失败（首次写入时可能再创建）", "error", err)
			}
			loader := ingest.NewDocumentLoader()
			parser := ingest.NewDocumentParser()
			splitter := ingest.NewDocumentSplitter(1000, 100, 1000)
			iwf := eino.NewIngestWorkflowExecutor(loader, parser, splitter, docEmbedding, docIndexer, bootstrap.Logger)
			if err := engine.RegisterWorkflow("ingest_pipeline", iwf); err != nil {
				bootstrap.Logger.Info("注册 ingest_pipeline 失败，将使用占位实现", "error", err)
			}
		}
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

	// 可选：启用链路追踪（OpenTelemetry）
	if a.config.Config != nil && a.config.Config.Monitoring.Tracing.Enable {
		serviceName := a.config.Config.Monitoring.Tracing.ServiceName
		if serviceName == "" {
			serviceName = "rag-api"
		}
		exportEndpoint := a.config.Config.Monitoring.Tracing.ExportEndpoint
		if exportEndpoint == "" {
			exportEndpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
		}
		if exportEndpoint != "" {
			opts := []provider.Option{
				provider.WithServiceName(serviceName),
				provider.WithExportEndpoint(exportEndpoint),
			}
			if a.config.Config.Monitoring.Tracing.Insecure {
				opts = append(opts, provider.WithInsecure())
			}
			p := provider.NewOpenTelemetryProvider(opts...)
			a.otelProvider = p
			tracerOpt, cfg := hertztracing.NewServerTracer()
			a.hertz = a.router.Build(addr, tracerOpt)
			a.hertz.Use(hertztracing.ServerMiddleware(cfg))
			a.config.Logger.Info("链路追踪已启用", "service_name", serviceName, "endpoint", exportEndpoint)
		} else {
			a.hertz = a.router.Build(addr)
		}
	} else {
		a.hertz = a.router.Build(addr)
	}
	return a.hertz.Run()
}

// Shutdown 优雅关闭（传入 ctx 以支持超时，如 cmd 层 WithTimeout）
func (a *App) Shutdown(ctx context.Context) error {
	if a.otelProvider != nil {
		_ = a.otelProvider.Shutdown(ctx)
	}
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
