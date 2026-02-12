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

package memory

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

type episodicMem struct {
	mu   sync.RWMutex
	list []*EpisodicEntry
}

// NewEpisodicMemoryStoreMem 创建内存版 EpisodicMemoryStore
func NewEpisodicMemoryStoreMem() EpisodicMemoryStore {
	return &episodicMem{list: nil}
}

func copyPayload(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (s *episodicMem) Append(ctx context.Context, entry *EpisodicEntry) error {
	if entry == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry.ID == "" {
		entry.ID = "ep-" + uuid.New().String()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	cp := *entry
	cp.Payload = copyPayload(entry.Payload)
	s.list = append(s.list, &cp)
	return nil
}

func (s *episodicMem) ListByAgent(ctx context.Context, agentID string, limit int) ([]*EpisodicEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*EpisodicEntry
	for i := len(s.list) - 1; i >= 0; i-- {
		e := s.list[i]
		if e.AgentID != agentID {
			continue
		}
		cp := *e
		cp.Payload = copyPayload(e.Payload)
		out = append(out, &cp)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *episodicMem) ListBySession(ctx context.Context, agentID, sessionID string, limit int) ([]*EpisodicEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*EpisodicEntry
	for i := len(s.list) - 1; i >= 0; i-- {
		e := s.list[i]
		if e.AgentID != agentID || e.SessionID != sessionID {
			continue
		}
		cp := *e
		cp.Payload = copyPayload(e.Payload)
		out = append(out, &cp)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}
