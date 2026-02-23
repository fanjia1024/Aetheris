// Copyright 2026 fanjia1024
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"
	"google.golang.org/grpc"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	hertzslog "github.com/hertz-contrib/logger/slog"
	"github.com/hertz-contrib/obs-opentelemetry/provider"
	hertztracing "github.com/hertz-contrib/obs-opentelemetry/tracing"
	"github.com/jackc/pgx/v5/pgxpool"
	apigrpc "rag-platform/internal/api/grpc"

	"rag-platform/internal/agent"
	"rag-platform/internal/agent/executor"
	"rag-platform/internal/agent/instance"
	"rag-platform/internal/agent/job"
	"rag-platform/internal/agent/messaging"
	"rag-platform/internal/agent/planner"
	replaysandbox "rag-platform/internal/agent/replay/sandbox"
	"rag-platform/internal/agent/runtime"
	agentexec "rag-platform/internal/agent/runtime/executor"
	"rag-platform/internal/agent/runtime/executor/verifier"
	"rag-platform/internal/agent/tools"
	"rag-platform/internal/api/http"
	"rag-platform/internal/api/http/middleware"
	"rag-platform/internal/app"
	"rag-platform/internal/einoext"
	"rag-platform/internal/ingestqueue"
	"rag-platform/internal/model/llm"
	"rag-platform/internal/pipeline/ingest"
	"rag-platform/internal/pipeline/query"
	"rag-platform/internal/runtime/eino"
	"rag-platform/internal/runtime/jobstore"
	"rag-platform/internal/runtime/session"
	"rag-platform/internal/splitter"
	"rag-platform/internal/storage/vector"
	"rag-platform/pkg/auth"
)

// otelProviderShutdown 用于优雅关闭时关闭 OpenTelemetry provider
type otelProviderShutdown interface {
	Shutdown(ctx context.Context) error
}

// App API 应用（装配 HTTP Router、Handler、Middleware；仅依赖 Engine + DocumentService）
type App struct {
	config       *app.Bootstrap
	engine       *eino.Engine
	docService   app.DocumentService
	router       *http.Router
	hertz        *server.Hertz
	grpcServer   *grpcRun
	otelProvider otelProviderShutdown
	jobScheduler *job.Scheduler
}

// jobStoreForRunnerAdapter 将 job.JobStore 适配为 agentexec.JobStoreForRunner（status int）
type jobStoreForRunnerAdapter struct {
	job.JobStore
}

var _ agentexec.JobStoreForRunner = (*jobStoreForRunnerAdapter)(nil)

func (a *jobStoreForRunnerAdapter) UpdateCursor(ctx context.Context, jobID string, cursor string) error {
	return a.JobStore.UpdateCursor(ctx, jobID, cursor)
}

func (a *jobStoreForRunnerAdapter) UpdateStatus(ctx context.Context, jobID string, status int) error {
	return a.JobStore.UpdateStatus(ctx, jobID, job.JobStatus(status))
}

// grpcRun 持有 gRPC Server 与 Listener，用于 GracefulStop 时关闭
type grpcRun struct {
	srv *grpc.Server
	lis net.Listener
}

func (g *grpcRun) GracefulStop() {
	if g.lis != nil {
		_ = g.lis.Close()
	}
	if g.srv != nil {
		g.srv.GracefulStop()
	}
}

