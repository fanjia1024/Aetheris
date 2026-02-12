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

package jobstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/google/uuid"
)

const leaseDuration = 30 * time.Second
const watchChanBuffer = 16

type claimRecord struct {
	WorkerID  string
	ExpiresAt time.Time
	AttemptID string
}

// memoryStore 内存实现：事件流 + 版本 + 租约 + Watch
type memoryStore struct {
	mu       sync.RWMutex
	byJob    map[string][]JobEvent
	claims   map[string]claimRecord
	watchers map[string][]chan JobEvent
}

// NewMemoryStore 创建内存版事件存储
func NewMemoryStore() JobStore {
	return &memoryStore{
		byJob:    make(map[string][]JobEvent),
		claims:   make(map[string]claimRecord),
		watchers: make(map[string][]chan JobEvent),
	}
}

func (s *memoryStore) ListEvents(ctx context.Context, jobID string) ([]JobEvent, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	events := s.byJob[jobID]
	version := len(events)
	if version == 0 {
		return nil, 0, nil
	}
	out := make([]JobEvent, version)
	for i := range events {
		e := events[i]
		if len(e.Payload) > 0 {
			e.Payload = make([]byte, len(events[i].Payload))
			copy(e.Payload, events[i].Payload)
		}
		out[i] = e
	}
	return out, version, nil
}

func (s *memoryStore) Append(ctx context.Context, jobID string, expectedVersion int, event JobEvent) (int, error) {
	if jobID == "" {
		return 0, ErrVersionMismatch
	}
	attemptID := AttemptIDFromContext(ctx)
	if attemptID != "" {
		s.mu.RLock()
		claim, ok := s.claims[jobID]
		s.mu.RUnlock()
		if !ok || claim.ExpiresAt.Before(time.Now()) || claim.AttemptID != attemptID {
			return 0, ErrStaleAttempt
		}
	}
	if event.ID == "" {
		event.ID = "ev-" + uuid.New().String()
	}
	event.JobID = jobID
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	payload := event.Payload
	if len(payload) > 0 {
		event.Payload = make([]byte, len(payload))
		copy(event.Payload, payload)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.byJob[jobID]
	if len(current) != expectedVersion {
		return 0, ErrVersionMismatch
	}

	// 2.0-M1: 计算 proof chain hash
	var prevHash string
	if expectedVersion > 0 && len(current) > 0 {
		prevHash = current[len(current)-1].Hash
	}
	eventHash := computeMemEventHash(jobID, event.Type, event.Payload, event.CreatedAt, prevHash)
	event.PrevHash = prevHash
	event.Hash = eventHash

	s.byJob[jobID] = append(current, event)
	newVersion := len(s.byJob[jobID])
	s.notifyWatchersLocked(jobID, event)
	return newVersion, nil
}

func (s *memoryStore) notifyWatchersLocked(jobID string, event JobEvent) {
	chans := s.watchers[jobID]
	if len(chans) == 0 {
		return
	}
	eventCopy := event
	if len(event.Payload) > 0 {
		eventCopy.Payload = make([]byte, len(event.Payload))
		copy(eventCopy.Payload, event.Payload)
	}
	var still []chan JobEvent
	for _, ch := range chans {
		select {
		case ch <- eventCopy:
			still = append(still, ch)
		default:
			close(ch)
		}
	}
	s.watchers[jobID] = still
}

func (s *memoryStore) Claim(ctx context.Context, workerID string) (string, int, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	attemptID := "attempt-" + uuid.New().String()
	for jobID, events := range s.byJob {
		if len(events) == 0 {
			continue
		}
		last := events[len(events)-1].Type
		if last == JobCompleted || last == JobFailed || last == JobCancelled {
			continue
		}
		claim, ok := s.claims[jobID]
		if ok && claim.ExpiresAt.After(now) {
			continue
		}
		s.claims[jobID] = claimRecord{WorkerID: workerID, ExpiresAt: now.Add(leaseDuration), AttemptID: attemptID}
		return jobID, len(events), attemptID, nil
	}
	return "", 0, "", ErrNoJob
}

func (s *memoryStore) ClaimJob(ctx context.Context, workerID string, jobID string) (int, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	events, ok := s.byJob[jobID]
	if !ok || len(events) == 0 {
		return 0, "", ErrNoJob
	}
	last := events[len(events)-1].Type
	if last == JobCompleted || last == JobFailed || last == JobCancelled {
		return 0, "", ErrNoJob
	}
	now := time.Now()
	if claim, ok := s.claims[jobID]; ok && claim.ExpiresAt.After(now) {
		return 0, "", ErrClaimNotFound
	}
	attemptID := "attempt-" + uuid.New().String()
	s.claims[jobID] = claimRecord{WorkerID: workerID, ExpiresAt: now.Add(leaseDuration), AttemptID: attemptID}
	return len(events), attemptID, nil
}

func (s *memoryStore) Heartbeat(ctx context.Context, workerID string, jobID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	claim, ok := s.claims[jobID]
	if !ok || claim.WorkerID != workerID || claim.ExpiresAt.Before(time.Now()) {
		return ErrClaimNotFound
	}
	s.claims[jobID] = claimRecord{WorkerID: workerID, ExpiresAt: time.Now().Add(leaseDuration)}
	return nil
}

func (s *memoryStore) GetCurrentAttemptID(ctx context.Context, jobID string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	claim, ok := s.claims[jobID]
	if !ok || claim.ExpiresAt.Before(time.Now()) {
		return "", nil
	}
	return claim.AttemptID, nil
}

func (s *memoryStore) ListJobIDsWithExpiredClaim(ctx context.Context) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	var ids []string
	for jobID, claim := range s.claims {
		if claim.ExpiresAt.Before(now) || claim.ExpiresAt.Equal(now) {
			ids = append(ids, jobID)
		}
	}
	return ids, nil
}

