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
	"errors"
	"fmt"
)

// Effect execution errors.
var (
	// ErrReplayingForbidden is returned when an effect is called during replay.
	ErrReplayingForbidden = errors.New("effects: real execution forbidden during replay")

	// ErrNotFound is returned when an effect ID is not found in history.
	ErrNotFound = errors.New("effects: effect not found in history")

	// ErrAlreadyExists is returned when an idempotency key already exists.
	ErrAlreadyExists = errors.New("effects: idempotency key already exists")

	// ErrNoRecorder is returned when no event recorder is configured.
	ErrNoRecorder = errors.New("effects: no event recorder configured")

	// ErrNoSystem is returned when no effect system is configured.
	ErrNoSystem = errors.New("effects: no effect system configured")
)

// Error messages for common effect types.
var (
	// LLM errors
	ErrLLMGenerationFailed = Error{
		Type:    "llm",
		Message: "LLM generation failed",
		Retriable: true,
	}
	ErrLLMTimeout = Error{
		Type:    "llm",
		Message: "LLM request timed out",
		Code:    408,
		Retriable: true,
	}
	ErrLLMRateLimited = Error{
		Type:    "llm",
		Message: "LLM rate limit exceeded",
		Code:    429,
		Retriable: true,
	}

	// Tool errors
	ErrToolExecutionFailed = Error{
		Type:    "tool",
		Message: "tool execution failed",
		Retriable: true,
	}
	ErrToolNotFound = Error{
		Type:    "tool",
		Message: "tool not found",
		Code:    404,
		Retriable: false,
	}
	ErrToolTimeout = Error{
		Type:    "tool",
		Message: "tool execution timed out",
		Code:    408,
		Retriable: true,
	}

	// HTTP errors
	ErrHTTPRequestFailed = Error{
		Type:    "http",
		Message: "HTTP request failed",
		Retriable: true,
	}
	ErrHTTPNotFound = Error{
		Type:    "http",
		Message: "HTTP resource not found",
		Code:    404,
		Retriable: false,
	}
	ErrHTTPServerError = Error{
		Type:    "http",
		Message: "HTTP server error",
		Code:    500,
		Retriable: true,
	}
)

// NewError creates a new effect error with format.
func NewError(kind string, format string, args ...any) Error {
	return Error{
		Type:    kind,
		Message: fmt.Sprintf(format, args...),
	}
}