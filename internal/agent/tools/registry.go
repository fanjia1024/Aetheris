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
