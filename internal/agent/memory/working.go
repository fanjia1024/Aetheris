package memory

import (
	"sync"
)

// Working 工作记忆的 in-memory 实现：当前任务步骤结果
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
