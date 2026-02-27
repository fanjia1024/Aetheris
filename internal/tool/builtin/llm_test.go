// Copyright 2026 fanjia1024
// Tests for LLM tool

package builtin

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockGenerator is a mock implementation of PromptGenerator
type mockGenerator struct {
	response string
	err      error
}

func (m *mockGenerator) Generate(ctx context.Context, prompt string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func TestLLMGenerateTool_Name(t *testing.T) {
	tool := NewLLMGenerateTool(&mockGenerator{response: "test"})
	assert.Equal(t, "llm.generate", tool.Name())
}

func TestLLMGenerateTool_Description(t *testing.T) {
	tool := NewLLMGenerateTool(&mockGenerator{response: "test"})
	assert.NotEmpty(t, tool.Description())
}

func TestLLMGenerateTool_Schema(t *testing.T) {
	tool := NewLLMGenerateTool(&mockGenerator{response: "test"})
	schema := tool.Schema()
	assert.Equal(t, "object", schema.Type)
	assert.Contains(t, schema.Properties, "prompt")
	assert.Contains(t, schema.Required, "prompt")
}

func TestLLMGenerateTool_NotConfigured(t *testing.T) {
	tool := NewLLMGenerateTool(nil)
	result, err := tool.Execute(context.Background(), map[string]any{
		"prompt": "hello",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result.Err)
	assert.Contains(t, result.Err, "generator not configured")
}

func TestLLMGenerateTool_MissingPrompt(t *testing.T) {
	tool := NewLLMGenerateTool(&mockGenerator{response: "test"})
	result, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	assert.NotEmpty(t, result.Err)
	assert.Contains(t, result.Err, "prompt is required")
}

func TestLLMGenerateTool_Success(t *testing.T) {
	mock := &mockGenerator{response: "Generated text"}
	tool := NewLLMGenerateTool(mock)

	result, err := tool.Execute(context.Background(), map[string]any{
		"prompt": "Generate something",
	})
	require.NoError(t, err)
	assert.Empty(t, result.Err)
	assert.Equal(t, "Generated text", result.Content)
}

func TestLLMGenerateTool_GeneratorError(t *testing.T) {
	mock := &mockGenerator{err: errors.New("API error")}
	tool := NewLLMGenerateTool(mock)

	result, err := tool.Execute(context.Background(), map[string]any{
		"prompt": "Generate something",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result.Err)
	assert.Contains(t, result.Err, "API error")
}

func TestLLMGenerateTool_EmptyPrompt(t *testing.T) {
	tool := NewLLMGenerateTool(&mockGenerator{response: "test"})
	result, err := tool.Execute(context.Background(), map[string]any{
		"prompt": "",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result.Err)
	assert.Contains(t, result.Err, "prompt is required")
}
