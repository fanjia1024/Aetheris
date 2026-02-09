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
	"encoding/json"
	"sync"
)

// Registry Agent 可发现的工具注册表
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry 创建新 Registry
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register 注册工具
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

// Get 按名称获取工具
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// List 返回所有已注册工具
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		list = append(list, t)
	}
	return list
}

// ToolSchemaForLLM 供 LLM 使用的工具描述
type ToolSchemaForLLM struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any  `json:"parameters"`
}

// ToolManifest 工具能力声明（可发现、可版本化）
type ToolManifest struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	InputSchema  map[string]any `json:"input_schema"`
	OutputSchema map[string]any `json:"output_schema,omitempty"`
	Timeout      string         `json:"timeout,omitempty"`
	Version      string         `json:"version,omitempty"`
}

// SchemasForLLM 返回所有工具的 Schema 列表（JSON，供 Planner 使用）
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

// Manifests 返回所有工具的 Manifest 列表（供 API/CLI 发现）
func (r *Registry) Manifests() []ToolManifest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]ToolManifest, 0, len(r.tools))
	for _, t := range r.tools {
		list = append(list, ToolManifest{
			Name:         t.Name(),
			Description:  t.Description(),
			InputSchema:  t.Schema(),
			OutputSchema: nil,
			Timeout:      "",
			Version:      "1.0",
		})
	}
	return list
}

// Manifest 返回指定名称工具的 Manifest，不存在返回 nil
func (r *Registry) Manifest(name string) *ToolManifest {
	t, ok := r.Get(name)
	if !ok {
		return nil
	}
	return &ToolManifest{
		Name:         t.Name(),
		Description:  t.Description(),
		InputSchema:  t.Schema(),
		OutputSchema: nil,
		Timeout:      "",
		Version:      "1.0",
	}
}
