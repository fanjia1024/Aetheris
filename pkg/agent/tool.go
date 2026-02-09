package agent

import (
	"context"

	"rag-platform/internal/agent/tools"
	"rag-platform/internal/runtime/session"
)

// ToolFunc 简单工具函数：无 Session 感知，供开发者注册
type ToolFunc func(ctx context.Context, input map[string]any) (string, error)

// ToolSchema 可选：工具入参的 JSON Schema（nil 时使用默认 object）
type ToolSchema map[string]any

// simpleTool 将 ToolFunc 适配为 agent/tools.Tool（Execute 忽略 session 与 state）
type simpleTool struct {
	name        string
	description string
	schema      map[string]any
	run         ToolFunc
}

func (t *simpleTool) Name() string        { return t.name }
func (t *simpleTool) Description() string { return t.description }

func (t *simpleTool) Schema() map[string]any {
	if t.schema != nil {
		return t.schema
	}
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

func (t *simpleTool) Execute(ctx context.Context, _ *session.Session, input map[string]any, _ interface{}) (any, error) {
	return t.run(ctx, input)
}

// Tool 在 Agent 上注册一个简单工具（name、description、run）；可选 schema 为 nil 时使用默认 object
// 注册后的工具会对 Planner 可见（Schema）并被 Runner 执行
func (a *Agent) Tool(name, description string, run ToolFunc, schema ToolSchema) {
	st := &simpleTool{
		name:        name,
		description: description,
		run:         run,
	}
	if schema != nil {
		st.schema = schema
	}
	a.registry.Register(st)
}

// RegisterTool 向 Agent 注册一个已实现的 tools.Tool（供需要 Session 或自定义 Schema 的进阶用法）
func (a *Agent) RegisterTool(t tools.Tool) {
	a.registry.Register(t)
}
