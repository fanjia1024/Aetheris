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

// ToolWithCapability 可选接口：声明工具所需 capability，供 RBAC/capability policy 校验；未实现时使用工具名
type ToolWithCapability interface {
	Tool
	// RequiredCapability 返回该工具所需能力标识，空则使用 Name()
	RequiredCapability() string
}
