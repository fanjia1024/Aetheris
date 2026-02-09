package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"rag-platform/internal/agent/job"
	"rag-platform/internal/agent/planner"
	"rag-platform/internal/agent/runtime"
	"rag-platform/internal/agent/tools"
	agentexec "rag-platform/internal/agent/runtime/executor"
	"rag-platform/internal/app"
	"rag-platform/internal/app/api"
	"rag-platform/internal/runtime/eino"
	"rag-platform/internal/runtime/jobstore"
	"rag-platform/internal/storage/metadata"
	"rag-platform/internal/storage/vector"
	"rag-platform/pkg/config"
	"rag-platform/pkg/log"
)

// App Worker 应用（Pipeline 由 eino 调度；JobStore=postgres 时拉取 Agent Job 执行）
type App struct {
	config          *config.Config
	logger          *log.Logger
	engine          *eino.Engine
	metadataStore   metadata.Store
	vectorStore     vector.Store
	shutdown        chan struct{}
	agentJobRunner  *AgentJobRunner
	agentJobCancel  context.CancelFunc
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

	appObj := &App{
		config:        cfg,
		logger:        logger,
		engine:        engine,
		metadataStore: metadataStore,
		vectorStore:   vectorStore,
		shutdown:      make(chan struct{}),
	}

	// Agent Job 模式：jobstore.type=postgres 时，从事件存储 Claim、从元数据存储取 Job、执行 DAG Runner
	if cfg != nil && cfg.JobStore.Type == "postgres" && cfg.JobStore.DSN != "" {
		dsn := cfg.JobStore.DSN
		leaseDur := 30 * time.Second
		if cfg.JobStore.LeaseDuration != "" {
			if d, err := time.ParseDuration(cfg.JobStore.LeaseDuration); err == nil && d > 0 {
				leaseDur = d
			}
		}
		pgEventStore, err := jobstore.NewPostgresStore(context.Background(), dsn, leaseDur)
		if err != nil {
			return nil, fmt.Errorf("初始化 JobStore 事件(postgres) 失败: %w", err)
		}
		pgJobStore, err := job.NewJobStorePg(context.Background(), dsn)
		if err != nil {
			return nil, fmt.Errorf("初始化 Job 元数据(postgres) 失败: %w", err)
		}
		llmClient, err := app.NewLLMClientFromConfig(cfg)
		if err != nil {
			return nil, fmt.Errorf("初始化 LLM 客户端失败: %w", err)
		}
		toolsReg := tools.NewRegistry()
		tools.RegisterBuiltin(toolsReg, engine, nil)
		var v1Planner planner.Planner
		if os.Getenv("PLANNER_TYPE") == "rule" {
			v1Planner = planner.NewRulePlanner()
			logger.Info("Worker 使用规则规划器")
		} else {
			v1Planner = planner.NewLLMPlanner(llmClient)
		}
		dagCompiler := api.NewDAGCompiler(llmClient, toolsReg, engine)
		dagRunner := api.NewDAGRunner(dagCompiler)
		checkpointStore := runtime.NewCheckpointStoreMem()
		dagRunner.SetCheckpointStores(checkpointStore, &jobStoreForRunnerAdapter{JobStore: pgJobStore})
		runJob := func(ctx context.Context, j *job.Job) error {
			sess := runtime.NewSession("", j.AgentID)
			plannerProv := newPlannerProviderAdapter(v1Planner)
			toolsProv := newToolsProviderAdapter(toolsReg)
			agent := runtime.NewAgent(j.AgentID, j.AgentID, sess, nil, plannerProv, toolsProv)
			err := dagRunner.RunForJob(ctx, agent, &agentexec.JobForRunner{
				ID: j.ID, AgentID: j.AgentID, Goal: j.Goal, Cursor: j.Cursor,
			})
			_, ver, _ := pgEventStore.ListEvents(ctx, j.ID)
			evType := jobstore.JobCompleted
			if err != nil {
				evType = jobstore.JobFailed
			}
			errStr := ""
			if err != nil {
				errStr = err.Error()
			}
			payload, _ := json.Marshal(map[string]interface{}{"goal": j.Goal, "error": errStr})
			_, _ = pgEventStore.Append(ctx, j.ID, ver, jobstore.JobEvent{JobID: j.ID, Type: evType, Payload: payload})
			if err != nil {
				_ = pgJobStore.UpdateStatus(ctx, j.ID, job.StatusFailed)
			} else {
				_ = pgJobStore.UpdateStatus(ctx, j.ID, job.StatusCompleted)
			}
			return err
		}
		pollInterval := 2 * time.Second
		if cfg.Worker.PollInterval != "" {
			if d, err := time.ParseDuration(cfg.Worker.PollInterval); err == nil && d > 0 {
				pollInterval = d
			}
		}
		appObj.agentJobRunner = NewAgentJobRunner(
			DefaultWorkerID(),
			pgEventStore,
			pgJobStore,
			runJob,
			pollInterval,
			leaseDur,
			logger,
		)
		logger.Info("Worker Agent Job 模式已启用", "worker_id", DefaultWorkerID(), "dsn", dsn)
	}

	return appObj, nil
}

// Start 启动应用（Pipeline 由 eino 调度；JobStore=postgres 时启动 Agent Job Claim 循环）
func (a *App) Start() error {
	a.logger.Info("启动 worker 应用")

	if a.agentJobRunner != nil {
		ctx, cancel := context.WithCancel(context.Background())
		a.agentJobCancel = cancel
		a.agentJobRunner.Start(ctx)
	}

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

	if a.agentJobCancel != nil {
		a.agentJobCancel()
	}
	if a.agentJobRunner != nil {
		a.agentJobRunner.Stop()
	}

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

