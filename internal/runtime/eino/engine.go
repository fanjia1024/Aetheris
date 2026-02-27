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
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
)

// Engine eino 引擎实例
type Engine struct {
	runners     map[string]*adk.Runner
	workflows   map[string]WorkflowExecutor
	config      *config.Config
	logger      *log.Logger
	mu          sync.RWMutex
	workflowsMu sync.RWMutex

	// 可选组件（由 app 注入后工具链对接真实实现）
	Retriever         Retriever
	Generator         Generator
	DocumentLoader    DocumentLoader
	DocumentParser    DocumentParser
	DocumentSplitter  DocumentSplitter
	DocumentEmbedding DocumentEmbedding
	DocumentIndexer   DocumentIndexer

	// EinoDocLoader / EinoDocTransformer 为 Eino 编排用（Load Source.URI → []*schema.Document；Transform 解析+切片）
	EinoDocLoader      document.Loader
	EinoDocTransformer document.Transformer
}

// NewEngine 创建新的 eino 引擎实例
func NewEngine(cfg *config.Config, logger *log.Logger) (*Engine, error) {
	// 创建引擎实例
	engine := &Engine{
		runners:   make(map[string]*adk.Runner),
		workflows: make(map[string]WorkflowExecutor),
		config:    cfg,
		logger:    logger,
	}

	// 启动引擎
	if err := engine.start(); err != nil {
		return nil, fmt.Errorf("启动 eino 引擎failed: %w", err)
	}

	logger.Info("eino 引擎初始化成功")
	return engine, nil
}

// start 启动 eino 引擎（仅注册默认 Workflow；Runner 懒创建）
func (e *Engine) start() error {
	e.registerDefaultWorkflows()
	return nil
}

// SetQueryComponents 设置查询侧组件（Retriever / Generator），供 qa_agent 工具使用
func (e *Engine) SetQueryComponents(retriever Retriever, generator Generator) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Retriever = retriever
	e.Generator = generator
}

// SetIngestComponents 设置入库侧组件，供 ingest_agent 工具使用
func (e *Engine) SetIngestComponents(loader DocumentLoader, parser DocumentParser, splitter DocumentSplitter, embedding DocumentEmbedding, indexer DocumentIndexer) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.DocumentLoader = loader
	e.DocumentParser = parser
	e.DocumentSplitter = splitter
	e.DocumentEmbedding = embedding
	e.DocumentIndexer = indexer
}

// SetEinoDocumentComponents 设置 Eino Document Loader / Transformer，供 Eino 链式编排（compose.AppendLoader / AppendDocumentTransformer）使用
func (e *Engine) SetEinoDocumentComponents(loader document.Loader, transformer document.Transformer) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.EinoDocLoader = loader
	e.EinoDocTransformer = transformer
}

// ensureRunner 懒创建并注册 Runner
func (e *Engine) ensureRunner(agentID string) (*adk.Runner, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if r, ok := e.runners[agentID]; ok {
		return r, nil
	}
	ctx := context.Background()
	var runner *adk.Runner
	var err error
	switch agentID {
	case "qa_agent":
		runner, err = e.createQARunner(ctx)
	case "ingest_agent":
		runner, err = e.createIngestRunner(ctx)
	default:
		return nil, fmt.Errorf("Runner %s not found", agentID)
	}
	if err != nil {
		return nil, err
	}
	e.runners[agentID] = runner
	return runner, nil
}

// createQARunner 创建 QA Agent Runner（使用已注入的 Retriever/Generator）
func (e *Engine) createQARunner(ctx context.Context) (*adk.Runner, error) {
	tools := []tool.BaseTool{
		CreateRetrieverTool(e),
		CreateGeneratorTool(e),
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

// createIngestRunner 创建 Ingest Agent Runner（使用已注入的 ingest 组件）
func (e *Engine) createIngestRunner(ctx context.Context) (*adk.Runner, error) {
	tools := []tool.BaseTool{
		CreateDocumentLoaderTool(e),
		CreateDocumentParserTool(e),
		CreateSplitterTool(e),
		CreateEmbeddingTool(e),
		CreateIndexBuilderTool(e),
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
		return nil, fmt.Errorf("LLM provider %q not configured", provider)
	}
	mi, ok := pc.Models[modelKey]
	if !ok {
		return nil, fmt.Errorf("LLM model %q not configured in provider %q", modelKey, provider)
	}
	if pc.APIKey == "" {
		return nil, fmt.Errorf("LLM provider %q api_key not configured", provider)
	}

	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		Model:  mi.Name,
		APIKey: pc.APIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 OpenAI ChatModel failed: %w", err)
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

// CreateChatModel 根据配置创建 ChatModel（供 app 层构建主 ADK Agent 等复用）
func (e *Engine) CreateChatModel(ctx context.Context) (model.ToolCallingChatModel, error) {
	cm, err := e.createChatModel(ctx)
	if err != nil || cm == nil {
		return nil, err
	}
	return cm, nil
}

// GetRunner 获取 Runner 实例（qa_agent / ingest_agent 懒创建）
func (e *Engine) GetRunner(agentID string) (*adk.Runner, error) {
	e.mu.RLock()
	runner, exists := e.runners[agentID]
	e.mu.RUnlock()

	if exists {
		return runner, nil
	}
	return e.ensureRunner(agentID)
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
		return fmt.Errorf("workflow name is required")
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
		return nil, fmt.Errorf("workflow not found: %s", name)
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
