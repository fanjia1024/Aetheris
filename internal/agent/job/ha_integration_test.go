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
	"time"

	"rag-platform/internal/runtime/jobstore"
)

// haFakeEventStore simulates an event store where a job has an expired claim
// and allows a second worker to ClaimJob after reclaim (simulating lease expiry).
type haFakeEventStore struct {
	expiredIDs    []string
	events        map[string][]jobstore.JobEvent
	allowClaimJob map[string]bool // after "reclaim", allow ClaimJob for this job
}

func (f *haFakeEventStore) ListJobIDsWithExpiredClaim(ctx context.Context) ([]string, error) {
	return f.expiredIDs, nil
}

func (f *haFakeEventStore) ListEvents(ctx context.Context, jobID string) ([]jobstore.JobEvent, int, error) {
	events := f.events[jobID]
	return events, len(events), nil
}

func (f *haFakeEventStore) Append(ctx context.Context, jobID string, expectedVersion int, event jobstore.JobEvent) (int, error) {
	panic("not used")
}

func (f *haFakeEventStore) Claim(ctx context.Context, workerID string) (string, int, string, error) {
	panic("not used")
}

func (f *haFakeEventStore) ClaimJob(ctx context.Context, workerID string, jobID string) (int, string, error) {
	if f.allowClaimJob[jobID] {
		return 2, "attempt-" + workerID, nil
	}
	return 0, "", jobstore.ErrClaimNotFound
}

func (f *haFakeEventStore) Heartbeat(ctx context.Context, workerID string, jobID string) error {
	panic("not used")
}

func (f *haFakeEventStore) Watch(ctx context.Context, jobID string) (<-chan jobstore.JobEvent, error) {
	panic("not used")
}

func (f *haFakeEventStore) GetCurrentAttemptID(ctx context.Context, jobID string) (string, error) {
	return "", nil
}

// Snapshot methods (2.0-M1 additions)
func (f *haFakeEventStore) CreateSnapshot(ctx context.Context, jobID string, upToVersion int, snapshot []byte) error {
	return nil // Not used in HA test
}

func (f *haFakeEventStore) GetLatestSnapshot(ctx context.Context, jobID string) (*jobstore.JobSnapshot, error) {
	return nil, nil // Not used in HA test
}

func (f *haFakeEventStore) DeleteSnapshotsBefore(ctx context.Context, jobID string, beforeVersion int) error {
	return nil // Not used in HA test
}

// TestHA_ReclaimThenSecondWorkerCanClaim simulates: worker1 had job, "crashed" (expired lease),
// reclaim runs and sets job to Pending, then worker2 can claim the same job (event stream consistent).
func TestHA_ReclaimThenSecondWorkerCanClaim(t *testing.T) {
	ctx := context.Background()
	metadata := NewJobStoreMem()

	j1 := &Job{ID: "j1", AgentID: "a1", Goal: "g1", Status: StatusRunning}
	_, _ = metadata.Create(ctx, j1)
	_ = metadata.UpdateStatus(ctx, "j1", StatusRunning)

	now := time.Now()
	ev := func(typ jobstore.EventType) jobstore.JobEvent {
		return jobstore.JobEvent{JobID: "j1", Type: typ, CreatedAt: now}
	}

	eventStore := &haFakeEventStore{
		expiredIDs: []string{"j1"},
		events: map[string][]jobstore.JobEvent{
			"j1": {ev(jobstore.JobCreated), ev(jobstore.JobRunning)},
		},
		allowClaimJob: map[string]bool{"j1": true}, // after reclaim, allow worker2 to claim
	}

	n, err := ReclaimOrphanedFromEventStore(ctx, metadata, eventStore)
	if err != nil {
		t.Fatalf("ReclaimOrphanedFromEventStore: %v", err)
	}
	if n != 1 {
		t.Errorf("reclaimed = %d, want 1", n)
	}

	j, _ := metadata.Get(ctx, "j1")
	if j == nil || j.Status != StatusPending {
		t.Fatalf("after reclaim j1 should be Pending, got %+v", j)
	}

	// Second worker claims the job (simulating event store allowing new claim after expiry)
	_, attemptID, err := eventStore.ClaimJob(ctx, "worker2", "j1")
	if err != nil {
		t.Errorf("worker2 ClaimJob(j1) should succeed after reclaim: %v", err)
	}
	if attemptID == "" {
		t.Errorf("expected attemptID from worker2 claim")
	}

	// Event stream unchanged (still 2 events)
	events, ver, _ := eventStore.ListEvents(ctx, "j1")
	if ver != 2 || len(events) != 2 {
		t.Errorf("event stream should remain consistent: version=%d len(events)=%d", ver, len(events))
	}
}
