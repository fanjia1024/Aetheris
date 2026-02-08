package tools

import (
	"context"

	"rag-platform/internal/runtime/session"
)

// ToolResult 工具执行结果；支持未完成时挂起、再次进入时携带 State
type ToolResult struct {
	Done   bool        `json:"done"`             // 是否已完成
	State  interface{} `json:"state,omitempty"`  // 未完成时携带的状态，再入时传入
	Output string      `json:"output,omitempty"` // 输出内容
	Err    string      `json:"error,omitempty"`
}

// Tool Session 感知的工具接口；state 为可选，再入时传入上次的 ToolResult.State
type Tool interface {
	Name() string
	Description() string
	Schema() map[string]any
	Execute(ctx context.Context, sess *session.Session, input map[string]any, state interface{}) (any, error)
}