// NewApp 创建 API 应用（由 cmd/api 调用）
func NewApp(bootstrap *app.Bootstrap) (*App, error) {
	engine, err := eino.NewEngine(bootstrap.Config, bootstrap.Logger)
	if err != nil {
		return nil, fmt.Errorf("初始化 eino 引擎失败: %w", err)
	}

	var llmClientForAgent llm.Client
	var generatorForAgent eino.Generator

	// 默认向量集合名（与 storage.vector.collection 一致，空则 "default"）
	defaultCollection := "default"
	if bootstrap.Config != nil && bootstrap.Config.Storage.Vector.Collection != "" {
		defaultCollection = bootstrap.Config.Storage.Vector.Collection
	}

	// 装配并注册 query_pipeline（Retriever + Generator + queryEmbedder）；memory 或 redis 等由 einoext 工厂创建
	vecCfg := bootstrap.Config.Storage.Vector
	queryPipelineEnabled := bootstrap.Config != nil && (bootstrap.VectorStore != nil || (vecCfg.Type != "" && vecCfg.Type != "memory"))
	if queryPipelineEnabled {
		llmClient, errLLM := app.NewLLMClientFromConfig(bootstrap.Config)
		queryEmbedder, errEmb := app.NewQueryEmbedderFromConfig(bootstrap.Config)
		if errLLM == nil && errEmb == nil && llmClient != nil && queryEmbedder != nil {
			generator := query.NewGenerator(llmClient, 4096, 0.1)
			einoEmbedder := NewEinoEmbedderAdapter(queryEmbedder)
			einoRetriever, errRet := einoext.NewRetriever(context.Background(), vecCfg, bootstrap.VectorStore, einoEmbedder)
			if errRet != nil {
				bootstrap.Logger.Info("einoext NewRetriever 失败，回退 memory", "error", errRet)
				if bootstrap.VectorStore != nil {
					einoRetriever, errRet = query.NewMemoryRetriever(&query.MemoryRetrieverConfig{
						VectorStore: bootstrap.VectorStore, DefaultIndex: defaultCollection, DefaultTopK: 10, DefaultThreshold: 0.3,
					})
					if errRet == nil {
						retrieverForWorkflow := query.NewRetriever(bootstrap.VectorStore, defaultCollection, 10, 0.3)
						qwf := eino.NewQueryWorkflowExecutor(retrieverForWorkflow, generator, queryEmbedder, bootstrap.Logger)
						_ = engine.RegisterWorkflow("query_pipeline", qwf)
						retrieverAdapter := NewRetrieverAdapter(queryEmbedder, einoRetriever, 0.3)
						ragGen := NewRAGGeneratorAdapter(retrieverAdapter, generator, queryEmbedder, defaultCollection)
						engine.SetQueryComponents(retrieverAdapter, ragGen)
						generatorForAgent = ragGen
					}
				}
			} else {
				retrieverForWorkflow := &EinoRetrieverQueryAdapter{EinoRetriever: einoRetriever, Embedder: queryEmbedder, TopK: 10}
				qwf := eino.NewQueryWorkflowExecutor(retrieverForWorkflow, generator, queryEmbedder, bootstrap.Logger)
				if err := engine.RegisterWorkflow("query_pipeline", qwf); err != nil {
					bootstrap.Logger.Info("注册 query_pipeline 失败，将使用占位实现", "error", err)
				}
				retrieverAdapter := NewRetrieverAdapter(queryEmbedder, einoRetriever, 0.3)
				ragGen := NewRAGGeneratorAdapter(retrieverAdapter, generator, queryEmbedder, defaultCollection)
				engine.SetQueryComponents(retrieverAdapter, ragGen)
				generatorForAgent = ragGen
			}
			llmClientForAgent = llmClient
		}
	}

	// LLM 限流：从配置加载 LLMRateLimiter 并包装 llmClientForAgent（防止打爆 Provider API）
	if llmClientForAgent != nil && bootstrap.Config != nil && len(bootstrap.Config.RateLimits.LLM) > 0 {
		llmLimiterConfigs := make(map[string]llm.LLMLimitConfig, len(bootstrap.Config.RateLimits.LLM))
		for provider, c := range bootstrap.Config.RateLimits.LLM {
			if provider == "_default" {
				continue
			}
			llmLimiterConfigs[provider] = llm.LLMLimitConfig{
				TokensPerMinute:   c.TokensPerMinute,
				RequestsPerMinute: c.RequestsPerMinute,
				MaxConcurrent:     c.MaxConcurrent,
			}
		}
		var llmDefaults *llm.LLMLimitConfig
		if d, ok := bootstrap.Config.RateLimits.LLM["_default"]; ok {
			llmDefaults = &llm.LLMLimitConfig{
				TokensPerMinute:   d.TokensPerMinute,
				RequestsPerMinute: d.RequestsPerMinute,
				MaxConcurrent:     d.MaxConcurrent,
			}
		}
		llmRateLimiter := llm.NewLLMRateLimiter(llmLimiterConfigs, llmDefaults)
		llmClientForAgent = llm.NewRateLimitedClient(llmClientForAgent, llmRateLimiter)
		bootstrap.Logger.Info("LLM 限流已启用", "providers", len(llmLimiterConfigs))
	}

	// 装配并注册 ingest_pipeline（loader → parser → splitter → embedding → indexer）；Indexer 由 einoext 工厂创建
	ingestPipelineEnabled := bootstrap.Config != nil && bootstrap.MetadataStore != nil && (bootstrap.VectorStore != nil || (vecCfg.Type != "" && vecCfg.Type != "memory"))
	if ingestPipelineEnabled {
		ingestEmbedder, errEmb := app.NewQueryEmbedderFromConfig(bootstrap.Config)
		if errEmb == nil && ingestEmbedder != nil {
			ingestConcurrency := 4
			ingestBatchSize := 100
			if bootstrap.Config.Storage.Ingest.Concurrency > 0 {
				ingestConcurrency = bootstrap.Config.Storage.Ingest.Concurrency
			}
			if bootstrap.Config.Storage.Ingest.BatchSize > 0 {
				ingestBatchSize = bootstrap.Config.Storage.Ingest.BatchSize
			}
			vectorStoreType := vecCfg.Type
			if vectorStoreType == "" {
				vectorStoreType = "memory"
			}
			docEmbedding := ingest.NewDocumentEmbedding(ingestEmbedder, ingestConcurrency)
			einoEmbedder := NewEinoEmbedderAdapter(ingestEmbedder)
			einoIndexer, errIdx := einoext.NewIndexer(context.Background(), vecCfg, bootstrap.VectorStore, einoEmbedder)
			var docIndexer *ingest.DocumentIndexer
			if errIdx != nil {
				bootstrap.Logger.Info("einoext NewIndexer 失败，回退 memory", "error", errIdx)
				if bootstrap.VectorStore != nil {
					if memoryIndexer, errM := ingest.NewMemoryIndexer(&ingest.MemoryIndexerConfig{
						VectorStore: bootstrap.VectorStore, DefaultCollection: defaultCollection, BatchSize: ingestBatchSize,
					}); errM == nil {
						docIndexer = ingest.NewDocumentIndexerFromEino(memoryIndexer, bootstrap.MetadataStore, defaultCollection, vectorStoreType)
					} else {
						docIndexer = ingest.NewDocumentIndexer(bootstrap.VectorStore, bootstrap.MetadataStore, ingestConcurrency, ingestBatchSize, defaultCollection, vectorStoreType)
					}
				}
			} else {
				docIndexer = ingest.NewDocumentIndexerFromEino(einoIndexer, bootstrap.MetadataStore, defaultCollection, vectorStoreType)
			}
			if docIndexer != nil {
				if bootstrap.VectorStore != nil {
					if err := vector.EnsureIndex(context.Background(), bootstrap.VectorStore, defaultCollection, ingestEmbedder.Dimension(), "cosine"); err != nil {
						bootstrap.Logger.Info("创建向量索引失败（首次写入时可能再创建）", "collection", defaultCollection, "error", err)
					}
				}
				loader := ingest.NewDocumentLoader()
				parser := ingest.NewDocumentParser()
				docSplitter := ingest.NewDocumentSplitter(1000, 100, 1000)
				splitterEngine := splitter.NewEngine(ingestEmbedder)
				docSplitter.SetEngine(splitterEngine, "structural")
				iwf := eino.NewIngestWorkflowExecutor(loader, parser, docSplitter, docEmbedding, docIndexer, bootstrap.Logger)
				if err := engine.RegisterWorkflow("ingest_pipeline", iwf); err != nil {
					bootstrap.Logger.Info("注册 ingest_pipeline 失败，将使用占位实现", "error", err)
				}
				engine.SetIngestComponents(
					NewLoaderAdapter(loader),
					NewParserAdapter(parser),
					NewSplitterAdapter(docSplitter),
					NewEmbeddingAdapter(docEmbedding),
					NewIndexerAdapter(docIndexer),
				)
				engine.SetEinoDocumentComponents(
					ingest.NewURIDocumentLoader(loader),
					ingest.NewSplitterTransformer(parser, docSplitter),
				)
			}
		}
	}

	// Agent Runtime：agent/tools.Registry（Session 感知）+ Builtin + Planner + Executor + Memory + Agent
	toolsReg := tools.NewRegistry()
	tools.RegisterBuiltin(toolsReg, engine, generatorForAgent)
	plannerAgent := planner.NewLLMPlanner(llmClientForAgent)
	execAgent := executor.NewSessionRegistryExecutor(toolsReg)
	agentRunner := agent.New(plannerAgent, execAgent, toolsReg)
	sessionStore := session.NewMemoryStore()
	sessionManager := session.NewManager(sessionStore)
	docService := app.NewDocumentService(bootstrap.MetadataStore)
	handler := http.NewHandler(engine, docService)
	handler.SetAgent(agentRunner)
	handler.SetSessionManager(sessionManager)
	// 主 ADK Runner：当启用时 /api/agent/run、resume、stream 使用 ADK 执行
	if engine != nil {
		adkEnabled := true
		if bootstrap.Config != nil && bootstrap.Config.Agent.ADK.Enabled != nil {
			adkEnabled = *bootstrap.Config.Agent.ADK.Enabled
		}
		if adkEnabled {
			cps := NewMemoryCheckPointStore()
			adkRunner, errADK := NewMainADKRunner(context.Background(), engine, cps)
			if errADK == nil && adkRunner != nil {
				handler.SetADKRunner(adkRunner)
			} else if errADK != nil {
				bootstrap.Logger.Info("创建主 ADK Runner 失败，将使用原 Agent", "error", errADK)
			}
		}
	}

	// v1 Agent Runtime：Manager + Scheduler + Creator（POST /api/agents 等）
	agentRuntimeManager := runtime.NewManager()
	// Planner 选择：PLANNER_TYPE=rule 时使用规则规划器（无 LLM，便于调试 Executor），否则使用 LLM
	var v1Planner planGoaler
	if os.Getenv("PLANNER_TYPE") == "rule" {
		v1Planner = planner.NewRulePlanner()
		bootstrap.Logger.Info("v1 Agent 使用规则规划器（RulePlanner）")
	} else {
		llmPlanner := planner.NewLLMPlanner(llmClientForAgent)
		if schema, err := toolsReg.SchemasForLLM(); err == nil && len(schema) > 0 {
			llmPlanner.SetToolsSchemaForGoal(schema)
		}
		v1Planner = llmPlanner
	}
	var dagCompiler *agentexec.Compiler
	var dagRunner *agentexec.Runner
	agentScheduler := runtime.NewScheduler(agentRuntimeManager, func(ctx context.Context, agentID string) {
		if dagRunner != nil {
			RunFuncForScheduler(agentRuntimeManager, dagRunner)(ctx, agentID)
		}
	})
	agentCreator := NewAgentCreator(agentRuntimeManager, v1Planner, toolsReg)
	handler.SetAgentRuntime(agentRuntimeManager, agentScheduler, agentCreator)
	// v0.8 Job System：message -> create Job -> Scheduler（并发/重试）-> Worker -> Executor；Checkpoint 支持恢复
	// Job 元数据存储：postgres 时与 Worker 共享 jobs 表，否则内存（仅 API 进程内）
	var jobStore job.JobStore
	var jobEventStore jobstore.JobStore
	if bootstrap.Config != nil && bootstrap.Config.JobStore.Type == "postgres" && bootstrap.Config.JobStore.DSN != "" {
		dsn := bootstrap.Config.JobStore.DSN
		leaseDur := 30 * time.Second
		if bootstrap.Config.JobStore.LeaseDuration != "" {
			if d, err := time.ParseDuration(bootstrap.Config.JobStore.LeaseDuration); err == nil && d > 0 {
				leaseDur = d
			}
		}
		pgEventStore, err := jobstore.NewPostgresStore(context.Background(), dsn, leaseDur)
		if err != nil {
			return nil, fmt.Errorf("初始化 JobStore 事件(postgres) 失败: %w", err)
		}
		jobEventStore = pgEventStore
		pgJobStore, err := job.NewJobStorePg(context.Background(), dsn)
		if err != nil {
			return nil, fmt.Errorf("初始化 Job 元数据(postgres) 失败: %w", err)
		}
		jobStore = pgJobStore
		bootstrap.Logger.Info("JobStore 使用 PostgreSQL 后端", "dsn", dsn)
	} else {
		jobStore = job.NewJobStoreMem()
		jobEventStore = jobstore.NewMemoryStore()
	}
	var invocationStore agentexec.ToolInvocationStore
	if bootstrap.Config != nil && bootstrap.Config.JobStore.Type == "postgres" && bootstrap.Config.JobStore.DSN != "" {
		invPoolConfig, errPool := pgxpool.ParseConfig(bootstrap.Config.JobStore.DSN)
		if errPool != nil {
			return nil, fmt.Errorf("解析 ToolInvocationStore DSN 失败: %w", errPool)
		}
		invPool, errPool := pgxpool.NewWithConfig(context.Background(), invPoolConfig)
		if errPool != nil {
			return nil, fmt.Errorf("创建 ToolInvocationStore 连接池失败: %w", errPool)
		}
		invocationStore = agentexec.NewToolInvocationStorePg(invPool)
	} else {
		invocationStore = agentexec.NewToolInvocationStoreMem()
	}
	nodeEventSink := NewNodeEventSink(jobEventStore)
	var resourceVerifier agentexec.ResourceVerifier
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		resourceVerifier = verifier.NewGitHubVerifier(token)
	}
	// Tool 限流器（可选）
	var toolRateLimiter *agentexec.ToolRateLimiter
	if bootstrap.Config != nil && len(bootstrap.Config.RateLimits.Tools) > 0 {
		toolLimiterConfigs := make(map[string]agentexec.ToolLimitConfig, len(bootstrap.Config.RateLimits.Tools))
		for toolName, c := range bootstrap.Config.RateLimits.Tools {
			if toolName == "_default" {
				continue
			}
			toolLimiterConfigs[toolName] = agentexec.ToolLimitConfig{
				QPS:           c.QPS,
				MaxConcurrent: c.MaxConcurrent,
				Burst:         c.Burst,
			}
		}
		var toolDefaults *agentexec.ToolLimitConfig
		if d, ok := bootstrap.Config.RateLimits.Tools["_default"]; ok {
			toolDefaults = &agentexec.ToolLimitConfig{QPS: d.QPS, MaxConcurrent: d.MaxConcurrent, Burst: d.Burst}
		}
		toolRateLimiter = agentexec.NewToolRateLimiter(toolLimiterConfigs, toolDefaults)
		bootstrap.Logger.Info("Tool 限流已启用", "tools", len(toolLimiterConfigs))
	}
	dagCompiler = NewDAGCompilerWithOptions(llmClientForAgent, toolsReg, engine, nodeEventSink, nodeEventSink, invocationStore, nil, resourceVerifier, NewAttemptValidator(jobEventStore), toolRateLimiter)
	dagRunner = NewDAGRunner(dagCompiler)
	var agentStateStore runtime.AgentStateStore
	if bootstrap.Config != nil && bootstrap.Config.JobStore.Type == "postgres" && bootstrap.Config.JobStore.DSN != "" {
		pgState, errState := runtime.NewAgentStateStorePg(context.Background(), bootstrap.Config.JobStore.DSN)
		if errState != nil {
			return nil, fmt.Errorf("初始化 AgentStateStore(postgres) 失败: %w", errState)
		}
		agentStateStore = pgState
	} else {
		agentStateStore = runtime.NewAgentStateStoreMem()
	}
	var agentInstanceStore instance.AgentInstanceStore
	if bootstrap.Config != nil && bootstrap.Config.JobStore.Type == "postgres" && bootstrap.Config.JobStore.DSN != "" {
		pgInst, errInst := instance.NewStorePg(context.Background(), bootstrap.Config.JobStore.DSN)
		if errInst != nil {
			return nil, fmt.Errorf("初始化 AgentInstanceStore(postgres) 失败: %w", errInst)
		}
		agentInstanceStore = pgInst
	} else {
		agentInstanceStore = instance.NewStoreMem()
	}
	handler.SetAgentInstanceStore(agentInstanceStore)
	var agentMessagingBus messaging.AgentMessagingBus
	if bootstrap.Config != nil && bootstrap.Config.JobStore.Type == "postgres" && bootstrap.Config.JobStore.DSN != "" {
		pgBus, errBus := messaging.NewStorePg(context.Background(), bootstrap.Config.JobStore.DSN)
		if errBus != nil {
			return nil, fmt.Errorf("初始化 AgentMessagingBus(postgres) 失败: %w", errBus)
		}
		agentMessagingBus = pgBus
	} else {
		agentMessagingBus = messaging.NewStoreMem()
	}
	handler.SetAgentMessagingBus(agentMessagingBus)
	if bootstrap.Config != nil && bootstrap.Config.JobStore.Type == "postgres" && bootstrap.Config.JobStore.DSN != "" {
		ingestPoolConfig, errIngest := pgxpool.ParseConfig(bootstrap.Config.JobStore.DSN)
		if errIngest == nil {
			if ingestPool, errIngest := pgxpool.NewWithConfig(context.Background(), ingestPoolConfig); errIngest == nil {
				handler.SetIngestQueue(ingestqueue.NewIngestQueuePg(ingestPool))
			}
		}
	}
	checkpointStore := runtime.NewCheckpointStoreMem()
	dagRunner.SetCheckpointStores(checkpointStore, &jobStoreForRunnerAdapter{JobStore: jobStore})
	dagRunner.SetPlanGeneratedSink(NewPlanGeneratedSink(jobEventStore))
	dagRunner.SetNodeEventSink(nodeEventSink)
	dagRunner.SetRecordedEffectsRecorder(NewRecordedEffectsRecorder(jobEventStore))
	dagRunner.SetReplayContextBuilder(NewReplayContextBuilder(jobEventStore))
	dagRunner.SetReplayPolicy(replaysandbox.DefaultPolicy{})
	if bootstrap.Config != nil && bootstrap.Config.Worker.Timeout != "" {
		if d, err := time.ParseDuration(bootstrap.Config.Worker.Timeout); err == nil && d > 0 {
			dagRunner.SetStepTimeout(d)
		}
	}
	waitPlanReady := func(ctx context.Context, jobID string, maxWait time.Duration) error {
		if maxWait <= 0 {
			maxWait = 15 * time.Second
		}
		deadline := time.Now().Add(maxWait)
		for {
			events, _, err := jobEventStore.ListEvents(ctx, jobID)
			if err == nil {
				for _, e := range events {
					if e.Type == jobstore.PlanGenerated {
						return nil
					}
				}
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("plan_generated not ready within %s", maxWait)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(200 * time.Millisecond):
			}
		}
	}
	runJob := func(ctx context.Context, j *job.Job) error {
		agent, _ := agentRuntimeManager.Get(ctx, j.AgentID)
		if agent == nil {
			return fmt.Errorf("agent not found: %s", j.AgentID)
		}
		tenantID := j.TenantID
		if tenantID == "" {
			tenantID = "default"
		}
		if err := waitPlanReady(ctx, j.ID, 20*time.Second); err != nil {
			return err
		}
		err := dagRunner.RunForJob(ctx, agent, &agentexec.JobForRunner{
			ID: j.ID, AgentID: j.AgentID, Goal: j.Goal, Cursor: j.Cursor, TenantID: tenantID,
		})
		if agentStateStore != nil && agent.Session != nil {
			_ = agentStateStore.SaveAgentState(ctx, j.AgentID, agent.Session.ID, runtime.SessionToAgentState(agent.Session))
		}
		// 事件流补全：执行结束后追加 JobCompleted / JobFailed，便于审计与回放
		if jobEventStore != nil {
			_, ver, _ := jobEventStore.ListEvents(ctx, j.ID)
			evType := jobstore.JobCompleted
			pl := map[string]interface{}{"goal": j.Goal}
			if err != nil {
				evType = jobstore.JobFailed
				pl["error"] = err.Error()
				var sf *agentexec.StepFailure
				if errors.As(err, &sf) {
					pl["result_type"] = string(sf.Type)
					pl["node_id"] = sf.FailedNodeID()
					pl["reason"] = err.Error()
				}
			}
			payload, _ := json.Marshal(pl)
			_, _ = jobEventStore.Append(ctx, j.ID, ver, jobstore.JobEvent{JobID: j.ID, Type: evType, Payload: payload})
		}
		return err
	}
	schedulerConfig := job.SchedulerConfig{
		MaxConcurrency: 2,
		RetryMax:       2,
		Backoff:        time.Second,
	}
	if bootstrap.Config != nil {
		sc := bootstrap.Config.Agent.JobScheduler
		if sc.MaxConcurrency > 0 {
			schedulerConfig.MaxConcurrency = sc.MaxConcurrency
		}
		if sc.RetryMax >= 0 {
			schedulerConfig.RetryMax = sc.RetryMax
		}
		if sc.Backoff != "" {
			schedulerConfig.Backoff = parseDuration(sc.Backoff, time.Second)
		}
		if len(sc.Queues) > 0 {
			schedulerConfig.Queues = sc.Queues
		}
	}
	jobScheduler := job.NewScheduler(jobStore, runJob, schedulerConfig)
	handler.SetJobStore(jobStore)
	if pgStore, ok := jobStore.(*job.JobStorePg); ok {
		handler.SetObservabilityReader(pgStore)
	}
	handler.SetJobEventStore(jobEventStore)
	handler.SetAgentStateStore(agentStateStore)
	handler.SetToolsRegistry(toolsReg)
	// 1.0 Plan 事件化：Job 创建时即生成并持久化 TaskGraph，执行阶段只读
	if jobEventStore != nil {
		handler.SetPlanAtJobCreation(PlanGoalForJobFunc(agentRuntimeManager, v1Planner))
	}

	mw := middleware.NewMiddleware()
	router := http.NewRouter(handler, mw)
	if bootstrap.Config != nil {
		router.SetForensicsExperimental(bootstrap.Config.API.Forensics.Experimental)
	}

	if bootstrap.Config != nil && bootstrap.Config.API.Middleware.Auth && bootstrap.Config.API.Middleware.JWTKey != "" {
		timeout := parseDuration(bootstrap.Config.API.Middleware.JWTTimeout, time.Hour)
		maxRefresh := parseDuration(bootstrap.Config.API.Middleware.JWTMaxRefresh, time.Hour)
		jwtAuth, err := middleware.NewJWTAuth([]byte(bootstrap.Config.API.Middleware.JWTKey), timeout, maxRefresh)
		if err != nil {
			bootstrap.Logger.Warn("JWT 初始化失败，将跳过认证", "error", err)
		} else {
			router.SetJWT(jwtAuth)
			bootstrap.Logger.Info("JWT 认证已启用")
		}
	}

	// RBAC：RoleStore + AuthZ 中间件（与 JWT 配合使用 tenant_id/user_id）
	roleStore := auth.NewMemoryRoleStore()
	// 预置开发账号角色，避免启用 JWT+RBAC 后本地环境无法创建 Agent。
	_ = roleStore.SetUserRole(context.Background(), "default", "admin", auth.RoleAdmin)
	_ = roleStore.SetUserRole(context.Background(), "default", "test", auth.RoleUser)
	// auth 未开启时 user_id 兜底为 "anonymous"，预置 Admin 角色以保持 API 可用（生产环境请开启 JWT）
	authEnabled := bootstrap.Config != nil && bootstrap.Config.API.Middleware.Auth
	if !authEnabled {
		_ = roleStore.SetUserRole(context.Background(), "default", "anonymous", auth.RoleAdmin)
	}
	rbacChecker := auth.NewSimpleRBACChecker(roleStore)
	router.SetAuthZ(middleware.NewAuthZMiddleware(rbacChecker))

	appObj := &App{
		config:       bootstrap,
		engine:       engine,
		docService:   docService,
		router:       router,
		hertz:        nil,
		jobScheduler: jobScheduler,
	}
	if bootstrap.Config != nil && bootstrap.Config.API.Grpc.Enable && bootstrap.Config.API.Grpc.Port > 0 {
		gs, err := startGRPC(engine, docService, bootstrap.Config.API.Grpc.Port)
		if err != nil {
			bootstrap.Logger.Warn("gRPC 服务启动失败", "error", err)
		} else {
			appObj.grpcServer = gs
			bootstrap.Logger.Info("gRPC 服务已启动", "port", bootstrap.Config.API.Grpc.Port)
		}
	}
	return appObj, nil
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
	// 单一执行权 / Control vs Data Plane：jobstore.type=postgres 时 API 不启动 Scheduler，不执行任何 Job（API = 控制面；Worker = 数据面，仅由 Worker 通过事件 Claim 执行）
	jobSchedulerEnabled := false
	if a.config.Config != nil && a.config.Config.JobStore.Type != "postgres" {
		jobSchedulerEnabled = true
		if a.config.Config.Agent.JobScheduler.Enabled != nil {
			jobSchedulerEnabled = *a.config.Config.Agent.JobScheduler.Enabled
		}
	}
	if a.jobScheduler != nil && jobSchedulerEnabled {
		go a.jobScheduler.Start(context.Background())
	}
	return a.hertz.Run()
}

// Shutdown 优雅关闭（传入 ctx 以支持超时，如 cmd 层 WithTimeout）
func (a *App) Shutdown(ctx context.Context) error {
	if a.jobScheduler != nil {
		a.jobScheduler.Stop()
	}
	if a.otelProvider != nil {
		_ = a.otelProvider.Shutdown(ctx)
	}
	if a.grpcServer != nil {
		a.grpcServer.GracefulStop()
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

// parseDuration 解析时长字符串，无效或空时返回 defaultVal
func parseDuration(s string, defaultVal time.Duration) time.Duration {
	if s == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return defaultVal
	}
	return d
}

// startGRPC 创建并启动 gRPC 服务（在 goroutine 中 Serve），返回 grpcRun 以便 Shutdown 时 GracefulStop
func startGRPC(engine *eino.Engine, docService app.DocumentService, port int) (*grpcRun, error) {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	srv := grpc.NewServer()
	apigrpc.NewServer(engine, docService).Register(srv)
	go func() {
		_ = srv.Serve(lis)
	}()
	return &grpcRun{srv: srv, lis: lis}, nil
}
