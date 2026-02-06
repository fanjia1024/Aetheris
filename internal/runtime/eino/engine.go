package eino

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"rag-platform/pkg/config"
	"rag-platform/pkg/log"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
)

// Engine eino 引擎实例
type Engine struct {
	runners   map[string]*adk.Runner
	workflows  map[string]WorkflowExecutor
	config     *config.Config
	logger     *log.Logger
	mu         sync.RWMutex
	workflowsMu sync.RWMutex
}

// NewEngine 创建新的 eino 引擎实例
func NewEngine(cfg *config.Config, logger *log.Logger) (*Engine, error) {
	// 创建引擎实例
	engine := &Engine{
		runners:   make(map[string]*adk.Runner),
		workflows:  make(map[string]WorkflowExecutor),
		config:     cfg,
		logger:     logger,
	}

	// 启动引擎
	if err := engine.start(); err != nil {
		return nil, fmt.Errorf("启动 eino 引擎失败: %w", err)
	}

	logger.Info("eino 引擎初始化成功")
	return engine, nil
}

// start 启动 eino 引擎
func (e *Engine) start() error {
	// 注册默认 Agent
	if err := e.registerDefaultAgents(); err != nil {
		return err
	}

	// 注册默认 Workflow（由 eino 统一调度）
	e.registerDefaultWorkflows()

	return nil
}

// registerDefaultAgents 注册默认 Agent
func (e *Engine) registerDefaultAgents() error {
	ctx := context.Background()

	// 注册 QA Agent
	qaRunner, err := e.createQARunner(ctx)
	if err != nil {
		return fmt.Errorf("创建 QA Agent 失败: %w", err)
	}

	// 注册 Ingest Agent
	ingestRunner, err := e.createIngestRunner(ctx)
	if err != nil {
		return fmt.Errorf("创建 Ingest Agent 失败: %w", err)
	}

	// 注册 Runner 到引擎
	e.mu.Lock()
	e.runners["qa_agent"] = qaRunner
	e.runners["ingest_agent"] = ingestRunner
	e.mu.Unlock()

	e.logger.Info("默认 Agent 注册成功", "agents", []string{"qa_agent", "ingest_agent"})
	return nil
}

// createQARunner 创建 QA Agent Runner
func (e *Engine) createQARunner(ctx context.Context) (*adk.Runner, error) {
	tools := []tool.BaseTool{
		CreateRetrieverTool(),
		CreateGeneratorTool(),
	}

	cfg := &adk.ChatModelAgentConfig{
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: tools,
			},
		},
	}
	if chatModel, err := e.createChatModel(ctx); err == nil && chatModel != nil {
		cfg.Model = chatModel
	}

	agent, err := adk.NewChatModelAgent(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return adk.NewRunner(ctx, adk.RunnerConfig{
		Agent: agent,
	}), nil
}

// createIngestRunner 创建 Ingest Agent Runner
func (e *Engine) createIngestRunner(ctx context.Context) (*adk.Runner, error) {
	tools := []tool.BaseTool{
		CreateDocumentLoaderTool(),
		CreateDocumentParserTool(),
		CreateSplitterTool(),
		CreateEmbeddingTool(),
		CreateIndexBuilderTool(),
	}

	cfg := &adk.ChatModelAgentConfig{
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: tools,
			},
		},
	}
	if chatModel, err := e.createChatModel(ctx); err == nil && chatModel != nil {
		cfg.Model = chatModel
	}

	agent, err := adk.NewChatModelAgent(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return adk.NewRunner(ctx, adk.RunnerConfig{
		Agent: agent,
	}), nil
}

