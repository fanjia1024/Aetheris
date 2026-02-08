package jobstore

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

const leaseDuration = 30 * time.Second
const watchChanBuffer = 16

type claimRecord struct {
	WorkerID  string
	ExpiresAt time.Time
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

func (s *memoryStore) Claim(ctx context.Context, workerID string) (string, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for jobID, events := range s.byJob {
		if len(events) == 0 {
			continue
		}
		last := events[len(events)-1].Type
		if last == JobCompleted || last == JobFailed {
			continue
		}
		claim, ok := s.claims[jobID]
		if ok && claim.ExpiresAt.After(now) {
			continue
		}
		s.claims[jobID] = claimRecord{WorkerID: workerID, ExpiresAt: now.Add(leaseDuration)}
		return jobID, len(events), nil
	}
	return "", 0, ErrNoJob
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
