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
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

// TestStress_MultiWorkerRace runs 3 workers that race to Acquire/Commit the same step.
// At least one worker commits; all others either get WaitOtherWorker then ReturnRecordedResult on replay,
// or ReturnRecordedResult. The in-memory store does not guarantee atomic Acquire, so we only assert
// that after all finish, Recover returns exactly one result (no duplicate visible state).
// Strict single-commit semantics are tested in TestLedger_3_DoubleWorker_OnlyOneCommit.
func TestStress_MultiWorkerRace(t *testing.T) {
	store := NewToolInvocationStoreMem()
	ledger := NewInvocationLedgerFromStore(store)
	ctx := context.Background()
	jobID, stepID, toolName, argsHash, key := "job-race", "step-1", "tool1", "hash1", "job-race|step-1|tool1|hash1"

	var commitCount int32
	var wg sync.WaitGroup
	for w := 0; w < 3; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			decision, rec, err := ledger.Acquire(ctx, jobID, stepID, toolName, argsHash, key, nil)
			if err != nil {
				t.Errorf("worker %d Acquire: %v", workerID, err)
				return
			}
			switch decision {
			case InvocationDecisionAllowExecute:
				result := []byte(fmt.Sprintf(`{"worker":%d}`, workerID))
				if err := ledger.Commit(ctx, rec.InvocationID, key, result); err != nil {
					t.Errorf("worker %d Commit: %v", workerID, err)
					return
				}
				atomic.AddInt32(&commitCount, 1)
			case InvocationDecisionWaitOtherWorker:
				// Under concurrency, another worker may not have committed yet. Retry until ReturnRecordedResult or we give up.
				for i := 0; i < 50; i++ {
					decision2, rec2, err2 := ledger.Acquire(ctx, jobID, stepID, toolName, argsHash, key, nil)
					if err2 != nil {
						t.Errorf("worker %d replay Acquire: %v", workerID, err2)
						return
					}
					if decision2 == InvocationDecisionReturnRecordedResult && rec2 != nil {
						break
					}
				}
			case InvocationDecisionReturnRecordedResult:
				// Already replayed
			default:
				t.Errorf("worker %d unexpected decision %v", workerID, decision)
			}
		}(w)
	}
	wg.Wait()
	if n := atomic.LoadInt32(&commitCount); n < 1 {
		t.Errorf("expected at least 1 commit, got %d", n)
	}
	// Recover must return exactly one result (store state is consistent)
	got, exists := ledger.Recover(ctx, jobID, key)
	if !exists || len(got) == 0 {
		t.Errorf("Recover: expected one result, got exists=%v result=%q", exists, got)
	}
}

// TestStress_ManyJobs runs many distinct steps (different keys); each step is Acquired and Committed once.
// Verifies ledger and store behave correctly under concurrent distinct jobs.
func TestStress_ManyJobs(t *testing.T) {
	store := NewToolInvocationStoreMem()
	ledger := NewInvocationLedgerFromStore(store)
	ctx := context.Background()
	const N = 30
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			jobID := fmt.Sprintf("job-%d", idx)
			stepID := fmt.Sprintf("step-%d", idx)
			key := fmt.Sprintf("%s|%s|tool1|hash%d", jobID, stepID, idx)
			decision, rec, err := ledger.Acquire(ctx, jobID, stepID, "tool1", fmt.Sprintf("hash%d", idx), key, nil)
			if err != nil {
				t.Errorf("job %d Acquire: %v", idx, err)
				return
			}
			if decision != InvocationDecisionAllowExecute || rec == nil {
				t.Errorf("job %d expected AllowExecute, got %v", idx, decision)
				return
			}
			result := []byte(fmt.Sprintf(`{"idx":%d}`, idx))
			if err := ledger.Commit(ctx, rec.InvocationID, key, result); err != nil {
				t.Errorf("job %d Commit: %v", idx, err)
				return
			}
			// Replay same key: must get ReturnRecordedResult
			decision2, rec2, err2 := ledger.Acquire(ctx, jobID, stepID, "tool1", fmt.Sprintf("hash%d", idx), key, nil)
			if err2 != nil {
				t.Errorf("job %d replay Acquire: %v", idx, err2)
				return
			}
			if decision2 != InvocationDecisionReturnRecordedResult || rec2 == nil || string(rec2.Result) != string(result) {
				t.Errorf("job %d replay: want ReturnRecordedResult with result, got %v rec=%v", idx, decision2, rec2)
			}
		}(i)
	}
	wg.Wait()
}

// TestStress_CrashAfterToolBeforeCommit simulates: worker runs tool, then "crashes" before Commit.
// On recovery, another Acquire runs; the ledger may still have the started record (WaitOtherWorker)
// or it may have been cleaned up. Per TestLedger_4, second Acquire gets WaitOtherWorker.
// Then we Commit from the first worker's record (simulating delayed commit after recovery);
// subsequent Acquire must get ReturnRecordedResult and must not execute again.
func TestStress_CrashAfterToolBeforeCommit(t *testing.T) {
	store := NewToolInvocationStoreMem()
	ledger := NewInvocationLedgerFromStore(store)
	ctx := context.Background()
	jobID, stepID, toolName, argsHash, key := "job-crash", "step-1", "tool1", "hash1", "job-crash|step-1|tool1|hash1"

	// Worker1: Acquire (AllowExecute), "run tool", then "crash" (do not Commit)
	decision1, rec1, err := ledger.Acquire(ctx, jobID, stepID, toolName, argsHash, key, nil)
	if err != nil {
		t.Fatalf("worker1 Acquire: %v", err)
	}
	if decision1 != InvocationDecisionAllowExecute || rec1 == nil {
		t.Fatalf("worker1 expected AllowExecute, got %v", decision1)
	}
	// Simulate crash: no Commit

	// Worker2 (or "recovery"): Acquire same key -> must get WaitOtherWorker (no duplicate execution)
	decision2, rec2, err := ledger.Acquire(ctx, jobID, stepID, toolName, argsHash, key, nil)
	if err != nil {
		t.Fatalf("worker2 Acquire: %v", err)
	}
	if decision2 != InvocationDecisionWaitOtherWorker {
		t.Fatalf("worker2 expected WaitOtherWorker (first worker did not commit), got %v", decision2)
	}
	if rec2 != nil {
		t.Fatalf("worker2 should get nil record for WaitOtherWorker, got %v", rec2)
	}

	// Recovery: worker1's "delayed" Commit (e.g. from persisted intent)
	result := []byte(`{"done":true,"recovered":1}`)
	if err := ledger.Commit(ctx, rec1.InvocationID, key, result); err != nil {
		t.Fatalf("worker1 delayed Commit: %v", err)
	}

	// Any future Acquire (replay) must get ReturnRecordedResult, no second execution
	decision3, rec3, err := ledger.Acquire(ctx, jobID, stepID, toolName, argsHash, key, nil)
	if err != nil {
		t.Fatalf("replay Acquire: %v", err)
	}
	if decision3 != InvocationDecisionReturnRecordedResult || rec3 == nil {
		t.Fatalf("replay expected ReturnRecordedResult, got %v rec=%v", decision3, rec3)
	}
	if string(rec3.Result) != string(result) {
		t.Fatalf("replay result: got %q, want %q", rec3.Result, result)
	}
}
