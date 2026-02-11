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
	"math/rand"
	"sync"
	"time"
)

// memorySystem is an in-memory implementation of the effect system.
// Used for testing and local development.
type memorySystem struct {
	byID   map[string]Result // effect ID -> result
	byKey  map[string]Result // idempotency key -> result
	byIDMu sync.RWMutex
	byKeyMu sync.RWMutex
}

func (s *memorySystem) Execute(ctx context.Context, effect Effect) (Result, error) {
	// Handle replay mode
	if IsReplaying(ctx) {
		// Try to find in history by ID first
		if effect.ID != "" {
			if result, ok := s.Replay(ctx, effect.ID); ok {
				return CachedResult(effect.ID, result), nil
			}
		}
		// Try by idempotency key
		if effect.IdempotencyKey != "" {
			s.byKeyMu.RLock()
			result, ok := s.byKey[effect.IdempotencyKey]
			s.byKeyMu.RUnlock()
			if ok {
				return CachedResult("", result), nil
			}
		}
		// Not found in history during replay - this is an error
		return Result{}, ErrReplayingForbidden
	}

	// Normal execution - generate ID if not set
	if effect.ID == "" {
		effect.ID = NewID()
	}

	start := time.Now()
	result := Result{
		ID:        effect.ID,
		Kind:      effect.Kind,
		Timestamp: start,
	}

	// Check idempotency for Tool and HTTP effects
	if effect.IdempotencyKey != "" {
		s.byKeyMu.RLock()
		existing, ok := s.byKey[effect.IdempotencyKey]
		s.byKeyMu.RUnlock()
		if ok {
			// Return cached result
			cached := CachedResult(existing.ID, existing)
			cached.DurationMs = time.Since(start).Milliseconds()
			return cached, nil
		}
	}

	// Store result
	s.byIDMu.Lock()
	s.byID[effect.ID] = result
	s.byIDMu.Unlock()

	if effect.IdempotencyKey != "" {
		s.byKeyMu.Lock()
		s.byKey[effect.IdempotencyKey] = result
		s.byKeyMu.Unlock()
	}

	return result, nil
}

// Complete updates a result in the history with the final data.
// This should be called by the caller after Execute() to store the result.
// It updates both byID and byKey maps.
func (s *memorySystem) Complete(id string, data any) error {
	s.byIDMu.Lock()
	s.byKeyMu.Lock()
	defer s.byIDMu.Unlock()
	defer s.byKeyMu.Unlock()

	// Update byID
	if existing, ok := s.byID[id]; ok {
		existing.Data = data
		s.byID[id] = existing
	}

	// Also update any byKey entries that reference this effect ID
	for key, result := range s.byKey {
		if result.ID == id {
			result.Data = data
			s.byKey[key] = result
		}
	}

	return nil
}

func (s *memorySystem) Replay(ctx context.Context, effectID string) (Result, bool) {
	s.byIDMu.RLock()
	defer s.byIDMu.RUnlock()
	result, ok := s.byID[effectID]
	return result, ok
}

func (s *memorySystem) History() []Result {
	s.byIDMu.RLock()
	defer s.byIDMu.RUnlock()
	history := make([]Result, 0, len(s.byID))
	for _, r := range s.byID {
		history = append(history, r)
	}
	return history
}

func (s *memorySystem) Clear() {
	s.byIDMu.Lock()
	s.byKeyMu.Lock()
	defer s.byIDMu.Unlock()
	defer s.byKeyMu.Unlock()
	s.byID = make(map[string]Result)
	s.byKey = make(map[string]Result)
}

// NewID generates a new unique effect ID.
func NewID() string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 16)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := range b {
		b[i] = letters[r.Intn(len(letters))]
	}
	return string(b)
}