// createChatModel 创建 OpenAI ChatModel（根据 config.Model.Defaults.LLM 解析 provider.model_key）
func (e *Engine) createChatModel(ctx context.Context) (*openai.ChatModel, error) {
	if e.config == nil || e.config.Model.Defaults.LLM == "" {
		return nil, nil
	}
	provider, modelKey, err := parseDefaultKey(e.config.Model.Defaults.LLM)
	if err != nil {
		return nil, err
	}
	pc, ok := e.config.Model.LLM.Providers[provider]
	if !ok {
		return nil, fmt.Errorf("LLM provider %q 未配置", provider)
	}
	mi, ok := pc.Models[modelKey]
	if !ok {
		return nil, fmt.Errorf("LLM model %q 未在 provider %q 中配置", modelKey, provider)
	}
	if pc.APIKey == "" {
		return nil, fmt.Errorf("LLM provider %q 的 api_key 未配置", provider)
	}

	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		Model:  mi.Name,
		APIKey: pc.APIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 OpenAI ChatModel 失败: %w", err)
	}
	return chatModel, nil
}

func parseDefaultKey(key string) (provider, modelKey string, err error) {
	parts := strings.SplitN(key, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("default key 格式应为 provider.model_key，如 openai.gpt_35_turbo，当前: %q", key)
	}
	return parts[0], parts[1], nil
}

// GetRunner 获取 Runner 实例
func (e *Engine) GetRunner(agentID string) (*adk.Runner, error) {
	e.mu.RLock()
	runner, exists := e.runners[agentID]
	e.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("Runner %s 不存在", agentID)
	}

	return runner, nil
}

// RegisterRunner 注册自定义 Runner
func (e *Engine) RegisterRunner(name string, runner *adk.Runner) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.runners[name]; exists {
		return fmt.Errorf("Runner %s 已存在", name)
	}

	e.runners[name] = runner
	e.logger.Info("Runner 注册成功", "name", name)
	return nil
}

// Execute 执行任务
func (e *Engine) Execute(ctx context.Context, agentID string, query string) (chan *adk.AgentEvent, error) {
	runner, err := e.GetRunner(agentID)
	if err != nil {
		return nil, err
	}

	iter := runner.Query(ctx, query)
	eventCh := make(chan *adk.AgentEvent)

	go func() {
		defer close(eventCh)
		for {
			event, ok := iter.Next()
			if !ok {
				break
			}
			eventCh <- event
		}
	}()

	return eventCh, nil
}

// Shutdown 关闭 eino 引擎
func (e *Engine) Shutdown() error {
	e.logger.Info("eino 引擎关闭成功")
	return nil
}

// GetAgents 获取所有 Agent
func (e *Engine) GetAgents() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	agentNames := make([]string, 0, len(e.runners))
	for name := range e.runners {
		agentNames = append(agentNames, name)
	}

	return agentNames
}

// registerDefaultWorkflows 注册默认工作流（ingest_pipeline / query_pipeline）
func (e *Engine) registerDefaultWorkflows() {
	e.workflowsMu.Lock()
	defer e.workflowsMu.Unlock()

	e.workflows["ingest_pipeline"] = &ingestWorkflowExecutor{logger: e.logger}
	e.workflows["query_pipeline"] = &queryWorkflowExecutor{logger: e.logger}
	e.logger.Info("默认 Workflow 注册成功", "workflows", []string{"ingest_pipeline", "query_pipeline"})
}

// RegisterWorkflow 注册工作流（可由 app 层注入真实实现，覆盖默认占位）
func (e *Engine) RegisterWorkflow(name string, wf WorkflowExecutor) error {
	e.workflowsMu.Lock()
	defer e.workflowsMu.Unlock()

	if name == "" {
		return fmt.Errorf("workflow name 不能为空")
	}
	e.workflows[name] = wf
	e.logger.Info("Workflow 注册成功", "name", name)
	return nil
}

// ExecuteWorkflow 执行已注册的工作流
func (e *Engine) ExecuteWorkflow(ctx context.Context, name string, params map[string]interface{}) (interface{}, error) {
	e.workflowsMu.RLock()
	wf, exists := e.workflows[name]
	e.workflowsMu.RUnlock()

	if !exists || wf == nil {
		return nil, fmt.Errorf("workflow 不存在: %s", name)
	}
	return wf.Execute(ctx, params)
}

// GetWorkflows 返回已注册的工作流名称列表
func (e *Engine) GetWorkflows() []string {
	e.workflowsMu.RLock()
	defer e.workflowsMu.RUnlock()

	names := make([]string, 0, len(e.workflows))
	for n := range e.workflows {
		names = append(names, n)
	}
	return names
}
