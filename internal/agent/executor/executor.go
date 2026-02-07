package executor

import (
	"context"
	"encoding/json"

	"rag-platform/internal/agent/planner"
	"rag-platform/internal/tool"
	"rag-platform/internal/tool/registry"
)

// Executor 按计划步骤执行工具调用
type Executor interface {
	ExecuteStep(ctx context.Context, step planner.PlanStep) (tool.ToolResult, error)
}

// RegistryExecutor 从 ToolRegistry 取工具并执行
type RegistryExecutor struct {
	reg *registry.Registry
}

// NewRegistryExecutor 创建基于 Registry 的 Executor
func NewRegistryExecutor(reg *registry.Registry) *RegistryExecutor {
	return &RegistryExecutor{reg: reg}
}

// ExecuteStep 实现 Executor
func (e *RegistryExecutor) ExecuteStep(ctx context.Context, step planner.PlanStep) (tool.ToolResult, error) {
	if e.reg == nil {
		return tool.ToolResult{Err: "Registry 未配置"}, nil
	}
	t, ok := e.reg.Get(step.Tool)
	if !ok {
		return tool.ToolResult{Err: "未知工具: " + step.Tool}, nil
	}
	input := step.Input
	if input == nil {
		input = make(map[string]any)
	}
	// 若 Input 来自 JSON 反序列化，数字可能是 float64
	normalizeInput(input)
	return t.Execute(ctx, input)
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
func ExecuteSteps(ctx context.Context, exec Executor, steps []planner.PlanStep) ([]tool.ToolResult, error) {
	results := make([]tool.ToolResult, 0, len(steps))
	for _, step := range steps {
		res, err := exec.ExecuteStep(ctx, step)
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
