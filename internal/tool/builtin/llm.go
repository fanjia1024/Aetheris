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

package builtin

import (
	"context"

	"rag-platform/internal/tool"
)

// PromptGenerator 生成文本的接口（可由 eino.Generator 或 llm.Client 适配）
type PromptGenerator interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

// LLMGenerateTool 实现 llm.generate
type LLMGenerateTool struct {
	gen PromptGenerator
}

// NewLLMGenerateTool 创建 llm.generate 工具
func NewLLMGenerateTool(gen PromptGenerator) *LLMGenerateTool {
	return &LLMGenerateTool{gen: gen}
}

// Name 实现 tool.Tool
func (t *LLMGenerateTool) Name() string { return "llm.generate" }

// Description 实现 tool.Tool
func (t *LLMGenerateTool) Description() string {
	return "调用大模型根据提示生成文本。传入 prompt 即可。"
}

// Schema 实现 tool.Tool
func (t *LLMGenerateTool) Schema() tool.Schema {
	return tool.Schema{
		Type:        "object",
		Description: "生成参数",
		Properties: map[string]tool.SchemaProperty{
			"prompt": {Type: "string", Description: "提示文本"},
		},
		Required: []string{"prompt"},
	}
}

// Execute 实现 tool.Tool
func (t *LLMGenerateTool) Execute(ctx context.Context, input map[string]any) (tool.ToolResult, error) {
	if t.gen == nil {
		return tool.ToolResult{Err: "生成器未配置"}, nil
	}
	prompt, _ := input["prompt"].(string)
	if prompt == "" {
		return tool.ToolResult{Err: "prompt 不能为空"}, nil
	}
	content, err := t.gen.Generate(ctx, prompt)
	if err != nil {
		return tool.ToolResult{Err: err.Error()}, nil
	}
	return tool.ToolResult{Content: content}, nil
}
