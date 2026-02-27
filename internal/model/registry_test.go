// Copyright 2026 fanjia1024
// Tests for model registry

package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetLLM_NotRegistered(t *testing.T) {
	// Get non-existent LLM
	_, err := GetLLM("non-existent-llm")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestGetEmbedding_NotRegistered(t *testing.T) {
	// Get non-existent Embedding
	_, err := GetEmbedding("non-existent-embedding")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestGetVision_NotRegistered(t *testing.T) {
	// Get non-existent Vision
	_, err := GetVision("non-existent-vision")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}
