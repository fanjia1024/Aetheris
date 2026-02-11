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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemorySystem_Execute(t *testing.T) {
	sys := NewMemorySystem()
	ctx := context.Background()

	effect := NewEffect(KindTool, map[string]any{
		"tool": "test_tool",
		"args": map[string]any{"x": 1},
	}).WithIdempotencyKey("test-key-1")

	result, err := sys.Execute(ctx, effect)
	require.NoError(t, err)
	assert.NotEmpty(t, result.ID)
	assert.Equal(t, KindTool, result.Kind)
	assert.False(t, result.Cached)
	assert.NotZero(t, result.Timestamp)
}

func TestMemorySystem_Idempotency(t *testing.T) {
	sys := NewMemorySystem()
	ctx := context.Background()

	// First execution
	key := uuid.New().String()
	effect1 := NewEffect(KindTool, "test").WithIdempotencyKey(key)
	result1, err := sys.Execute(ctx, effect1)
	require.NoError(t, err)

	// Second execution with same key
	effect2 := NewEffect(KindTool, "test").WithIdempotencyKey(key)
	result2, err := sys.Execute(ctx, effect2)
	require.NoError(t, err)

	// Should return same result
	assert.Equal(t, result1.ID, result2.ID)
	assert.True(t, result2.Cached)
}

func TestMemorySystem_Replay(t *testing.T) {
	sys := NewMemorySystem()
	ctx := context.Background()

	// Execute an effect
	effect := NewEffect(KindLLM, "test prompt").WithDescription("test llm")
	result1, err := sys.Execute(ctx, effect)
	require.NoError(t, err)

	// Replay from history
	result2, ok := sys.Replay(ctx, result1.ID)
	assert.True(t, ok)
	assert.Equal(t, result1.ID, result2.ID)
	assert.Equal(t, result1.Kind, result2.Kind)
}

func TestMemorySystem_ReplayMode_Forbidden(t *testing.T) {
	sys := NewMemorySystem()
	ctx := context.Background()

	// Execute an effect first to have it in history
	effect := NewEffect(KindTool, "test").WithDescription("test")
	_, err := sys.Execute(ctx, effect)
	require.NoError(t, err)

	// Try to execute a different effect in replay mode without history
	ctxReplay := WithReplay(ctx, true)
	newEffect := NewEffect(KindTool, "new test").WithIdempotencyKey("new-key")

	_, err = sys.Execute(ctxReplay, newEffect)
	assert.ErrorIs(t, err, ErrReplayingForbidden)
}

func TestMemorySystem_ReplayMode_Cached(t *testing.T) {
	sys := NewMemorySystem()
	ctx := context.Background()

	// Execute an effect first
	effect := NewEffect(KindHTTP, map[string]any{
		"method": "GET",
		"url":    "http://example.com",
	}).WithIdempotencyKey("http-key")

	originalResult := Result{
		ID:        effect.ID,
		Kind:      KindHTTP,
		Data:      map[string]any{"status": 200, "body": "hello"},
		Timestamp: time.Now(),
	}

	// Use Execute to populate history
	_, err := sys.Execute(ctx, effect)
	require.NoError(t, err)

	// Replay should return cached result
	ctxReplay := WithReplay(ctx, true)
	result, err := sys.Execute(ctxReplay, effect)
	require.NoError(t, err)
	assert.True(t, result.Cached)
	assert.Equal(t, originalResult.ID, result.ReplayFromID)
}

func TestMemorySystem_History(t *testing.T) {
	sys := NewMemorySystem()
	ctx := context.Background()

	// Execute multiple effects
	for i := 0; i < 3; i++ {
		effect := NewEffect(KindLLM, "prompt").WithIdempotencyKey(uuid.New().String())
		_, err := sys.Execute(ctx, effect)
		require.NoError(t, err)
	}

	history := sys.History()
	assert.Len(t, history, 3)
}

