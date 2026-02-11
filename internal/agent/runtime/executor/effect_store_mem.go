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

package executor

import (
	"context"
	"sync"
	"time"
)

// EffectStoreMem 内存版 EffectStore；单进程有效，多 Worker 时需用 PG 等持久化实现
type EffectStoreMem struct {
	mu    sync.RWMutex
	byKey map[string]*EffectRecord // jobID+"\x00"+idempotencyKey or jobID+"\x00"+commandID
	byJob map[string][]*EffectRecord
}

// NewEffectStoreMem 创建内存版 EffectStore
func NewEffectStoreMem() *EffectStoreMem {
	return &EffectStoreMem{
		byKey: make(map[string]*EffectRecord),
		byJob: make(map[string][]*EffectRecord),
	}
}

func keyID(jobID, idempotencyKey string) string {
	return jobID + "\x00" + idempotencyKey
}

func keyCmd(jobID, commandID string) string {
	return jobID + "\x00cmd\x00" + commandID
}

func copyRecord(r *EffectRecord) *EffectRecord {
	if r == nil {
		return nil
	}
	out := &EffectRecord{
		JobID:          r.JobID,
		CommandID:      r.CommandID,
		IdempotencyKey: r.IdempotencyKey,
		Kind:           r.Kind,
		Error:          r.Error,
		CreatedAt:      r.CreatedAt,
	}
	if len(r.Input) > 0 {
		out.Input = make([]byte, len(r.Input))
		copy(out.Input, r.Input)
	}
	if len(r.Output) > 0 {
		out.Output = make([]byte, len(r.Output))
		copy(out.Output, r.Output)
	}
	if len(r.Metadata) > 0 {
		out.Metadata = make(map[string]any, len(r.Metadata))
		for k, v := range r.Metadata {
			out.Metadata[k] = v
		}
	}
	return out
}

// PutEffect 实现 EffectStore
func (s *EffectStoreMem) PutEffect(ctx context.Context, r *EffectRecord) error {
	if r == nil || r.JobID == "" {
		return nil
	}
	r.CreatedAt = time.Now().UTC()
	c := copyRecord(r)
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.IdempotencyKey != "" {
		s.byKey[keyID(r.JobID, r.IdempotencyKey)] = c
	}
	if r.CommandID != "" {
		s.byKey[keyCmd(r.JobID, r.CommandID)] = c
	}
	s.byJob[r.JobID] = append(s.byJob[r.JobID], c)
	return nil
}

// GetEffectByJobAndIdempotencyKey 实现 EffectStore
func (s *EffectStoreMem) GetEffectByJobAndIdempotencyKey(ctx context.Context, jobID, idempotencyKey string) (*EffectRecord, error) {
	if jobID == "" || idempotencyKey == "" {
		return nil, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	r := s.byKey[keyID(jobID, idempotencyKey)]
	return copyRecord(r), nil
}

// GetEffectByJobAndCommandID 实现 EffectStore
func (s *EffectStoreMem) GetEffectByJobAndCommandID(ctx context.Context, jobID, commandID string) (*EffectRecord, error) {
	if jobID == "" || commandID == "" {
		return nil, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	r := s.byKey[keyCmd(jobID, commandID)]
	return copyRecord(r), nil
}
