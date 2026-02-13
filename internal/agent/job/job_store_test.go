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
	list, err := s.ListByAgent(ctx, "agent-1", "")
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

func TestJobStoreMem_ClaimNextPendingForWorker(t *testing.T) {
	ctx := context.Background()
	s := NewJobStoreMem()
	// 无能力要求的 Job
	id1, _ := s.Create(ctx, &Job{AgentID: "a1", Goal: "g1"})
	// 需要 llm 的 Job
	id2, _ := s.Create(ctx, &Job{AgentID: "a1", Goal: "g2", RequiredCapabilities: []string{"llm"}})
	// 需要 llm,tool 的 Job
	id3, _ := s.Create(ctx, &Job{AgentID: "a1", Goal: "g3", RequiredCapabilities: []string{"llm", "tool"}})

	// capabilities 为空等价于不按能力过滤，应拿到 id1
	j, err := s.ClaimNextPendingForWorker(ctx, "", nil, "")
	if err != nil || j == nil {
		t.Fatalf("ClaimNextPendingForWorker(nil): %v, j=%v", err, j)
	}
	if j.ID != id1 {
		t.Errorf("expected first job (no caps), got %s", j.ID)
	}

	// 仅有 tool 的 Worker 不应拿到需要 llm 或 llm+tool 的 Job，应拿到无能力要求的下一个（id2 需要 llm 不匹配，id3 需要 llm+tool 不匹配，但 id2 和 id3 都在 pending）
	j2, _ := s.ClaimNextPendingForWorker(ctx, "", []string{"tool"}, "")
	if j2 != nil {
		t.Errorf("worker [tool] should not get job requiring llm or llm+tool, got %s", j2.ID)
	}

	// 有 llm 的 Worker 可拿 id2
	j3, _ := s.ClaimNextPendingForWorker(ctx, "", []string{"llm"}, "")
	if j3 == nil || j3.ID != id2 {
		t.Errorf("worker [llm] expected id2, got %+v", j3)
	}

	// 有 llm,tool 的 Worker 可拿 id3
	j4, _ := s.ClaimNextPendingForWorker(ctx, "", []string{"llm", "tool"}, "")
	if j4 == nil || j4.ID != id3 {
		t.Errorf("worker [llm,tool] expected id3, got %+v", j4)
	}

	j5, _ := s.ClaimNextPendingForWorker(ctx, "", []string{"llm", "tool", "rag"}, "")
	if j5 != nil {
		t.Errorf("no more pending, got %+v", j5)
	}
}
