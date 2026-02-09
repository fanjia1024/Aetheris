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
