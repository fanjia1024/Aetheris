package session

import (
	"time"
)

// ToolCallRecord 单次工具调用记录
type ToolCallRecord struct {
	Tool    string         `json:"tool"`
	Input   map[string]any `json:"input,omitempty"`
	Output  string         `json:"output"`
	Err     string         `json:"error,omitempty"`
	At      time.Time      `json:"at"`
}

// WorkingState 的键约定（可选，供工具与 Planner 使用）
const (
	WorkingKeyLastObservation = "last_observation"
	WorkingKeyStepIndex       = "step_index"
)
