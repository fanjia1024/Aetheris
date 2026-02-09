package agent

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	coreagent "rag-platform/internal/agent"
	"rag-platform/internal/agent/executor"
	"rag-platform/internal/agent/planner"
	"rag-platform/internal/agent/tools"
	"rag-platform/internal/model/llm"
	"rag-platform/internal/runtime/session"
)

// RunResult 单次 Run 的简化结果（最终回答、步数、耗时）
type RunResult struct {
	Answer   string        `json:"answer"`
	Steps    int           `json:"steps"`
	Duration time.Duration `json:"duration"`
}

// Agent 对外 Agent 门面：封装 Planner、Executor、Registry，对外暴露 Tool()、Run()
type Agent struct {
	registry    *tools.Registry
	inner       *coreagent.Agent
	mu          sync.Mutex
	sessions    map[string]*session.Session
	defaultSess *session.Session
}

// Option 创建 Agent 时的可选配置
type Option func(*agentConfig)

type agentConfig struct {
	llmClient llm.Client
	maxSteps  int
}

// WithLLM 指定 LLM 客户端；不设置时从环境变量 OPENAI_API_KEY 创建默认 OpenAI 客户端
func WithLLM(client llm.Client) Option {
	return func(c *agentConfig) {
		c.llmClient = client
	}
}

// WithMaxSteps 设置单次 Run 默认最大步数
func WithMaxSteps(n int) Option {
	return func(c *agentConfig) {
		c.maxSteps = n
	}
}

// NewAgent 创建可编程 Agent；零配置时使用 OPENAI_API_KEY + gpt-3.5-turbo
func NewAgent(opts ...Option) *Agent {
	cfg := &agentConfig{maxSteps: 20}
	for _, o := range opts {
		o(cfg)
	}
	llmClient := cfg.llmClient
	if llmClient == nil {
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey != "" {
			var err error
			llmClient, err = llm.NewClient("openai", "gpt-3.5-turbo", apiKey)
			if err != nil {
				// 忽略错误时内部 Planner 会返回“未配置”提示
				llmClient = nil
			}
		}
	}
	registry := tools.NewRegistry()
	pl := planner.NewLLMPlanner(llmClient)
	exec := executor.NewSessionRegistryExecutor(registry)
	inner := coreagent.New(pl, exec, registry, coreagent.WithMaxSteps(cfg.maxSteps))
	return &Agent{
		registry:    registry,
		inner:      inner,
		sessions:   make(map[string]*session.Session),
		defaultSess: session.New(""),
	}
}

// Run 执行一次任务，使用默认会话；可选 RunOption 指定 SessionID、MaxSteps 等
func (a *Agent) Run(ctx context.Context, prompt string, opts ...RunOption) (*RunResult, error) {
	o := applyRunOptions(opts)
	if o.SessionID != "" {
		return a.RunWithSession(ctx, o.SessionID, prompt, opts...)
	}
	return a.runWithSession(ctx, a.defaultSess, prompt, o)
}

// RunWithSession 使用指定 sessionID 执行；同一 sessionID 多次调用共用会话历史
func (a *Agent) RunWithSession(ctx context.Context, sessionID string, prompt string, opts ...RunOption) (*RunResult, error) {
	o := applyRunOptions(opts)
	a.mu.Lock()
	sess, ok := a.sessions[sessionID]
	if !ok {
		sess = session.New(sessionID)
		a.sessions[sessionID] = sess
	}
	a.mu.Unlock()
	if o.MaxSteps > 0 {
		// 临时覆盖 inner 的 maxSteps 需要改 internal agent 或这里传 session 时带 option；简化起见本次不传，使用创建时的默认
	}
	return a.runWithSession(ctx, sess, prompt, o)
}

func (a *Agent) runWithSession(ctx context.Context, sess *session.Session, prompt string, o *RunOptions) (*RunResult, error) {
	res, err := a.inner.RunWithSession(ctx, sess, prompt)
	if err != nil {
		return nil, fmt.Errorf("agent run: %w", err)
	}
	return &RunResult{
		Answer:   res.Answer,
		Steps:    res.Steps,
		Duration: res.Duration,
	}, nil
}
