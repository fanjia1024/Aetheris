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
	"time"
)

type contextKey string

const (
	// replayingKey indicates the context is in replay mode.
	// When true, Execute() should not perform real effect calls.
	replayingKey contextKey = "effects.replaying"

	// effectIDKey is used to pass effect ID through context for nested calls.
	effectIDKey contextKey = "effects.id"

	// eventRecorderKey holds the event recorder for this context.
	eventRecorderKey contextKey = "effects.recorder"

	// effectSystemKey holds the effect system for this context.
	effectSystemKey contextKey = "effects.system"
)

// WithReplay sets the replay mode flag in the context.
func WithReplay(ctx context.Context, replaying bool) context.Context {
	return context.WithValue(ctx, replayingKey, replaying)
}

// IsReplaying returns true if the context is in replay mode.
func IsReplaying(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	if v := ctx.Value(replayingKey); v != nil {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// WithEffectID sets the current effect ID in the context.
func WithEffectID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, effectIDKey, id)
}

// EffectIDFromContext returns the current effect ID from context.
func EffectIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v := ctx.Value(effectIDKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// EventRecorder records effect events to the event stream.
type EventRecorder interface {
	// RecordLLM records an LLM invocation.
	RecordLLM(ctx context.Context, effectID string, req any, result Result) error

	// RecordTool records a tool invocation.
	RecordTool(ctx context.Context, effectID string, idempotencyKey string, req any, result Result) error

	// RecordHTTP records an HTTP request.
	RecordHTTP(ctx context.Context, effectID string, idempotencyKey string, req any, result Result) error

	// RecordTime records a time value.
	RecordTime(ctx context.Context, effectID string, t time.Time) error

	// RecordRandom records random values.
	RecordRandom(ctx context.Context, effectID string, source string, values []byte) error

	// RecordSleep records a sleep duration.
	RecordSleep(ctx context.Context, effectID string, durationMs int64) error
}

// WithEventRecorder sets the event recorder in the context.
func WithEventRecorder(ctx context.Context, recorder EventRecorder) context.Context {
	return context.WithValue(ctx, eventRecorderKey, recorder)
}

// EventRecorderFromContext returns the event recorder from context.
func EventRecorderFromContext(ctx context.Context) EventRecorder {
	if ctx == nil {
		return nil
	}
	if v := ctx.Value(eventRecorderKey); v != nil {
		if r, ok := v.(EventRecorder); ok {
			return r
		}
	}
	return nil
}

// EffectSystem is the effect system interface.
type EffectSystem interface {
	// Execute performs the effect and records it.
	Execute(ctx context.Context, effect Effect) (Result, error)

	// Replay looks up a result from the history.
	Replay(ctx context.Context, effectID string) (Result, bool)

	// History returns all recorded effects (for testing).
	History() []Result

	// Clear clears the history (for testing).
	Clear()
}

// WithSystem sets the effect system in the context.
func WithSystem(ctx context.Context, sys EffectSystem) context.Context {
	return context.WithValue(ctx, effectSystemKey, sys)
}

// SystemFromContext returns the effect system from context.
func SystemFromContext(ctx context.Context) EffectSystem {
	if ctx == nil {
		return nil
	}
	if v := ctx.Value(effectSystemKey); v != nil {
		if s, ok := v.(EffectSystem); ok {
			return s
		}
	}
	return nil
}