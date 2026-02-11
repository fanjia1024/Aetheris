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
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Effect represents a side effect operation that must be recorded for replay.
type Effect struct {
	// ID is the unique identifier of this effect. Auto-generated if empty.
	ID string `json:"id"`

	// Kind is the type of effect (LLM, Tool, HTTP, etc.)
	Kind Kind `json:"kind"`

	// Payload contains the operation parameters (LLMRequest, ToolCall, etc.)
	Payload any `json:"payload,omitempty"`

	// IdempotencyKey is used for deduplication. Same key returns same result.
	// Required for Tool and HTTP effects.
	IdempotencyKey string `json:"idempotency_key,omitempty"`

	// Description is for debugging and tracing purposes.
	Description string `json:"description,omitempty"`

	// JobID is the job this effect belongs to (used for event stream recording).
	JobID string `json:"job_id,omitempty"`

	// AttemptID is the execution attempt (for validation).
	AttemptID string `json:"attempt_id,omitempty"`
}

// Result represents the outcome of an effect execution.
type Result struct {
	// ID matches the Effect.ID
	ID string `json:"id"`

	// Kind matches the Effect.Kind
	Kind Kind `json:"kind"`

	// Data contains the operation result (LLMResponse, ToolOutput, etc.)
	Data any `json:"data,omitempty"`

	// Error is set if the operation failed
	Error *Error `json:"error,omitempty"`

	// Timestamp when the effect was originally executed.
	Timestamp time.Time `json:"timestamp"`

	// Duration of the effect execution in milliseconds.
	DurationMs int64 `json:"duration_ms"`

	// Cached indicates this result was replayed from history, not freshly executed.
	Cached bool `json:"cached"`

	// ReplayFromID is the ID of the original effect this was replayed from.
	ReplayFromID string `json:"replay_from_id,omitempty"`
}

// Error represents an effect execution error.
type Error struct {
	Type    string `json:"type"`     // "llm", "tool", "http", "timeout", "internal"
	Message string `json:"message"`  // User-facing error message
	Code    int    `json:"code,omitempty"` // HTTP status code, tool exit code, etc.
	Retriable bool `json:"retriable"` // Whether this error should trigger a retry
}

// NewEffect creates a new effect with auto-generated ID.
func NewEffect(kind Kind, payload any) Effect {
	return Effect{
		ID:      uuid.New().String(),
		Kind:    kind,
		Payload: payload,
	}
}

// WithIdempotencyKey sets the idempotency key.
func (e Effect) WithIdempotencyKey(key string) Effect {
	e.IdempotencyKey = key
	return e
}

// WithDescription sets the description.
func (e Effect) WithDescription(desc string) Effect {
	e.Description = desc
	return e
}

// WithJobID sets the job ID for event stream recording.
func (e Effect) WithJobID(jobID string) Effect {
	e.JobID = jobID
	return e
}

// WithAttemptID sets the attempt ID for validation.
func (e Effect) WithAttemptID(attemptID string) Effect {
	e.AttemptID = attemptID
	return e
}

// SuccessResult creates a successful result.
func SuccessResult(id string, kind Kind, data any, duration time.Duration) Result {
	return Result{
		ID:         id,
		Kind:       kind,
		Data:       data,
		Timestamp:  time.Now(),
		DurationMs: duration.Milliseconds(),
	}
}

// FailedResult creates a failed result.
func FailedResult(id string, kind Kind, err Error, duration time.Duration) Result {
	return Result{
		ID:         id,
		Kind:       kind,
		Error:      &err,
		Timestamp:  time.Now(),
		DurationMs: duration.Milliseconds(),
	}
}

// CachedResult wraps a result as cached (replay from history).
func CachedResult(originalID string, result Result) Result {
	result.Cached = true
	result.ReplayFromID = originalID
	result.Timestamp = time.Now()
	return result
}

// MarshalPayload serializes the effect payload to JSON bytes.
func (e Effect) MarshalPayload() ([]byte, error) {
	if e.Payload == nil {
		return []byte("null"), nil
	}
	return json.Marshal(e.Payload)
}