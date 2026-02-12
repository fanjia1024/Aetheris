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
	"hash/fnv"
	"math/rand"
	"time"
)

// contextKey is a private type for context keys to avoid collisions.
type contextKey int

const (
	ctxKeyClock contextKey = iota
	ctxKeyRNG
)

// ClockFunc returns the current time. When injected by the Runner during replay,
// it returns a deterministic value so step execution is replay-safe.
type ClockFunc func() time.Time

// RNGFunc returns a random int in [0, n). When injected by the Runner during replay,
// it uses a deterministic seed (jobID + stepID) so step execution is replay-safe.
type RNGFunc func(n int) int

// WithClock attaches a clock function to ctx. Steps should use Clock(ctx) instead of time.Now().
func WithClock(ctx context.Context, fn ClockFunc) context.Context {
	if fn == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyClock, fn)
}

// WithRNG attaches a deterministic RNG function to ctx. Steps should use RandIntn(ctx, n) instead of rand.Intn(n).
func WithRNG(ctx context.Context, fn RNGFunc) context.Context {
	if fn == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyRNG, fn)
}

// Clock returns the current time from the context-injected clock if set.
// If not set, it falls back to time.Now(). Using time.Now() directly in steps
// breaks deterministic replay; prefer the Runner to inject a clock via WithClock.
func Clock(ctx context.Context) time.Time {
	if v := ctx.Value(ctxKeyClock); v != nil {
		if fn, ok := v.(ClockFunc); ok {
			return fn()
		}
	}
	return time.Now()
}

// RandIntn returns a random int in [0, n) from the context-injected RNG if set.
// If not set, it falls back to rand.Intn(n). Using rand.* directly in steps
// breaks deterministic replay; prefer the Runner to inject an RNG via WithRNG.
func RandIntn(ctx context.Context, n int) int {
	if n <= 0 {
		return 0
	}
	if v := ctx.Value(ctxKeyRNG); v != nil {
		if fn, ok := v.(RNGFunc); ok {
			return fn(n)
		}
	}
	return rand.Intn(n)
}

// ReplayClock returns a ClockFunc that always returns a deterministic time derived from jobID and stepID.
// Used by the Runner when replayCtx != nil so replay is deterministic.
func ReplayClock(jobID, stepID string) ClockFunc {
	h := fnv.New64a()
	h.Write([]byte(jobID))
	h.Write([]byte(stepID))
	ns := h.Sum64()
	// Use a fixed epoch plus hash-derived nanoseconds so same job+step always gets same time.
	return func() time.Time {
		return time.Unix(0, int64(ns)).UTC()
	}
}

// ReplayRNG returns an RNGFunc that is deterministic for the given jobID and stepID.
// Used by the Runner when replayCtx != nil so replay is deterministic.
func ReplayRNG(jobID, stepID string) RNGFunc {
	h := fnv.New64a()
	h.Write([]byte(jobID))
	h.Write([]byte(stepID))
	seed := int64(h.Sum64())
	r := rand.New(rand.NewSource(seed))
	return func(n int) int {
		if n <= 0 {
			return 0
		}
		return r.Intn(n)
	}
}
