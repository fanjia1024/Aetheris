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
