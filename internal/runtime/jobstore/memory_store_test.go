package jobstore

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestMemoryStore_ListEvents_Empty(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	events, ver, err := s.ListEvents(ctx, "job-1")
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if ver != 0 || len(events) != 0 {
		t.Errorf("expected version 0 and no events, got version %d and %d events", ver, len(events))
	}
}

func TestMemoryStore_Append_ListEvents(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	jobID := "job-1"
	payload, _ := json.Marshal(map[string]string{"goal": "test"})
	ev := JobEvent{JobID: jobID, Type: JobCreated, Payload: payload}

	newVer, err := s.Append(ctx, jobID, 0, ev)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if newVer != 1 {
		t.Errorf("expected newVersion 1, got %d", newVer)
	}

	events, ver, err := s.ListEvents(ctx, jobID)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if ver != 1 || len(events) != 1 {
		t.Errorf("expected version 1 and 1 event, got version %d and %d events", ver, len(events))
	}
	if events[0].Type != JobCreated || events[0].JobID != jobID {
		t.Errorf("event mismatch: %+v", events[0])
	}
}

func TestMemoryStore_Append_VersionMismatch(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	jobID := "job-1"
	ev := JobEvent{JobID: jobID, Type: JobCreated}

	_, _ = s.Append(ctx, jobID, 0, ev)
	ev2 := JobEvent{JobID: jobID, Type: PlanGenerated}
	_, err := s.Append(ctx, jobID, 0, ev2) // expectedVersion 0 but current is 1
	if err != ErrVersionMismatch {
		t.Errorf("expected ErrVersionMismatch, got %v", err)
	}
	newVer, err := s.Append(ctx, jobID, 1, ev2)
	if err != nil {
		t.Fatalf("Append with correct version: %v", err)
	}
	if newVer != 2 {
		t.Errorf("expected newVersion 2, got %d", newVer)
	}
}

func TestMemoryStore_Claim_Heartbeat(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	jobID := "job-1"
	_, _ = s.Append(ctx, jobID, 0, JobEvent{JobID: jobID, Type: JobCreated})

	// Claim 应返回该 job
	claimedID, ver, err := s.Claim(ctx, "worker-1")
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if claimedID != jobID || ver != 1 {
		t.Errorf("Claim: got jobID=%s version=%d", claimedID, ver)
	}

	// 同一 job 不应再被其他 worker 抢占（租约未过期）
	_, _, err = s.Claim(ctx, "worker-2")
	if err != ErrNoJob {
		t.Errorf("expected ErrNoJob when job already claimed, got %v", err)
	}

	// Heartbeat 续租
	err = s.Heartbeat(ctx, "worker-1", jobID)
	if err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}

	// 错误 worker 或错误 job 应返回 ErrClaimNotFound
	err = s.Heartbeat(ctx, "worker-2", jobID)
	if err != ErrClaimNotFound {
		t.Errorf("expected ErrClaimNotFound for wrong worker, got %v", err)
	}
}

func TestMemoryStore_Claim_NoJob(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	_, _, err := s.Claim(ctx, "worker-1")
	if err != ErrNoJob {
		t.Errorf("expected ErrNoJob, got %v", err)
	}
}

func TestMemoryStore_Claim_SkipsCompleted(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	jobID := "job-1"
	_, _ = s.Append(ctx, jobID, 0, JobEvent{JobID: jobID, Type: JobCreated})
	_, _ = s.Append(ctx, jobID, 1, JobEvent{JobID: jobID, Type: JobCompleted})

	_, _, err := s.Claim(ctx, "worker-1")
	if err != ErrNoJob {
		t.Errorf("expected ErrNoJob for completed job, got %v", err)
	}
}

func TestMemoryStore_Watch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := NewMemoryStore()
	jobID := "job-1"
	ch, err := s.Watch(ctx, jobID)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	var received []JobEvent
	go func() {
		defer wg.Done()
		for e := range ch {
			received = append(received, e)
		}
	}()

	_, _ = s.Append(ctx, jobID, 0, JobEvent{JobID: jobID, Type: JobCreated})
	time.Sleep(50 * time.Millisecond)
	_, _ = s.Append(ctx, jobID, 1, JobEvent{JobID: jobID, Type: PlanGenerated})
	time.Sleep(50 * time.Millisecond)
	cancel()
	wg.Wait()

	if len(received) < 2 {
		t.Errorf("expected at least 2 events from Watch, got %d", len(received))
	}
}