func TestMemorySystem_Clear(t *testing.T) {
	sys := NewMemorySystem()
	ctx := context.Background()

	effect := NewEffect(KindTool, "test").WithIdempotencyKey("key")
	_, err := sys.Execute(ctx, effect)
	require.NoError(t, err)

	assert.Len(t, sys.History(), 1)

	sys.Clear()
	assert.Len(t, sys.History(), 0)
}

func TestEffect_Builder(t *testing.T) {
	effect := NewEffect(KindLLM, map[string]any{
		"model":  "gpt-4",
		"prompt": "hello",
	}).WithIdempotencyKey("key-123").
		WithDescription("test LLM call").
		WithJobID("job-1").
		WithAttemptID("attempt-1")

	assert.Equal(t, KindLLM, effect.Kind)
	assert.Equal(t, "key-123", effect.IdempotencyKey)
	assert.Equal(t, "test LLM call", effect.Description)
	assert.Equal(t, "job-1", effect.JobID)
	assert.Equal(t, "attempt-1", effect.AttemptID)
}

func TestSuccessResult(t *testing.T) {
	result := SuccessResult("id-1", KindLLM, map[string]any{"text": "hello"}, 100*time.Millisecond)

	assert.Equal(t, "id-1", result.ID)
	assert.Equal(t, KindLLM, result.Kind)
	assert.NotNil(t, result.Data)
	assert.Nil(t, result.Error)
	assert.Equal(t, int64(100), result.DurationMs)
	assert.False(t, result.Cached)
}

func TestFailedResult(t *testing.T) {
	err := Error{
		Type:    "llm",
		Message: "rate limited",
		Code:    429,
	}
	result := FailedResult("id-1", KindLLM, err, 50*time.Millisecond)

	assert.Equal(t, "id-1", result.ID)
	assert.Equal(t, KindLLM, result.Kind)
	assert.Nil(t, result.Data)
	assert.NotNil(t, result.Error)
	assert.Equal(t, "rate limited", result.Error.Message)
	assert.Equal(t, 429, result.Error.Code)
}

func TestContextHelpers(t *testing.T) {
	ctx := context.Background()

	// IsReplaying
	assert.False(t, IsReplaying(ctx))
	ctx = WithReplay(ctx, true)
	assert.True(t, IsReplaying(ctx))

	// EffectID
	assert.Empty(t, EffectIDFromContext(ctx))
	ctx = WithEffectID(ctx, "effect-123")
	assert.Equal(t, "effect-123", EffectIDFromContext(ctx))
}

func TestContextWithSystem(t *testing.T) {
	sys := NewMemorySystem()
	ctx := WithSystem(context.Background(), sys)

	// SystemFromContext
	assert.Equal(t, sys, SystemFromContext(ctx))
	assert.Nil(t, SystemFromContext(context.Background()))
}

func TestContextWithEventRecorder(t *testing.T) {
	// Create a no-op recorder for testing
	recorder := &nopRecorder{}
	ctx := WithEventRecorder(context.Background(), recorder)

	assert.Equal(t, recorder, EventRecorderFromContext(ctx))
	assert.Nil(t, EventRecorderFromContext(context.Background()))
}

type nopRecorder struct{}

func (r *nopRecorder) RecordLLM(ctx context.Context, effectID string, req any, result Result) error {
	return nil
}

func (r *nopRecorder) RecordTool(ctx context.Context, effectID string, idempotencyKey string, req any, result Result) error {
	return nil
}

func (r *nopRecorder) RecordHTTP(ctx context.Context, effectID string, idempotencyKey string, req any, result Result) error {
	return nil
}

func (r *nopRecorder) RecordTime(ctx context.Context, effectID string, t time.Time) error {
	return nil
}

func (r *nopRecorder) RecordRandom(ctx context.Context, effectID string, source string, values []byte) error {
	return nil
}

func (r *nopRecorder) RecordSleep(ctx context.Context, effectID string, durationMs int64) error {
	return nil
}
