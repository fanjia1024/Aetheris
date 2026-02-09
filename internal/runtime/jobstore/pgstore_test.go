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
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"
)

func testDSN(t *testing.T) string {
	dsn := os.Getenv("TEST_JOBSTORE_DSN")
	if dsn == "" {
		t.Skip("TEST_JOBSTORE_DSN not set, skipping Postgres JobStore tests")
	}
	return dsn
}

func newTestPgStore(t *testing.T, ctx context.Context) (JobStore, func()) {
	store, err := NewPostgresStore(ctx, testDSN(t), 2*time.Second)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	pg, ok := store.(*pgStore)
	if !ok {
		t.Fatal("expected *pgStore")
	}
	// 清空表以便测试独立
	_, _ = pg.pool.Exec(ctx, `DELETE FROM job_claims`)
	_, _ = pg.pool.Exec(ctx, `DELETE FROM job_events`)
	return store, func() { pg.Close() }
}

func TestPgStore_ListEvents_Empty(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestPgStore(t, ctx)
	defer cleanup()
	events, ver, err := store.ListEvents(ctx, "job-1")
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if ver != 0 || len(events) != 0 {
		t.Errorf("expected version 0 and no events, got version %d and %d events", ver, len(events))
	}
}

func TestPgStore_Append_ListEvents(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestPgStore(t, ctx)
	defer cleanup()
	jobID := "job-1"
	payload, _ := json.Marshal(map[string]string{"goal": "test"})
	ev := JobEvent{JobID: jobID, Type: JobCreated, Payload: payload}

	newVer, err := store.Append(ctx, jobID, 0, ev)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if newVer != 1 {
		t.Errorf("expected newVersion 1, got %d", newVer)
	}

	events, ver, err := store.ListEvents(ctx, jobID)
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

func TestPgStore_Append_VersionMismatch(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestPgStore(t, ctx)
	defer cleanup()
	jobID := "job-1"
	ev := JobEvent{JobID: jobID, Type: JobCreated}

	_, _ = store.Append(ctx, jobID, 0, ev)
	ev2 := JobEvent{JobID: jobID, Type: PlanGenerated}
	_, err := store.Append(ctx, jobID, 0, ev2)
	if err != ErrVersionMismatch {
		t.Errorf("expected ErrVersionMismatch, got %v", err)
	}
	newVer, err := store.Append(ctx, jobID, 1, ev2)
	if err != nil {
		t.Fatalf("Append with correct version: %v", err)
	}
	if newVer != 2 {
		t.Errorf("expected newVersion 2, got %d", newVer)
	}
}

func TestPgStore_Claim_Heartbeat(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestPgStore(t, ctx)
	defer cleanup()
	jobID := "job-1"
	_, _ = store.Append(ctx, jobID, 0, JobEvent{JobID: jobID, Type: JobCreated})

	claimedID, ver, err := store.Claim(ctx, "worker-1")
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if claimedID != jobID || ver != 1 {
		t.Errorf("Claim: got jobID=%s version=%d", claimedID, ver)
	}

	_, _, err = store.Claim(ctx, "worker-2")
	if err != ErrNoJob {
		t.Errorf("expected ErrNoJob when job already claimed, got %v", err)
	}

	err = store.Heartbeat(ctx, "worker-1", jobID)
	if err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}

	err = store.Heartbeat(ctx, "worker-2", jobID)
	if err != ErrClaimNotFound {
		t.Errorf("expected ErrClaimNotFound for wrong worker, got %v", err)
	}
}

func TestPgStore_Claim_NoJob(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestPgStore(t, ctx)
	defer cleanup()
	_, _, err := store.Claim(ctx, "worker-1")
	if err != ErrNoJob {
		t.Errorf("expected ErrNoJob, got %v", err)
	}
}

func TestPgStore_Claim_SkipsCompleted(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestPgStore(t, ctx)
	defer cleanup()
	jobID := "job-1"
	_, _ = store.Append(ctx, jobID, 0, JobEvent{JobID: jobID, Type: JobCreated})
	_, _ = store.Append(ctx, jobID, 1, JobEvent{JobID: jobID, Type: JobCompleted})

	_, _, err := store.Claim(ctx, "worker-1")
	if err != ErrNoJob {
		t.Errorf("expected ErrNoJob for completed job, got %v", err)
	}
}

func TestPgStore_Watch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store, cleanup := newTestPgStore(t, ctx)
	defer cleanup()
	jobID := "job-1"
	ch, err := store.Watch(ctx, jobID)
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

	_, _ = store.Append(ctx, jobID, 0, JobEvent{JobID: jobID, Type: JobCreated})
	time.Sleep(300 * time.Millisecond)
	_, _ = store.Append(ctx, jobID, 1, JobEvent{JobID: jobID, Type: PlanGenerated})
	time.Sleep(300 * time.Millisecond)
	cancel()
	wg.Wait()

	if len(received) < 2 {
		t.Errorf("expected at least 2 events from Watch, got %d", len(received))
	}
}
