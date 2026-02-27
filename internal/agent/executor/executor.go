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

package executor

import (
	"context"
	"encoding/json"
	"fmt"

	"rag-platform/internal/agent/planner"
	"rag-platform/internal/agent/tools"
	"rag-platform/internal/runtime/session"
	"rag-platform/internal/tool"
	"rag-platform/internal/tool/registry"
)

// Executor 按计划步骤执行工具调用（Session 感知）
type Executor interface {
	ExecuteStep(ctx context.Context, sess *session.Session, step planner.PlanStep) (tool.ToolResult, error)
}

// RegistryExecutor 从 tool/registry 取工具并执行（兼容旧路径，session 可忽略）
type RegistryExecutor struct {
	reg *registry.Registry
}

// NewRegistryExecutor 创建基于 tool/registry 的 Executor
func NewRegistryExecutor(reg *registry.Registry) *RegistryExecutor {
	return &RegistryExecutor{reg: reg}
}

// ExecuteStep 实现 Executor（忽略 session）
func (e *RegistryExecutor) ExecuteStep(ctx context.Context, _ *session.Session, step planner.PlanStep) (tool.ToolResult, error) {
	if e.reg == nil {
		return tool.ToolResult{Err: "Registry not configured"}, nil
	}
	t, ok := e.reg.Get(step.Tool)
	if !ok {
		return tool.ToolResult{Err: "Unknown tool: " + step.Tool}, nil
	}
	input := step.Input
	if input == nil {
		input = make(map[string]any)
	}
	normalizeInput(input)
	return t.Execute(ctx, input)
}

// SessionRegistryExecutor 从 agent/tools.Registry 取工具并执行（传入 Session）
type SessionRegistryExecutor struct {
	reg *tools.Registry
}

// NewSessionRegistryExecutor 创建基于 agent/tools.Registry 的 Executor
func NewSessionRegistryExecutor(reg *tools.Registry) *SessionRegistryExecutor {
	return &SessionRegistryExecutor{reg: reg}
}

// ExecuteStep 实现 Executor
func (e *SessionRegistryExecutor) ExecuteStep(ctx context.Context, sess *session.Session, step planner.PlanStep) (tool.ToolResult, error) {
	if e.reg == nil {
		return tool.ToolResult{Err: "Registry not configured"}, nil
	}
	if sess == nil {
		return tool.ToolResult{Err: "Session not provided"}, nil
	}
	t, ok := e.reg.Get(step.Tool)
	if !ok {
		return tool.ToolResult{Err: "Unknown tool: " + step.Tool}, nil
	}
	input := step.Input
	if input == nil {
		input = make(map[string]any)
	}
	normalizeInput(input)
	out, err := t.Execute(ctx, sess, input, nil)
	if err != nil {
		return tool.ToolResult{Err: err.Error()}, nil
	}
	if tr, ok := out.(tool.ToolResult); ok {
		return tr, nil
	}
	content := ""
	if out != nil {
		content = fmt.Sprint(out)
		if b, err := json.Marshal(out); err == nil && len(b) > 0 && b[0] == '"' {
			content = string(b)
		}
	}
	return tool.ToolResult{Content: content}, nil
}

// normalizeInput 将 map 中 JSON 反序列化得到的 float64 转为 int 等，避免工具层类型断言失败
func normalizeInput(m map[string]any) {
	for k, v := range m {
		if f, ok := v.(float64); ok && f == float64(int(f)) {
			m[k] = int(f)
		}
		if nested, ok := v.(map[string]any); ok {
			normalizeInput(nested)
		}
	}
}

// ExecuteSteps 顺序执行多步，返回每步结果（JSON 序列化）供 Planner 下一轮使用
func ExecuteSteps(ctx context.Context, exec Executor, sess *session.Session, steps []planner.PlanStep) ([]tool.ToolResult, error) {
	results := make([]tool.ToolResult, 0, len(steps))
	for _, step := range steps {
		res, err := exec.ExecuteStep(ctx, sess, step)
		if err != nil {
			res = tool.ToolResult{Err: err.Error()}
		}
		results = append(results, res)
	}
	return results, nil
}

// FormatStepResultsForLLM 将步骤结果格式化为供 LLM 阅读的字符串
func FormatStepResultsForLLM(results []tool.ToolResult) string {
	if len(results) == 0 {
		return ""
	}
	b, _ := json.Marshal(results)
	return string(b)
}
