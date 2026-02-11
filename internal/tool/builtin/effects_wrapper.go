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
	"encoding/json"
	"net/http"
	"time"

	"rag-platform/internal/tool"
	"rag-platform/pkg/effects"
)

// EffectsToolAdapter wraps any tool.Tool with effects isolation.
type EffectsToolAdapter struct {
	tool    tool.Tool
	effects effects.System
	caller  effects.ToolCaller
}

// WrapToolWithEffects wraps any tool with effects isolation.
func WrapToolWithEffects(t tool.Tool, sys effects.System, caller effects.ToolCaller) *EffectsToolAdapter {
	return &EffectsToolAdapter{
		tool:    t,
		effects: sys,
		caller:  caller,
	}
}

// SetEffectsSystem updates the effects system.
func (a *EffectsToolAdapter) SetEffectsSystem(sys effects.System) {
	a.effects = sys
}

// Name implements tool.Tool
func (a *EffectsToolAdapter) Name() string {
	return a.tool.Name()
}

// Description implements tool.Tool
func (a *EffectsToolAdapter) Description() string {
	return a.tool.Description()
}

// Schema implements tool.Tool
func (a *EffectsToolAdapter) Schema() tool.Schema {
	return a.tool.Schema()
}

// Execute implements tool.Tool with effects isolation.
func (a *EffectsToolAdapter) Execute(ctx context.Context, input map[string]any) (tool.ToolResult, error) {
	result, err := effects.ExecuteTool(ctx, a.effects, a.tool.Name(), input, a.caller)
	if err != nil {
		return tool.ToolResult{Err: err.Error()}, nil
	}

	content, _ := result["content"].(string)
	return tool.ToolResult{Content: content}, nil
}

// Unwrap returns the underlying tool.
func (a *EffectsToolAdapter) Unwrap() tool.Tool {
	return a.tool
}

// EffectsToolRegistry manages effects-wrapped tools.
type EffectsToolRegistry struct {
	tools   map[string]tool.Tool
	effects effects.System
}

// NewEffectsToolRegistry creates a new registry.
func NewEffectsToolRegistry(sys effects.System) *EffectsToolRegistry {
	return &EffectsToolRegistry{
		tools:   make(map[string]tool.Tool),
		effects: sys,
	}
}

// Register registers a tool with effects isolation.
func (r *EffectsToolRegistry) Register(t tool.Tool, caller effects.ToolCaller) {
	adapter := WrapToolWithEffects(t, r.effects, caller)
	r.tools[t.Name()] = adapter
}

// RegisterWithCaller registers a tool with a custom caller.
func (r *EffectsToolRegistry) RegisterWithCaller(name string, desc string, schema tool.Schema, caller effects.ToolCaller) {
	adapter := &EffectsToolAdapter{
		tool:    &baseTool{name: name, description: desc, schema: schema},
		effects: r.effects,
		caller:  caller,
	}
	r.tools[name] = adapter
}

