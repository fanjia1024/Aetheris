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
	"net/http"
	"testing"

	"rag-platform/internal/tool"
	"rag-platform/pkg/effects"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTool is a simple mock tool for testing.
type mockTool struct {
	executeCalls int
	lastInput    map[string]any
	response     string
}

func (m *mockTool) Name() string        { return "mock_tool" }
func (m *mockTool) Description() string { return "Mock tool for testing" }
func (m *mockTool) Schema() tool.Schema { return tool.Schema{} }

func (m *mockTool) Execute(ctx context.Context, input map[string]any) (tool.ToolResult, error) {
	m.executeCalls++
	m.lastInput = input
	return tool.ToolResult{Content: m.response}, nil
}

func TestEffectsToolAdapter_Execute(t *testing.T) {
	sys := effects.NewMemorySystem()

	// Create an adapter with a mock caller that returns response
	adapter := WrapToolWithEffects(&mockTool{response: "mock response"}, sys, func(ctx context.Context, name string, args map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"content": "mock response"}, nil
	})

	result, err := adapter.Execute(context.Background(), map[string]any{"key": "value"})
	require.NoError(t, err)
	assert.Equal(t, "mock response", result.Content)
}

func TestEffectsToolAdapter_Replay(t *testing.T) {
	sys := effects.NewMemorySystem()

	// Create an adapter with a mock caller
	adapter := WrapToolWithEffects(&mockTool{response: "cached response"}, sys, func(ctx context.Context, name string, args map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"content": "cached response"}, nil
	})

	// First call - should actually call the caller
	result1, err := adapter.Execute(context.Background(), map[string]any{"key": "value"})
	require.NoError(t, err)
	assert.Equal(t, "cached response", result1.Content)

	// Replay - should use cached result
	ctx := effects.WithReplay(context.Background(), true)
	result2, err := adapter.Execute(ctx, map[string]any{"key": "value"})
	require.NoError(t, err)
	assert.Equal(t, result1.Content, result2.Content)
}

func TestEffectsToolAdapter_Idempotency(t *testing.T) {
	sys := effects.NewMemorySystem()

	var callCount int
	adapter := WrapToolWithEffects(&mockTool{response: "idempotent response"}, sys, func(ctx context.Context, name string, args map[string]interface{}) (map[string]interface{}, error) {
		callCount++
		return map[string]interface{}{"content": "idempotent response"}, nil
	})

	// Same input twice
	result1, err := adapter.Execute(context.Background(), map[string]any{"key": "same"})
	require.NoError(t, err)
	assert.Equal(t, "idempotent response", result1.Content)

	result2, err := adapter.Execute(context.Background(), map[string]any{"key": "same"})
	require.NoError(t, err)
	assert.Equal(t, result1.Content, result2.Content)

	// Only one real call due to idempotency
	assert.Equal(t, 1, callCount)
}

func TestEffectsToolAdapter_DifferentInputs(t *testing.T) {
	sys := effects.NewMemorySystem()

	var callCount int
	adapter := WrapToolWithEffects(&mockTool{response: "response"}, sys, func(ctx context.Context, name string, args map[string]interface{}) (map[string]interface{}, error) {
		callCount++
		return map[string]interface{}{"content": "response"}, nil
	})

	// Different inputs should both call the caller
	_, err := adapter.Execute(context.Background(), map[string]any{"key": "a"})
	require.NoError(t, err)

	_, err = adapter.Execute(context.Background(), map[string]any{"key": "b"})
	require.NoError(t, err)

	// Two different inputs = two calls
	assert.Equal(t, 2, callCount)
}

func TestEffectsToolAdapter_Unwrap(t *testing.T) {
	mock := &mockTool{response: "test"}
	sys := effects.NewMemorySystem()

	adapter := WrapToolWithEffects(mock, sys, func(ctx context.Context, name string, args map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"content": "test"}, nil
	})

	unwrapped := adapter.Unwrap()
	assert.Same(t, mock, unwrapped)
}

func TestEffectsToolRegistry(t *testing.T) {
	sys := effects.NewMemorySystem()
	registry := NewEffectsToolRegistry(sys)

	mock := &mockTool{response: "registered response"}
	registry.Register(mock, func(ctx context.Context, name string, args map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"content": "registered response"}, nil
	})

	// Get tool
	tool, ok := registry.Get("mock_tool")
	require.True(t, ok)

	result, err := tool.Execute(context.Background(), map[string]any{"key": "value"})
	require.NoError(t, err)
	assert.Equal(t, "registered response", result.Content)
}

func TestEffectsToolRegistry_List(t *testing.T) {
	sys := effects.NewMemorySystem()
	registry := NewEffectsToolRegistry(sys)

	// Use RegisterWithCaller with unique names to avoid name collision
	registry.RegisterWithCaller("tool_1", "Test tool 1", tool.Schema{}, func(ctx context.Context, name string, args map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"content": "response1"}, nil
	})
	registry.RegisterWithCaller("tool_2", "Test tool 2", tool.Schema{}, func(ctx context.Context, name string, args map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"content": "response2"}, nil
	})

	tools := registry.List()
	assert.Len(t, tools, 2)
}

func TestEffectsHTTPAdapter(t *testing.T) {
	sys := effects.NewMemorySystem()

	// Create an HTTP adapter with a mock caller
	adapter := &EffectsHTTPAdapter{
		tool:    &HTTPTool{client: http.DefaultClient},
		effects: sys,
		caller: func(ctx context.Context, req effects.HTTPRequest) (effects.HTTPResponse, error) {
			return effects.HTTPResponse{
				StatusCode: 200,
				Body:       []byte(`{"message":"hello"}`),
			}, nil
		},
	}

	result, err := adapter.Execute(context.Background(), map[string]any{
		"method": "GET",
		"url":    "http://example.com",
	})
	require.NoError(t, err)
	assert.Contains(t, result.Content, "200")
}

func TestEffectsHTTPAdapter_Replay(t *testing.T) {
	sys := effects.NewMemorySystem()

	var realCall bool
	adapter := &EffectsHTTPAdapter{
		tool:    &HTTPTool{},
		effects: sys,
		caller: func(ctx context.Context, req effects.HTTPRequest) (effects.HTTPResponse, error) {
			realCall = true
			return effects.HTTPResponse{
				StatusCode: 200,
				Body:       []byte(`{"message":"hello"}`),
			}, nil
		},
	}

	// First call
	_, err := adapter.Execute(context.Background(), map[string]any{
		"method": "GET",
		"url":    "http://example.com",
	})
	require.NoError(t, err)
	assert.True(t, realCall)

	// Replay - should not call real HTTP again
	ctx := effects.WithReplay(context.Background(), true)
	result, err := adapter.Execute(ctx, map[string]any{
		"method": "GET",
		"url":    "http://example.com",
	})
	require.NoError(t, err)
	assert.Contains(t, result.Content, "200")
	// realCall is still true, but no new call was made
}

func TestEffectsToolRegistry_SetEffectsSystem(t *testing.T) {
	sys1 := effects.NewMemorySystem()
	sys2 := effects.NewMemorySystem()
	registry := NewEffectsToolRegistry(sys1)

	mock := &mockTool{response: "test"}
	registry.Register(mock, func(ctx context.Context, name string, args map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"content": "test"}, nil
	})

	// Update effects system
	registry.SetEffectsSystem(sys2)

	// Should still work with new system
	result, ok := registry.Get("mock_tool")
	require.True(t, ok)
	_, err := result.Execute(context.Background(), map[string]any{"key": "value"})
	require.NoError(t, err)
}
