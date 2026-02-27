// Copyright 2026 fanjia1024
// Tests for Workflow tool

package builtin

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkflowTool_Name(t *testing.T) {
	tool := NewWorkflowTool(nil)
	assert.Equal(t, "workflow.run", tool.Name())
}

func TestWorkflowTool_Description(t *testing.T) {
	tool := NewWorkflowTool(nil)
	assert.NotEmpty(t, tool.Description())
}

func TestWorkflowTool_Schema(t *testing.T) {
	tool := NewWorkflowTool(nil)
	schema := tool.Schema()
	assert.Equal(t, "object", schema.Type)
	assert.Contains(t, schema.Properties, "name")
	assert.Contains(t, schema.Properties, "params")
	assert.Contains(t, schema.Required, "name")
}

func TestWorkflowTool_NotConfigured(t *testing.T) {
	tool := NewWorkflowTool(nil)
	result, err := tool.Execute(context.Background(), map[string]any{
		"name": "test-workflow",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result.Err)
	assert.Contains(t, result.Err, "engine not configured")
}

func TestWorkflowTool_MissingName(t *testing.T) {
	tool := NewWorkflowTool(nil)

	result, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	// When engine is nil, it returns "engine not configured" first
	assert.NotEmpty(t, result.Err)
}

func TestWorkflowTool_EmptyName(t *testing.T) {
	tool := NewWorkflowTool(nil)

	result, err := tool.Execute(context.Background(), map[string]any{
		"name": "",
	})
	require.NoError(t, err)
	// When engine is nil, it returns "engine not configured" first
	assert.NotEmpty(t, result.Err)
}
