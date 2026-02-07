package tools

import (
	"context"

	"rag-platform/internal/runtime/session"
)

// Tool Session 感知的工具接口
type Tool interface {
	Name() string
	Description() string
	Schema() map[string]any
	Execute(ctx context.Context, sess *session.Session, input map[string]any) (any, error)
}