// Get returns a tool by name.
func (r *EffectsToolRegistry) Get(name string) (tool.Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// List returns all registered tools.
func (r *EffectsToolRegistry) List() []tool.Tool {
	result := make([]tool.Tool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	return result
}

// SetEffectsSystem updates the effects system for all tools.
func (r *EffectsToolRegistry) SetEffectsSystem(sys effects.System) {
	r.effects = sys
	for _, t := range r.tools {
		if adapter, ok := t.(*EffectsToolAdapter); ok {
			adapter.SetEffectsSystem(sys)
		}
	}
}

// baseTool is a simple tool implementation for registration.
type baseTool struct {
	name        string
	description string
	schema      tool.Schema
}

func (t *baseTool) Name() string        { return t.name }
func (t *baseTool) Description() string { return t.description }
func (t *baseTool) Schema() tool.Schema { return t.schema }
func (t *baseTool) Execute(ctx context.Context, input map[string]any) (tool.ToolResult, error) {
	return tool.ToolResult{}, nil
}

// EffectsHTTPAdapter wraps HTTP tool with effects isolation.
type EffectsHTTPAdapter struct {
	tool    tool.Tool
	effects effects.System
	caller  effects.HTTPCaller
}

// WrapHTTPToolWithEffects wraps an HTTP tool with effects isolation.
func WrapHTTPToolWithEffects(t tool.Tool, sys effects.System, caller effects.HTTPCaller) *EffectsHTTPAdapter {
	return &EffectsHTTPAdapter{
		tool:    t,
		effects: sys,
		caller:  caller,
	}
}

// SetEffectsSystem updates the effects system.
func (a *EffectsHTTPAdapter) SetEffectsSystem(sys effects.System) {
	a.effects = sys
}

// Name implements tool.Tool
func (a *EffectsHTTPAdapter) Name() string {
	return a.tool.Name()
}

// Description implements tool.Tool
func (a *EffectsHTTPAdapter) Description() string {
	return a.tool.Description()
}

// Schema implements tool.Tool
func (a *EffectsHTTPAdapter) Schema() tool.Schema {
	return a.tool.Schema()
}

// Execute implements tool.Tool with effects isolation.
func (a *EffectsHTTPAdapter) Execute(ctx context.Context, input map[string]any) (tool.ToolResult, error) {
	method, _ := input["method"].(string)
	urlStr, _ := input["url"].(string)
	if method == "" || urlStr == "" {
		return tool.ToolResult{Err: "method 和 url 不能为空"}, nil
	}

	headers := make(map[string]string)
	if h, ok := input["headers"].(map[string]interface{}); ok {
		for k, v := range h {
			if s, ok := v.(string); ok {
				headers[k] = s
			}
		}
	}

	var body []byte
	if b, ok := input["body"].(string); ok && b != "" {
		body = []byte(b)
	}

	req := effects.HTTPRequest{
		Method:  method,
		URL:     urlStr,
		Headers: headers,
		Body:    body,
	}

	response, err := effects.ExecuteHTTP(ctx, a.effects, req, a.caller)
	if err != nil {
		return tool.ToolResult{Err: err.Error()}, nil
	}

	out := map[string]interface{}{
		"status_code": response.StatusCode,
		"body":        string(response.Body),
	}
	data, _ := json.Marshal(out)
	return tool.ToolResult{Content: string(data)}, nil
}

// DefaultHTTPCaller creates a default HTTP caller using the standard http.Client.
func DefaultHTTPCaller() effects.HTTPCaller {
	return func(ctx context.Context, req effects.HTTPRequest) (effects.HTTPResponse, error) {
		// Convert effect request to http.Request
		httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, nil)
		if err != nil {
			return effects.HTTPResponse{}, err
		}

		for k, v := range req.Headers {
			httpReq.Header.Set(k, v)
		}

		if len(req.Body) > 0 {
			// Would need to add body handling here
		}

		client := &http.Client{Timeout: 30 * time.Second}
		if req.Timeout != nil {
			client.Timeout = *req.Timeout
		}

		start := time.Now()
		resp, err := client.Do(httpReq)
		duration := time.Since(start)

		if err != nil {
			return effects.HTTPResponse{}, err
		}
		defer resp.Body.Close()

		body, _ := json.Marshal(resp.Body)

		return effects.HTTPResponse{
			StatusCode: resp.StatusCode,
			Headers:    req.Headers,
			Body:       body,
			Duration:   duration,
		}, nil
	}
}

// EffectsLLMAdapter wraps LLM tool with effects isolation.
type EffectsLLMAdapter struct {
	tool    tool.Tool
	effects effects.System
	caller  effects.LLMCaller
}

// WrapLLMToolWithEffects wraps an LLM tool with effects isolation.
func WrapLLMToolWithEffects(t tool.Tool, sys effects.System, caller effects.LLMCaller) *EffectsLLMAdapter {
	return &EffectsLLMAdapter{
		tool:    t,
		effects: sys,
		caller:  caller,
	}
}

// SetEffectsSystem updates the effects system.
func (a *EffectsLLMAdapter) SetEffectsSystem(sys effects.System) {
	a.effects = sys
}

// Name implements tool.Tool
func (a *EffectsLLMAdapter) Name() string {
	return a.tool.Name()
}

// Description implements tool.Tool
func (a *EffectsLLMAdapter) Description() string {
	return a.tool.Description()
}

// Schema implements tool.Tool
func (a *EffectsLLMAdapter) Schema() tool.Schema {
	return a.tool.Schema()
}

// Execute implements tool.Tool with effects isolation.
func (a *EffectsLLMAdapter) Execute(ctx context.Context, input map[string]any) (tool.ToolResult, error) {
	prompt, _ := input["prompt"].(string)
	if prompt == "" {
		return tool.ToolResult{Err: "prompt 不能为空"}, nil
	}

	req := effects.LLMRequest{
		Model: "default",
		Messages: []effects.LLMMessage{
			{Role: "user", Content: prompt},
		},
	}

	response, err := effects.ExecuteLLM(ctx, a.effects, req, a.caller)
	if err != nil {
		return tool.ToolResult{Err: err.Error()}, nil
	}

	return tool.ToolResult{Content: response.Content}, nil
}
