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

package effects

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// LLMRequest represents an LLM effect request.
type LLMRequest struct {
	Model        string                 `json:"model"`
	Messages     []LLMMessage           `json:"messages"`
	Params       LLMParams              `json:"params,omitempty"`
	SystemPrompt string                 `json:"system_prompt,omitempty"`
	Stream       bool                   `json:"stream,omitempty"`
	Extra        map[string]interface{} `json:"extra,omitempty"`
}

// LLMMessage represents a single message in an LLM request.
type LLMMessage struct {
	Role    string `json:"role"` // "system", "user", "assistant", "tool"
	Content string `json:"content"`
}

// LLMParams represents generation parameters.
type LLMParams struct {
	Temperature      *float64 `json:"temperature,omitempty"`
	MaxTokens        *int     `json:"max_tokens,omitempty"`
	TopP             *float64 `json:"top_p,omitempty"`
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64 `json:"presence_penalty,omitempty"`
	StopSequences    []string `json:"stop_sequences,omitempty"`
}

// LLMResponse represents an LLM effect response.
type LLMResponse struct {
	Content     string   `json:"content"`
	StopReason  string   `json:"stop_reason"` // "stop", "length", "tool_calls", "content_filter", "null"
	Usage       LLMUsage `json:"usage"`
	Model       string   `json:"model"`
	RawResponse any      `json:"raw_response,omitempty"`
}

// LLMUsage represents token usage.
type LLMUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// LLMCaller is the function type for actual LLM calls.
type LLMCaller func(ctx context.Context, req LLMRequest) (LLMResponse, error)

// ExecuteLLM executes an LLM effect.
// If the context is in replay mode, it returns the cached result.
func ExecuteLLM(ctx context.Context, sys System, req LLMRequest, caller LLMCaller) (LLMResponse, error) {
	// Compute idempotency key from request
	key := computeLLMIdempotencyKey(req)

	effect := NewEffect(KindLLM, req).
		WithIdempotencyKey(key).
		WithDescription("llm." + req.Model)

	result, err := sys.Execute(ctx, effect)
	if err != nil {
		return LLMResponse{}, err
	}

	if result.Cached {
		// Replay mode - return cached data
		if result.Data != nil {
			return result.Data.(LLMResponse), nil
		}
		return LLMResponse{}, nil
	}

	// Real execution
	response, err := caller(ctx, req)
	if err != nil {
		return LLMResponse{}, err
	}

	// Store the result data using Complete
	_ = sys.Complete(result.ID, response)

	return response, nil
}

// computeLLMIdempotencyKey creates a deterministic key from the LLM request.
func computeLLMIdempotencyKey(req LLMRequest) string {
	// Create a canonical representation for hashing
	canonical := struct {
		Model        string
		Messages     []LLMMessage
		SystemPrompt string
		Params       LLMParams
	}{
		Model:        req.Model,
		Messages:     req.Messages,
		SystemPrompt: req.SystemPrompt,
		Params:       req.Params,
	}

	data, _ := json.Marshal(canonical)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// NewLLMRequest creates a new LLM request with the given parameters.
func NewLLMRequest(model, prompt string) LLMRequest {
	return LLMRequest{
		Model: model,
		Messages: []LLMMessage{
			{Role: "user", Content: prompt},
		},
	}
}

// WithSystemMessage adds a system message to the request.
func (r LLMRequest) WithSystemMessage(msg string) LLMRequest {
	r.SystemPrompt = msg
	return r
}

// WithTemperature sets the temperature parameter.
func (r LLMRequest) WithTemperature(temp float64) LLMRequest {
	r.Params.Temperature = &temp
	return r
}

// WithMaxTokens sets the max tokens parameter.
func (r LLMRequest) WithMaxTokens(tokens int) LLMRequest {
	r.Params.MaxTokens = &tokens
	return r
}

// AddMessage adds a message to the messages list.
func (r LLMRequest) AddMessage(role, content string) LLMRequest {
	r.Messages = append(r.Messages, LLMMessage{Role: role, Content: content})
	return r
}

// LLMEffect creates an effect for LLM execution.
func LLMEffect(req LLMRequest) Effect {
	return NewEffect(KindLLM, req).
		WithIdempotencyKey(computeLLMIdempotencyKey(req)).
		WithDescription("llm." + req.Model)
}

// ToLLMResponse converts a Result to LLMResponse.
func ToLLMResponse(result Result) (LLMResponse, bool) {
	resp, ok := result.Data.(LLMResponse)
	return resp, ok
}

// RecordLLMToRecorder records an LLM effect using the EventRecorder.
func RecordLLMToRecorder(ctx context.Context, recorder EventRecorder, effectID string, req LLMRequest, resp LLMResponse, duration time.Duration) error {
	if recorder == nil {
		return ErrNoRecorder
	}
	result := SuccessResult(effectID, KindLLM, resp, duration)
	return recorder.RecordLLM(ctx, effectID, req, result)
}
