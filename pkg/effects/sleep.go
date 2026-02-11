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

// SleepResult represents the recorded sleep duration.
type SleepResult struct {
	DurationMs  int64         `json:"duration_ms"`
	Duration    time.Duration `json:"duration"`
	CompletedAt time.Time     `json:"completed_at"`
}

// ExecuteSleep executes a sleep effect.
func ExecuteSleep(ctx context.Context, sys System, duration time.Duration) (SleepResult, error) {
	result := SleepResult{
		DurationMs:  duration.Milliseconds(),
		Duration:    duration,
		CompletedAt: time.Now(),
	}

	effect := NewEffect(KindSleep, result).
		WithDescription("sleep." + duration.String())

	res, err := sys.Execute(ctx, effect)
	if err != nil {
		return SleepResult{}, err
	}

	if res.Cached && res.Data != nil {
		return res.Data.(SleepResult), nil
	}

	// Record completion but don't actually sleep during replay
	res.Data = result
	return result, nil
}

// SleepEffect creates an effect for sleep.
func SleepEffect(duration time.Duration) Effect {
	result := SleepResult{
		DurationMs: duration.Milliseconds(),
		Duration:   duration,
	}
	return NewEffect(KindSleep, result).WithDescription("sleep." + duration.String())
}

// RecordSleepToRecorder records a sleep effect using the EventRecorder.
func RecordSleepToRecorder(ctx context.Context, recorder EventRecorder, effectID string, durationMs int64) error {
	if recorder == nil {
		return ErrNoRecorder
	}
	return recorder.RecordSleep(ctx, effectID, durationMs)
}
