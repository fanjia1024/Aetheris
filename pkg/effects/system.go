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
	"sync"
)

// System is the core effect execution system.
// It handles effect execution, replay, and idempotency.
type System interface {
	// Execute performs the effect and records it.
	// If the context is in replay mode and the effect has a cached result,
	// it returns the cached result without performing real execution.
	Execute(ctx context.Context, effect Effect) (Result, error)

	// Complete updates a stored result with the execution data.
	// Call this after Execute() to store the actual result data.
	Complete(effectID string, data any) error

	// Replay looks up a result from the history by effect ID.
	Replay(ctx context.Context, effectID string) (Result, bool)

	// History returns all recorded effects (for testing/debugging).
	History() []Result

	// Clear clears the history (for testing).
	Clear()
}

// DefaultSystem is the default system used by global Execute function.
var DefaultSystem System = NewMemorySystem()

// Execute performs an effect using the default system.
func Execute(ctx context.Context, effect Effect) (Result, error) {
	return DefaultSystem.Execute(ctx, effect)
}

// Replay looks up a result from the default system's history.
func Replay(ctx context.Context, effectID string) (Result, bool) {
	return DefaultSystem.Replay(ctx, effectID)
}

// History returns the history from the default system.
func History() []Result {
	return DefaultSystem.History()
}

// Clear clears the default system's history.
func Clear() {
	DefaultSystem.Clear()
}

// RegisterSystem sets a new default system (used for testing).
func RegisterSystem(sys System) {
	DefaultSystem = sys
}

// NewMemorySystem creates an in-memory effect system for testing.
func NewMemorySystem() System {
	return &memorySystem{
		byID:   make(map[string]Result),
		byKey:  make(map[string]Result),
		byIDMu: sync.RWMutex{},
		byKeyMu: sync.RWMutex{},
	}
}