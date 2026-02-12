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
)

type longTermMem struct {
	mu      sync.RWMutex
	store   map[string]map[string]map[string][]byte // agentID -> namespace -> key -> value
	updated map[string]time.Time                    // agentID+namespace+key -> updated_at (optional)
}

// NewLongTermMemoryStoreMem 创建内存版 LongTermMemoryStore
func NewLongTermMemoryStoreMem() LongTermMemoryStore {
	return &longTermMem{
		store:   make(map[string]map[string]map[string][]byte),
		updated: make(map[string]time.Time),
	}
}

func keyOf(agentID, namespace, k string) string {
	return agentID + "\x00" + namespace + "\x00" + k
}

func (s *longTermMem) Get(ctx context.Context, agentID, namespace, key string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ns, ok := s.store[agentID]
	if !ok {
		return nil, nil
	}
	kb, ok := ns[namespace]
	if !ok {
		return nil, nil
	}
	v, ok := kb[key]
	if !ok {
		return nil, nil
	}
	out := make([]byte, len(v))
	copy(out, v)
	return out, nil
}

func (s *longTermMem) Set(ctx context.Context, agentID, namespace, key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store[agentID] == nil {
		s.store[agentID] = make(map[string]map[string][]byte)
	}
	if s.store[agentID][namespace] == nil {
		s.store[agentID][namespace] = make(map[string][]byte)
	}
	v := make([]byte, len(value))
	copy(v, value)
	s.store[agentID][namespace][key] = v
	s.updated[keyOf(agentID, namespace, key)] = time.Now()
	return nil
}

func (s *longTermMem) ListByAgent(ctx context.Context, agentID string, namespace string, limit int) ([]KeyValue, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ns, ok := s.store[agentID]
	if !ok {
		return nil, nil
	}
	var list map[string][]byte
	if namespace != "" {
		list = ns[namespace]
	} else {
		list = make(map[string][]byte)
		for _, m := range ns {
			for k, v := range m {
				list[k] = v
			}
		}
	}
	if list == nil {
		return nil, nil
	}
	var out []KeyValue
	for k, v := range list {
		nsName := namespace
		if nsName == "" {
			nsName = "_"
		}
		out = append(out, KeyValue{Namespace: nsName, Key: k, Value: append([]byte(nil), v...)})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}
