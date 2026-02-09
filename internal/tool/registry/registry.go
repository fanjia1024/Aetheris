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

package registry

import (
	"encoding/json"
	"sync"

	"rag-platform/internal/tool"
)

// Registry 工具注册表：注册、发现、供 LLM 使用的 Schema 列表
type Registry struct {
	mu    sync.RWMutex
	tools map[string]tool.Tool
}

// New 创建新的 ToolRegistry
func New() *Registry {
	return &Registry{
		tools: make(map[string]tool.Tool),
	}
}

// Register 注册工具
func (r *Registry) Register(t tool.Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

// Get 按名称获取工具
func (r *Registry) Get(name string) (tool.Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// List 返回所有已注册工具
func (r *Registry) List() []tool.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]tool.Tool, 0, len(r.tools))
	for _, t := range r.tools {
		list = append(list, t)
	}
	return list
}

// ToolSchemaForLLM 单个工具供 LLM 使用的描述（name, description, parameters）
type ToolSchemaForLLM struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  tool.Schema `json:"parameters"`
}

// SchemasForLLM 返回所有工具的 Schema 列表（JSON 序列化供 Planner/LLM 使用）
func (r *Registry) SchemasForLLM() ([]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]ToolSchemaForLLM, 0, len(r.tools))
	for _, t := range r.tools {
		list = append(list, ToolSchemaForLLM{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Schema(),
		})
	}
	return json.Marshal(list)
}
