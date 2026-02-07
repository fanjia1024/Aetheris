package tool

import (
	"context"
)

// Schema 表示工具的 JSON Schema（供 LLM function-calling 使用）
type Schema struct {
	Type        string                    `json:"type,omitempty"`
	Description string                    `json:"description,omitempty"`
	Properties  map[string]SchemaProperty `json:"properties,omitempty"`
	Required    []string                  `json:"required,omitempty"`
}

// SchemaProperty 表示 Schema 中单个属性的描述
type SchemaProperty struct {
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
}

// ToolResult 工具执行结果
type ToolResult struct {
	Content string `json:"content"`
	Err     string `json:"error,omitempty"`
}

// Tool Runtime 级工具接口
type Tool interface {
	Name() string
	Description() string
	Schema() Schema
	Execute(ctx context.Context, input map[string]any) (ToolResult, error)
}
