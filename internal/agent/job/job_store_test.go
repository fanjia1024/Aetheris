package job

import (
	"context"
	"testing"
)

func TestJobStoreMem_Create_Get(t *testing.T) {
	ctx := context.Background()
	s := NewJobStoreMem()
	j := &Job{AgentID: "agent-1", Goal: "goal1", Status: StatusPending}
	id, err := s.Create(ctx, j)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id == "" {
		t.Fatal("Create returned empty id")
	}
	got, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.ID != id || got.AgentID != "agent-1" || got.Goal != "goal1" || got.Status != StatusPending {
		t.Errorf("Get: %+v", got)
	}
}

func TestJobStoreMem_ListByAgent(t *testing.T) {
	ctx := context.Background()
	s := NewJobStoreMem()
	_, _ = s.Create(ctx, &Job{AgentID: "agent-1", Goal: "g1"})
	_, _ = s.Create(ctx, &Job{AgentID: "agent-1", Goal: "g2"})
	_, _ = s.Create(ctx, &Job{AgentID: "agent-2", Goal: "g3"})
	list, err := s.ListByAgent(ctx, "agent-1")
	if err != nil {
		t.Fatalf("ListByAgent: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 jobs for agent-1, got %d", len(list))
	}
}

func TestJobStoreMem_UpdateStatus_UpdateCursor(t *testing.T) {
	ctx := context.Background()
	s := NewJobStoreMem()
	id, _ := s.Create(ctx, &Job{AgentID: "a1", Goal: "g"})
	if err := s.UpdateStatus(ctx, id, StatusRunning); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	got, _ := s.Get(ctx, id)
	if got.Status != StatusRunning {
		t.Errorf("expected StatusRunning, got %v", got.Status)
	}
	if err := s.UpdateCursor(ctx, id, "cp-1"); err != nil {
		t.Fatalf("UpdateCursor: %v", err)
	}
	got, _ = s.Get(ctx, id)
	if got.Cursor != "cp-1" {
		t.Errorf("expected cursor cp-1, got %q", got.Cursor)
	}
}

func TestJobStoreMem_ClaimNextPending(t *testing.T) {
	ctx := context.Background()
	s := NewJobStoreMem()
	id1, _ := s.Create(ctx, &Job{AgentID: "a1", Goal: "g1"})
	id2, _ := s.Create(ctx, &Job{AgentID: "a1", Goal: "g2"})

	j, err := s.ClaimNextPending(ctx)
	if err != nil || j == nil {
		t.Fatalf("ClaimNextPending: %v, j=%v", err, j)
	}
	if j.ID != id1 || j.Status != StatusRunning {
		t.Errorf("first claim: id=%s status=%v", j.ID, j.Status)
	}

	j2, _ := s.ClaimNextPending(ctx)
	if j2 == nil || j2.ID != id2 {
		t.Errorf("second claim: %+v", j2)
	}

	j3, _ := s.ClaimNextPending(ctx)
	if j3 != nil {
		t.Errorf("expected nil when no pending, got %+v", j3)
	}
}

func TestJobStoreMem_Requeue(t *testing.T) {
	ctx := context.Background()
	s := NewJobStoreMem()
	id, _ := s.Create(ctx, &Job{AgentID: "a1", Goal: "g"})
	j, _ := s.ClaimNextPending(ctx)
	if j.ID != id {
		t.Fatalf("claimed wrong job")
	}
	if err := s.Requeue(ctx, j); err != nil {
		t.Fatalf("Requeue: %v", err)
	}
	got, _ := s.Get(ctx, id)
	if got.Status != StatusPending || got.RetryCount != 1 {
		t.Errorf("after Requeue: status=%v retry_count=%d", got.Status, got.RetryCount)
	}
	// 应能再次被 Claim
	j2, _ := s.ClaimNextPending(ctx)
	if j2 == nil || j2.ID != id {
		t.Errorf("requeued job not claimable: %+v", j2)
	}
}

func TestJobStoreMem_Get_NotFound(t *testing.T) {
	ctx := context.Background()
	s := NewJobStoreMem()
	got, err := s.Get(ctx, "nonexistent")
	if err != nil || got != nil {
		t.Errorf("Get nonexistent: err=%v got=%v", err, got)
	}
}

func TestJobStatus_String(t *testing.T) {
	if StatusPending.String() != "pending" || StatusRunning.String() != "running" ||
		StatusCompleted.String() != "completed" || StatusFailed.String() != "failed" ||
		StatusCancelled.String() != "cancelled" {
		t.Errorf("JobStatus.String mismatch")
	}
}
