package tools

import (
	"context"
	"encoding/json"

	"rag-platform/internal/runtime/session"
	"rag-platform/internal/tool"
)

// Wrap 将无 Session 的 tool.Tool 包装为 Session 感知的 tools.Tool（Execute 时忽略 session）
func Wrap(t tool.Tool) Tool {
	return &wrappedTool{t: t}
}

type wrappedTool struct {
	t tool.Tool
}

func (w *wrappedTool) Name() string        { return w.t.Name() }
func (w *wrappedTool) Description() string { return w.t.Description() }

func (w *wrappedTool) Schema() map[string]any {
	s := w.t.Schema()
	b, _ := json.Marshal(s)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

func (w *wrappedTool) Execute(ctx context.Context, _ *session.Session, input map[string]any) (any, error) {
	res, err := w.t.Execute(ctx, input)
	if err != nil {
		return nil, err
	}
	return res, nil
}
