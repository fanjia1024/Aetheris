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

package runtime

import (
	"context"
	"testing"
)

// TestClock_ReplayDeterministic verifies that when ReplayClock is injected, Clock(ctx) returns
// the same value for the same jobID+stepID (Step Contract: deterministic replay).
func TestClock_ReplayDeterministic(t *testing.T) {
	jobID, stepID := "job-1", "step-1"
	fn := ReplayClock(jobID, stepID)
	t1 := fn()
	t2 := fn()
	if !t1.Equal(t2) {
		t.Errorf("ReplayClock must be deterministic: got %v and %v", t1, t2)
	}
	ctx := WithClock(context.Background(), fn)
	if got := Clock(ctx); !got.Equal(t1) {
		t.Errorf("Clock(ctx) with injected ReplayClock: got %v, want %v", got, t1)
	}
}

// TestRandIntn_ReplayDeterministic verifies that when ReplayRNG is injected, RandIntn(ctx, n)
// returns the same sequence for the same jobID+stepID (Step Contract: deterministic replay).
func TestRandIntn_ReplayDeterministic(t *testing.T) {
	jobID, stepID := "job-1", "step-1"
	fn := ReplayRNG(jobID, stepID)
	ctx := WithRNG(context.Background(), fn)
	// First 5 values must be identical on two contexts with same seed
	ctx2 := WithRNG(context.Background(), ReplayRNG(jobID, stepID))
	n := 100
	for i := 0; i < 5; i++ {
		a, b := RandIntn(ctx, n), RandIntn(ctx2, n)
		if a != b {
			t.Errorf("RandIntn replay deterministic: step %d got %d vs %d", i, a, b)
		}
	}
}

// TestClock_WithoutInjection falls back to time.Now() when no clock is injected.
// (Replay may be non-deterministic if steps use Clock(ctx) without Runner injection.)
func TestClock_WithoutInjection(t *testing.T) {
	ctx := context.Background()
	t1 := Clock(ctx)
	t2 := Clock(ctx)
	// Both should be real time; we only check they are reasonable (not zero)
	if t1.IsZero() || t2.IsZero() {
		t.Error("Clock(ctx) without injection should return non-zero time")
	}
}

// TestRandIntn_WithoutInjection falls back to rand.Intn when no RNG is injected.
func TestRandIntn_WithoutInjection(t *testing.T) {
	ctx := context.Background()
	v := RandIntn(ctx, 10)
	if v < 0 || v >= 10 {
		t.Errorf("RandIntn(ctx, 10) without injection: got %d", v)
	}
}
