package eino

import (
	"context"
	"fmt"
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
	runners map[string]*adk.Runner
	config  *config.Config
	logger  *log.Logger
	mu      sync.RWMutex
}

// NewEngine 创建新的 eino 引擎实例
func NewEngine(cfg *config.Config, logger *log.Logger) (*Engine, error) {
	// 创建引擎实例
	engine := &Engine{
		runners: make(map[string]*adk.Runner),
		config:  cfg,
		logger:  logger,
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
	// 创建工具
	tools := []tool.BaseTool{
		CreateRetrieverTool(),
		CreateGeneratorTool(),
	}

	// 创建 ChatModelAgent
	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: tools,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	// 创建 Runner
	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent: agent,
	})

	return runner, nil
}

// createIngestRunner 创建 Ingest Agent Runner
func (e *Engine) createIngestRunner(ctx context.Context) (*adk.Runner, error) {
	// 创建工具
	tools := []tool.BaseTool{
		CreateDocumentLoaderTool(),
		CreateDocumentParserTool(),
		CreateSplitterTool(),
		CreateEmbeddingTool(),
		CreateIndexBuilderTool(),
	}

	// 创建 ChatModelAgent
	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: tools,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	// 创建 Runner
	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent: agent,
	})

	return runner, nil
}

// createChatModel 创建 OpenAI 模型
func (e *Engine) createChatModel(ctx context.Context) (*openai.Model, error) {
	// 从配置中获取模型配置
	modelConfig := e.config.Model
	llmConfig := modelConfig.LLM

	// 获取默认提供商配置
	defaultProvider := "openai"
	providerConfig, exists := llmConfig.Providers[defaultProvider]
	if !exists {
		return nil, fmt.Errorf("未找到默认提供商配置: %s", defaultProvider)
	}

	// 创建 OpenAI 模型
	model, err := openai.NewModel(ctx, &openai.ModelConfig{
		Model:  providerConfig.Models["default"].Name,
		APIKey: providerConfig.APIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 OpenAI 模型失败: %w", err)
	}

	return model, nil
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
func (e *Engine) Execute(ctx context.Context, agentID string, query string) (chan adk.Event, error) {
	runner, err := e.GetRunner(agentID)
	if err != nil {
		return nil, err
	}

	// 执行查询
	iter := runner.Query(ctx, query)

	// 创建事件通道
	eventCh := make(chan adk.Event)

	// 启动 goroutine 处理事件
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
