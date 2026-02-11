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

// TimeResult represents the recorded time value.
type TimeResult struct {
	Timestamp time.Time `json:"timestamp"`
	Timezone  string    `json:"timezone"`
	UnixNano  int64     `json:"unix_nano"`
}

// ExecuteTime records the current time as an effect.
func ExecuteTime(ctx context.Context, sys System) (TimeResult, error) {
	now := time.Now()
	result := TimeResult{
		Timestamp: now,
		UnixNano:  now.UnixNano(),
	}

	effect := NewEffect(KindTime, result).
		WithDescription("time.now")

	res, err := sys.Execute(ctx, effect)
	if err != nil {
		return TimeResult{}, err
	}

	if res.Cached && res.Data != nil {
		return res.Data.(TimeResult), nil
	}

	// Update with recorded time
	res.Data = result
	return result, nil
}

// TimeEffect creates an effect for time recording.
func TimeEffect(t time.Time) Effect {
	result := TimeResult{
		Timestamp: t,
		UnixNano:  t.UnixNano(),
	}
	return NewEffect(KindTime, result).WithDescription("time.now")
}

// RecordTimeToRecorder records a time effect using the EventRecorder.
func RecordTimeToRecorder(ctx context.Context, recorder EventRecorder, effectID string, t time.Time) error {
	if recorder == nil {
		return ErrNoRecorder
	}
	return recorder.RecordTime(ctx, effectID, t)
}
