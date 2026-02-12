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

package signal

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

type inboxMem struct {
	mu   sync.RWMutex
	byID map[string]*SignalRecord
}

// NewInboxMem 创建内存版 SignalInbox；单进程或测试用，多进程需 PG 实现
func NewInboxMem() SignalInbox {
	return &inboxMem{byID: make(map[string]*SignalRecord)}
}

func (m *inboxMem) Append(ctx context.Context, jobID, correlationKey string, payload []byte) (string, error) {
	id := "sig-" + uuid.New().String()
	now := time.Now()
	m.mu.Lock()
	m.byID[id] = &SignalRecord{
		ID:             id,
		JobID:          jobID,
		CorrelationKey: correlationKey,
		Payload:        append([]byte(nil), payload...),
		CreatedAt:      now,
	}
	m.mu.Unlock()
	return id, nil
}

func (m *inboxMem) MarkAcked(ctx context.Context, jobID, id string) error {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.byID[id]; ok {
		r.AckedAt = &now
	}
	return nil
}