func (s *memoryStore) Watch(ctx context.Context, jobID string) (<-chan JobEvent, error) {
	ch := make(chan JobEvent, watchChanBuffer)
	s.mu.Lock()
	s.watchers[jobID] = append(s.watchers[jobID], ch)
	s.mu.Unlock()
	go func() {
		<-ctx.Done()
		s.mu.Lock()
		defer s.mu.Unlock()
		chans := s.watchers[jobID]
		for i, c := range chans {
			if c == ch {
				s.watchers[jobID] = append(chans[:i], chans[i+1:]...)
				if len(s.watchers[jobID]) == 0 {
					delete(s.watchers, jobID)
				}
				break
			}
		}
		close(ch)
	}()
	return ch, nil
}

// CreateSnapshot 创建快照（内存实现）
func (s *memoryStore) CreateSnapshot(ctx context.Context, jobID string, upToVersion int, snapshot []byte) error {
	// Memory store doesn't need snapshots (replay is already fast), but implement for interface compatibility
	return nil
}

// GetLatestSnapshot 获取最新快照（内存实现返回 nil）
func (s *memoryStore) GetLatestSnapshot(ctx context.Context, jobID string) (*JobSnapshot, error) {
	return nil, nil
}

// DeleteSnapshotsBefore 删除快照（内存实现无操作）
func (s *memoryStore) DeleteSnapshotsBefore(ctx context.Context, jobID string, beforeVersion int) error {
	return nil
}

// computeMemEventHash 计算事件哈希（2.0-M1 proof chain）
// Hash = SHA256(JobID|Type|Payload|Timestamp|PrevHash)
func computeMemEventHash(jobID string, eventType EventType, payload []byte, timestamp time.Time, prevHash string) string {
	h := sha256.New()
	h.Write([]byte(jobID))
	h.Write([]byte("|"))
	h.Write([]byte(eventType))
	h.Write([]byte("|"))
	if payload != nil {
		h.Write(payload)
	}
	h.Write([]byte("|"))
	h.Write([]byte(timestamp.Format(time.RFC3339Nano)))
	h.Write([]byte("|"))
	h.Write([]byte(prevHash))
	return hex.EncodeToString(h.Sum(nil))
}
