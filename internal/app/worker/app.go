package worker

import (
	"context"
	"fmt"

	"rag-platform/internal/runtime/eino"
	"rag-platform/internal/storage/metadata"
	"rag-platform/internal/storage/vector"
	"rag-platform/pkg/config"
	"rag-platform/pkg/log"
)

// App Worker 应用（Pipeline 由 eino 调度，不持有主动运行的 Pipeline）
type App struct {
	config        *config.Config
	logger        *log.Logger
	engine        *eino.Engine
	metadataStore metadata.Store
	vectorStore   vector.Store
	shutdown      chan struct{}
}

// NewApp 创建新的 Worker 应用
func NewApp(cfg *config.Config) (*App, error) {
	// 初始化日志
	logCfg := &log.Config{}
	if cfg != nil {
		logCfg.Level = cfg.Log.Level
		logCfg.Format = cfg.Log.Format
		logCfg.File = cfg.Log.File
	}
	logger, err := log.NewLogger(logCfg)
	if err != nil {
		return nil, fmt.Errorf("初始化日志失败: %w", err)
	}

	// 初始化存储
	metadataStore, err := metadata.NewStore(cfg.Storage.Metadata)
	if err != nil {
		return nil, fmt.Errorf("初始化元数据存储失败: %w", err)
	}

	vectorStore, err := vector.NewStore(cfg.Storage.Vector)
	if err != nil {
		return nil, fmt.Errorf("初始化向量存储失败: %w", err)
	}

	// 初始化 eino 引擎（ingest 任务通过 ExecuteWorkflow(ctx, "ingest_pipeline", payload) 执行）
	engine, err := eino.NewEngine(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("初始化 eino 引擎失败: %w", err)
	}

	return &App{
		config:        cfg,
		logger:        logger,
		engine:        engine,
		metadataStore: metadataStore,
		vectorStore:   vectorStore,
		shutdown:      make(chan struct{}),
	}, nil
}

// Start 启动应用（Pipeline 由 eino 调度，不主动 Start；仅启动任务队列消费者）
func (a *App) Start() error {
	a.logger.Info("启动 worker 应用")

	// 启动工作队列消费者：收到入库任务时调用 engine.ExecuteWorkflow(ctx, "ingest_pipeline", payload)
	if err := a.startWorkerQueue(); err != nil {
		return fmt.Errorf("启动工作队列失败: %w", err)
	}

	a.logger.Info("worker 应用启动成功")
	return nil
}

// Shutdown 关闭应用
func (a *App) Shutdown(ctx context.Context) error {
	a.logger.Info("关闭 worker 应用")

	// 关闭工作队列
	close(a.shutdown)

	// 关闭存储
	if err := a.metadataStore.Close(); err != nil {
		a.logger.Error("关闭元数据存储失败", "error", err)
	}

	if err := a.vectorStore.Close(); err != nil {
		a.logger.Error("关闭向量存储失败", "error", err)
	}

	// 关闭 eino 引擎
	if err := a.engine.Shutdown(); err != nil {
		a.logger.Error("关闭 eino 引擎失败", "error", err)
	}

	a.logger.Info("worker 应用关闭成功")
	return nil
}

// startWorkerQueue 启动工作队列消费者；每个入库任务应调用 a.engine.ExecuteWorkflow(ctx, "ingest_pipeline", taskPayload)
func (a *App) startWorkerQueue() error {
	// 任务驱动、eino 执行：从队列取到任务后调用 engine.ExecuteWorkflow(..., "ingest_pipeline", payload)
	// 暂无实际队列实现时仅返回成功
	return nil
}

