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

// ToolRequest represents a tool effect request.
type ToolRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
	Timeout   *time.Duration         `json:"timeout,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// ToolResponse represents a tool effect response.
type ToolResponse struct {
	Content    string                 `json:"content"`
	Result     map[string]interface{} `json:"result,omitempty"`
	Error      *ToolError             `json:"error,omitempty"`
	StartedAt  time.Time              `json:"started_at"`
	FinishedAt time.Time              `json:"finished_at"`
}

// ToolError represents a tool execution error.
type ToolError struct {
	Type    string `json:"type"` // "execution", "timeout", "not_found", "invalid_args"
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// ToolCaller is the function type for actual tool calls.
type ToolCaller func(ctx context.Context, name string, args map[string]interface{}) (map[string]interface{}, error)

// ExecuteTool executes a tool effect.
// This is the primary entry point for all tool executions.
func ExecuteTool(ctx context.Context, sys System, name string, args map[string]interface{}, caller ToolCaller) (map[string]interface{}, error) {
	req := ToolRequest{
		Name:      name,
		Arguments: args,
	}

	key := computeToolIdempotencyKey(name, args)

	effect := NewEffect(KindTool, req).
		WithIdempotencyKey(key).
		WithDescription("tool." + name)

	result, err := sys.Execute(ctx, effect)
	if err != nil {
		return nil, err
	}

	if result.Cached {
		// Replay mode - return cached data
		if result.Data != nil {
			return result.Data.(map[string]interface{}), nil
		}
		return nil, nil
	}

	// Real execution
	response, err := caller(ctx, name, args)
	if err != nil {
		return nil, err
	}

	// Store the result data using Complete
	_ = sys.Complete(result.ID, response)

	return response, nil
}

// ExecuteToolWithTimeout executes a tool effect with timeout.
func ExecuteToolWithTimeout(ctx context.Context, sys System, name string, args map[string]interface{}, timeout time.Duration, caller ToolCaller) (map[string]interface{}, error) {
	req := ToolRequest{
		Name:      name,
		Arguments: args,
		Timeout:   &timeout,
	}

	key := computeToolIdempotencyKey(name, args)

	effect := NewEffect(KindTool, req).
		WithIdempotencyKey(key).
		WithDescription("tool." + name)

	result, err := sys.Execute(ctx, effect)
	if err != nil {
		return nil, err
	}

	if result.Cached {
		if result.Data != nil {
			return result.Data.(map[string]interface{}), nil
		}
		return nil, nil
	}

	response, err := caller(ctx, name, args)
	if err != nil {
		return nil, err
	}

	// Store the result data using Complete
	_ = sys.Complete(result.ID, response)

	return response, nil
}

// computeToolIdempotencyKey creates a deterministic key from tool name and arguments.
func computeToolIdempotencyKey(name string, args map[string]interface{}) string {
	keyData := struct {
		Name string
		Args map[string]interface{}
	}{
		Name: name,
		Args: args,
	}

	data, _ := json.Marshal(keyData)
	hash := sha256.Sum256(data)
	return "tool:" + hex.EncodeToString(hash[:])
}

// NewToolRequest creates a new tool request.
func NewToolRequest(name string, args map[string]interface{}) ToolRequest {
	return ToolRequest{
		Name:      name,
		Arguments: args,
	}
}

// WithTimeout sets the timeout for the tool request.
func (r ToolRequest) WithTimeout(timeout time.Duration) ToolRequest {
	r.Timeout = &timeout
	return r
}

// WithMetadata adds metadata to the tool request.
func (r ToolRequest) WithMetadata(key string, value interface{}) ToolRequest {
	if r.Metadata == nil {
		r.Metadata = make(map[string]interface{})
	}
	r.Metadata[key] = value
	return r
}

// ToolEffect creates an effect for tool execution.
func ToolEffect(name string, args map[string]interface{}) Effect {
	return NewEffect(KindTool, ToolRequest{
		Name:      name,
		Arguments: args,
	}).WithIdempotencyKey(computeToolIdempotencyKey(name, args)).
		WithDescription("tool." + name)
}

// RecordToolToRecorder records a tool effect using the EventRecorder.
func RecordToolToRecorder(ctx context.Context, recorder EventRecorder, effectID string, idempotencyKey string, req ToolRequest, response map[string]interface{}, duration time.Duration) error {
	if recorder == nil {
		return ErrNoRecorder
	}
	result := SuccessResult(effectID, KindTool, response, duration)
	return recorder.RecordTool(ctx, effectID, idempotencyKey, req, result)
}

// NewToolError creates a new tool error.
func NewToolError(etype, message string, code int) *ToolError {
	return &ToolError{
		Type:    etype,
		Message: message,
		Code:    code,
	}
}
