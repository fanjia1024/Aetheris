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

package llm

import (
	"context"
	"testing"

	"rag-platform/pkg/effects"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockClient is a simple mock for testing.
type mockClient struct {
	generateCalls int
	chatCalls     int
	lastPrompt    string
	lastMessages  []Message
	lastOptions   GenerateOptions
	response      string
}

func (m *mockClient) Generate(prompt string, options GenerateOptions) (string, error) {
	m.generateCalls++
	m.lastPrompt = prompt
	m.lastOptions = options
	return m.response, nil
}

func (m *mockClient) GenerateWithContext(ctx context.Context, prompt string, options GenerateOptions) (string, error) {
	return m.Generate(prompt, options)
}

func (m *mockClient) Chat(messages []Message, options GenerateOptions) (string, error) {
	m.chatCalls++
	m.lastMessages = messages
	m.lastOptions = options
	return m.response, nil
}

func (m *mockClient) ChatWithContext(ctx context.Context, messages []Message, options GenerateOptions) (string, error) {
	return m.Chat(messages, options)
}

func (m *mockClient) Model() string     { return "test-model" }
func (m *mockClient) Provider() string  { return "test" }
func (m *mockClient) SetModel(model string) {}
func (m *mockClient) SetAPIKey(apiKey string) {}

func TestEffectAdapter_Generate(t *testing.T) {
	mock := &mockClient{response: "Hello, World!"}
	sys := effects.NewMemorySystem()
	adapter := NewEffectAdapter(mock, sys)

	result, err := adapter.Generate("Hello", GenerateOptions{Temperature: 0.7})
	require.NoError(t, err)
	assert.Equal(t, "Hello, World!", result)
	// Note: adapter internally uses ChatWithContext for single prompt
	assert.Equal(t, 1, mock.chatCalls)
	assert.Equal(t, "Hello", mock.lastMessages[0].Content)
}

func TestEffectAdapter_ReplayDoesNotCallClient(t *testing.T) {
	mock := &mockClient{response: "Cached response"}
	sys := effects.NewMemorySystem()
	adapter := NewEffectAdapter(mock, sys)

	// First call - should actually call the client
	_, err := adapter.Generate("test", GenerateOptions{})
	require.NoError(t, err)
	// Note: adapter internally uses ChatWithContext
	assert.Equal(t, 1, mock.chatCalls)

	// Second call in replay mode - should NOT call the client
	ctx := effects.WithReplay(context.Background(), true)
	_, err = adapter.GenerateWithContext(ctx, "test", GenerateOptions{})
	require.NoError(t, err)
	// Still 1 call because replay cached the result
	assert.Equal(t, 1, mock.chatCalls)
}

func TestEffectAdapter_Idempotency(t *testing.T) {
	mock := &mockClient{response: "Same response"}
	sys := effects.NewMemorySystem()
	adapter := NewEffectAdapter(mock, sys)

	// Same prompt should be deduplicated
	result1, err := adapter.Generate("same prompt", GenerateOptions{})
	require.NoError(t, err)
	assert.Equal(t, "Same response", result1)

	result2, err := adapter.Generate("same prompt", GenerateOptions{})
	require.NoError(t, err)
	assert.Equal(t, result1, result2)

	// Should only have called client once due to idempotency
	// Note: adapter internally uses ChatWithContext
	assert.Equal(t, 1, mock.chatCalls)
}

func TestEffectAdapter_Unwrap(t *testing.T) {
	mock := &mockClient{response: "test"}
	sys := effects.NewMemorySystem()
	adapter := NewEffectAdapter(mock, sys)

	unwrapped := adapter.Unwrap()
	assert.Same(t, mock, unwrapped)
}

func TestEffectClient(t *testing.T) {
	var called bool
	sys := effects.NewMemorySystem()
	client := NewEffectClient(
		func(ctx context.Context, req effects.LLMRequest) (effects.LLMResponse, error) {
			called = true
			return effects.LLMResponse{
				Content:    "Response from caller",
				StopReason: "stop",
				Model:      req.Model,
			}, nil
		},
		"gpt-4",
		"custom",
		sys,
	)

	result, err := client.Generate("test", GenerateOptions{})
	require.NoError(t, err)
	assert.Equal(t, "Response from caller", result)
	assert.True(t, called)
	assert.Equal(t, "gpt-4", client.Model())
	assert.Equal(t, "custom", client.Provider())
}

func TestEffectClient_Replay(t *testing.T) {
	sys := effects.NewMemorySystem()
	var callCount int

	client := NewEffectClient(
		func(ctx context.Context, req effects.LLMRequest) (effects.LLMResponse, error) {
			callCount++
			return effects.LLMResponse{
				Content:    "Real response",
				StopReason: "stop",
				Model:      req.Model,
			}, nil
		},
		"test-model",
		"test",
		sys,
	)

	// First call
	_, err := client.Generate("test", GenerateOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)

	// Replay - should not call again
	ctx := effects.WithReplay(context.Background(), true)
	_, err = client.GenerateWithContext(ctx, "test", GenerateOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, callCount) // Still 1 because of replay
}