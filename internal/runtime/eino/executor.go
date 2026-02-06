package eino

import (
	"context"
)

// WorkflowExecutor 工作流执行器：按名称执行工作流，参数与结果为通用 map，便于 API 与 eino 解耦。
type WorkflowExecutor interface {
	Execute(ctx context.Context, params map[string]interface{}) (interface{}, error)
}
