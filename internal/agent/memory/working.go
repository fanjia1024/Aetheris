package memory

import (
	"context"
	"sync"

	"rag-platform/internal/agent/runtime"
)

// WorkingSession 基于 runtime.Session 的 Working Memory：当前上下文（Messages / Variables）
type WorkingSession struct {
	mu      sync.RWMutex
	session *runtime.Session
}

// NewWorkingSession 创建基于 Session 的 Working Memory
func NewWorkingSession(session *runtime.Session) *WorkingSession {
	return &WorkingSession{session: session}
}

// SetSession 设置当前 Session（用于切换会话）
func (w *WorkingSession) SetSession(session *runtime.Session) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.session = session
}

// Recall 从 Session.Messages 与 Variables 中召回与 query 相关的项（简单实现：返回最近 N 条消息）
func (w *WorkingSession) Recall(ctx context.Context, query string) ([]MemoryItem, error) {
	w.mu.RLock()
	sess := w.session
	w.mu.RUnlock()
	if sess == nil {
		return nil, nil
	}
	msgs := sess.CopyMessages()
	items := make([]MemoryItem, 0, len(msgs))
	for _, m := range msgs {
		items = append(items, MemoryItem{
			Type:    "working",
			Content: m.Content,
			Metadata: map[string]any{
				"role": m.Role,
			},
			At: m.Time,
		})
	}
	return items, nil
}

// Store 将 item 写入 Session（追加一条 message 或 set variable）
func (w *WorkingSession) Store(ctx context.Context, item MemoryItem) error {
	w.mu.RLock()
	sess := w.session
	w.mu.RUnlock()
	if sess == nil {
		return nil
	}
	if role, ok := item.Metadata["role"].(string); ok && role != "" {
		sess.AddMessage(role, item.Content)
		return nil
	}
	if key, ok := item.Metadata["key"].(string); ok && key != "" {
		sess.SetVariable(key, item.Content)
		return nil
	}
	sess.AddMessage("system", item.Content)
	return nil
}

// Working 独立于 Session 的工作记忆实现：当前任务步骤结果（map[sessionID][]StepResult）
type Working struct {
	mu       sync.RWMutex
	sessions map[string][]StepResult
}

// NewWorking 创建工作记忆
func NewWorking() *Working {
	return &Working{
		sessions: make(map[string][]StepResult),
	}
}

// GetStepResults 返回该 session 当前任务的步骤结果
func (w *Working) GetStepResults(sessionID string) []StepResult {
	w.mu.RLock()
	defer w.mu.RUnlock()
	list := w.sessions[sessionID]
	if len(list) == 0 {
		return nil
	}
	out := make([]StepResult, len(list))
	copy(out, list)
	return out
}

// SetStepResults 设置该 session 的步骤结果（覆盖）
func (w *Working) SetStepResults(sessionID string, results []StepResult) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(results) == 0 {
		delete(w.sessions, sessionID)
		return
	}
	w.sessions[sessionID] = results
}

// Clear 清空该 session 的工作记忆
func (w *Working) Clear(sessionID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.sessions, sessionID)
}

// Recall 实现 Memory：按 query 忽略，返回该 session 的步骤结果（若 sessionID 在 metadata）
func (w *Working) Recall(ctx context.Context, query string) ([]MemoryItem, error) {
	// 无 session 上下文时无法召回；调用方可通过 Store 写入
	return nil, nil
}

// Store 实现 Memory：将 item 视为步骤结果写入（需 metadata["session_id"]）
func (w *Working) Store(ctx context.Context, item MemoryItem) error {
	if item.Metadata == nil {
		return nil
	}
	sid, _ := item.Metadata["session_id"].(string)
	if sid == "" {
		return nil
	}
	tool, _ := item.Metadata["tool"].(string)
	input, _ := item.Metadata["input"].(string)
	errStr, _ := item.Metadata["error"].(string)
	w.mu.Lock()
	defer w.mu.Unlock()
	w.sessions[sid] = append(w.sessions[sid], StepResult{
		Tool:   tool,
		Input:  input,
		Output: item.Content,
		Err:    errStr,
	})
	return nil
}
