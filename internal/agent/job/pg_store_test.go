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
	"os"
	"testing"
)

func testJobStoreDSN(t *testing.T) string {
	dsn := os.Getenv("TEST_JOBSTORE_DSN")
	if dsn == "" {
		t.Skip("TEST_JOBSTORE_DSN not set, skipping Postgres JobStore tests")
	}
	return dsn
}

func newTestJobStorePg(t *testing.T, ctx context.Context) (*JobStorePg, func()) {
	store, err := NewJobStorePg(ctx, testJobStoreDSN(t))
	if err != nil {
		t.Fatalf("NewJobStorePg: %v", err)
	}
	_, _ = store.pool.Exec(ctx, `DELETE FROM jobs`)
	return store, func() { store.Close() }
}

func TestJobStorePg_CreateGet(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestJobStorePg(t, ctx)
	defer cleanup()
	j := &Job{AgentID: "a1", Goal: "hello"}
	id, err := store.Create(ctx, j)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}
	got, err := store.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.AgentID != "a1" || got.Goal != "hello" || got.Status != StatusPending {
		t.Errorf("Get: got %+v", got)
	}
}

func TestJobStorePg_ClaimNextPending(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestJobStorePg(t, ctx)
	defer cleanup()
	_, _ = store.Create(ctx, &Job{AgentID: "a1", Goal: "g1"})
	claimed, err := store.ClaimNextPending(ctx)
	if err != nil {
		t.Fatalf("ClaimNextPending: %v", err)
	}
	if claimed == nil || claimed.Status != StatusRunning {
		t.Errorf("expected one claimed job Running, got %+v", claimed)
	}
	// 无更多 Pending
	claimed2, _ := store.ClaimNextPending(ctx)
	if claimed2 != nil {
		t.Errorf("expected nil second claim, got %+v", claimed2)
	}
}
