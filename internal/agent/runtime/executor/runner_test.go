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
	"strings"
	"sync"
	"testing"

	"rag-platform/internal/agent/replay"
	"rag-platform/internal/agent/runtime"
	"rag-platform/internal/runtime/jobstore"
)

// fakeJobStoreForRunner 记录 UpdateStatus 调用，用于断言 RunForJob 无 PlanGenerated 时置 Job Failed（design/runtime-contract.md §5）
type fakeJobStoreForRunner struct {
	mu         sync.Mutex
	lastJobID  string
	lastStatus int
}

func (f *fakeJobStoreForRunner) UpdateCursor(ctx context.Context, jobID string, cursor string) error {
	return nil
}

func (f *fakeJobStoreForRunner) UpdateStatus(ctx context.Context, jobID string, status int) error {
	f.mu.Lock()
	f.lastJobID = jobID
	f.lastStatus = status
	f.mu.Unlock()
	return nil
}

func (f *fakeJobStoreForRunner) getLast() (jobID string, status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastJobID, f.lastStatus
}

// TestRunForJob_NoPlanGenerated_FailsAndSetsJobFailed 契约：事件流中无 PlanGenerated 时 RunForJob 返回错误并将 Job 置为 Failed（design/runtime-contract.md §4、§5）
func TestRunForJob_NoPlanGenerated_FailsAndSetsJobFailed(t *testing.T) {
	ctx := context.Background()
	jobID := "job-no-plan"
	eventStore := jobstore.NewMemoryStore()
	// 不追加 PlanGenerated，仅空事件流（或仅有 JobCreated 也会因无 TaskGraph 走到失败路径）
	replayBuilder := replay.NewReplayContextBuilder(eventStore)
	fakeJobStore := &fakeJobStoreForRunner{}
	cpStore := runtime.NewCheckpointStoreMem()
	compiler := NewCompiler(nil)
	runner := NewRunner(compiler)
	runner.SetCheckpointStores(cpStore, fakeJobStore)
	runner.SetReplayContextBuilder(replayBuilder)

	agent := &runtime.Agent{ID: "a1"}
	j := &JobForRunner{ID: jobID, AgentID: "a1", Goal: "g1", Cursor: ""}

	err := runner.RunForJob(ctx, agent, j)
	if err == nil {
		t.Fatal("RunForJob with no PlanGenerated should return error")
	}
	if !strings.Contains(err.Error(), "PlanGenerated") {
		t.Errorf("error should mention PlanGenerated, got: %v", err)
	}
	gotJobID, gotStatus := fakeJobStore.getLast()
	if gotJobID != jobID {
		t.Errorf("UpdateStatus jobID = %q, want %q", gotJobID, jobID)
	}
	const statusFailed = 3
	if gotStatus != statusFailed {
		t.Errorf("UpdateStatus status = %d, want %d (Failed)", gotStatus, statusFailed)
	}
}

// 事件流仅有 JobCreated（无 PlanGenerated）时同样应失败并置 Failed
func TestRunForJob_OnlyJobCreated_NoPlanGenerated_FailsAndSetsJobFailed(t *testing.T) {
	ctx := context.Background()
	jobID := "job-created-only"
	eventStore := jobstore.NewMemoryStore()
	_, _ = eventStore.Append(ctx, jobID, 0, jobstore.JobEvent{JobID: jobID, Type: jobstore.JobCreated})
	replayBuilder := replay.NewReplayContextBuilder(eventStore)
	fakeJobStore := &fakeJobStoreForRunner{}
	cpStore := runtime.NewCheckpointStoreMem()
	compiler := NewCompiler(nil)
	runner := NewRunner(compiler)
	runner.SetCheckpointStores(cpStore, fakeJobStore)
	runner.SetReplayContextBuilder(replayBuilder)

	agent := &runtime.Agent{ID: "a1"}
	j := &JobForRunner{ID: jobID, AgentID: "a1", Goal: "g1", Cursor: ""}

	err := runner.RunForJob(ctx, agent, j)
	if err == nil {
		t.Fatal("RunForJob with only JobCreated (no PlanGenerated) should return error")
	}
	if !strings.Contains(err.Error(), "PlanGenerated") {
		t.Errorf("error should mention PlanGenerated, got: %v", err)
	}
	_, gotStatus := fakeJobStore.getLast()
	const statusFailed = 3
	if gotStatus != statusFailed {
		t.Errorf("UpdateStatus status = %d, want %d (Failed)", gotStatus, statusFailed)
	}
}
