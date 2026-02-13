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
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"rag-platform/internal/agent/planner"
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

// buildReplayableEventStream 构造含 PlanGenerated + command_committed + NodeFinished 的事件流，使 Replay 时所有步骤均从事件注入（design/effect-system.md）
func buildReplayableEventStream(t *testing.T, store jobstore.JobStore, jobID string, taskGraph *planner.TaskGraph, nodeResults map[string][]byte) {
	t.Helper()
	ctx := context.Background()
	graphBytes, err := taskGraph.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	ver := 0
	// PlanGenerated
	planPl, _ := json.Marshal(map[string]interface{}{"task_graph": json.RawMessage(graphBytes), "goal": "test"})
	_, err = store.Append(ctx, jobID, ver, jobstore.JobEvent{JobID: jobID, Type: jobstore.PlanGenerated, Payload: planPl})
	if err != nil {
		t.Fatal(err)
	}
	ver++
	payloadResults := make(map[string]any)
	for _, n := range taskGraph.Nodes {
		resultBytes, ok := nodeResults[n.ID]
		if !ok {
			resultBytes = []byte(`"ok"`)
		}
		// command_committed
		cmdPl, _ := json.Marshal(map[string]interface{}{"node_id": n.ID, "command_id": n.ID, "result": json.RawMessage(resultBytes)})
		_, err = store.Append(ctx, jobID, ver, jobstore.JobEvent{JobID: jobID, Type: jobstore.CommandCommitted, Payload: cmdPl})
		if err != nil {
			t.Fatal(err)
		}
		ver++
		var nodeResult interface{}
		_ = json.Unmarshal(resultBytes, &nodeResult)
		payloadResults[n.ID] = nodeResult
		payloadResultsBytes, _ := json.Marshal(payloadResults)
		// NodeFinished
		nfPl, _ := json.Marshal(map[string]interface{}{
			"node_id":         n.ID,
			"step_id":         n.ID,
			"payload_results": json.RawMessage(payloadResultsBytes),
			"result_type":     "success",
		})
		_, err = store.Append(ctx, jobID, ver, jobstore.JobEvent{JobID: jobID, Type: jobstore.NodeFinished, Payload: nfPl})
		if err != nil {
			t.Fatal(err)
		}
		ver++
	}
}

// TestReplayDeterminism_BuildFromEventsTwice 同一事件流 Replay 两次得到的 ReplayContext 一致（design/effect-system.md 断言与测试）
func TestReplayDeterminism_BuildFromEventsTwice(t *testing.T) {
	ctx := context.Background()
	jobID := "job-replay-twice"
	store := jobstore.NewMemoryStore()
	taskGraph := &planner.TaskGraph{
		Nodes: []planner.TaskNode{{ID: "n1", Type: planner.NodeLLM}, {ID: "n2", Type: planner.NodeTool, ToolName: "t1"}},
		Edges: []planner.TaskEdge{{From: "n1", To: "n2"}},
	}
	buildReplayableEventStream(t, store, jobID, taskGraph, map[string][]byte{
		"n1": []byte(`"llm-out"`),
		"n2": []byte(`{"output":"tool-out"}`),
	})
	builder := replay.NewReplayContextBuilder(store)
	rc1, err1 := builder.BuildFromEvents(ctx, jobID)
	rc2, err2 := builder.BuildFromEvents(ctx, jobID)
	if err1 != nil || err2 != nil {
		t.Fatalf("BuildFromEvents: err1=%v err2=%v", err1, err2)
	}
	if rc1 == nil || rc2 == nil {
		t.Fatal("ReplayContext nil")
	}
	// CompletedNodeIDs 一致
	if len(rc1.CompletedNodeIDs) != len(rc2.CompletedNodeIDs) {
		t.Errorf("CompletedNodeIDs len: %d vs %d", len(rc1.CompletedNodeIDs), len(rc2.CompletedNodeIDs))
	}
	for k := range rc1.CompletedNodeIDs {
		if _, ok := rc2.CompletedNodeIDs[k]; !ok {
			t.Errorf("CompletedNodeIDs missing in rc2: %q", k)
		}
	}
	// CommandResults 一致
	if len(rc1.CommandResults) != len(rc2.CommandResults) {
		t.Errorf("CommandResults len: %d vs %d", len(rc1.CommandResults), len(rc2.CommandResults))
	}
	for k, v1 := range rc1.CommandResults {
		v2, ok := rc2.CommandResults[k]
		if !ok || string(v1) != string(v2) {
			t.Errorf("CommandResults[%q]: rc1=%q rc2=%q", k, v1, v2)
		}
	}
	// PayloadResults 一致
	if string(rc1.PayloadResults) != string(rc2.PayloadResults) {
		t.Errorf("PayloadResults: %q vs %q", rc1.PayloadResults, rc2.PayloadResults)
	}
}

// countingLLM 用于断言 Replay 路径下不调用 LLM（design/effect-system.md）
type countingLLM struct {
	generateCalls int64
}

func (c *countingLLM) Generate(ctx context.Context, prompt string) (string, error) {
	atomic.AddInt64(&c.generateCalls, 1)
	return "mock", nil
}

func (c *countingLLM) Calls() int { return int(atomic.LoadInt64(&c.generateCalls)) }

// TestReplayPath_DoesNotCallLLM 事件流已含全部 command_committed 时 RunForJob 仅注入、不调用 LLM（Replay 不触发副作用）
func TestReplayPath_DoesNotCallLLM(t *testing.T) {
	ctx := context.Background()
	jobID := "job-replay-no-llm"
	eventStore := jobstore.NewMemoryStore()
	taskGraph := &planner.TaskGraph{
		Nodes: []planner.TaskNode{{ID: "n1", Type: planner.NodeLLM}},
		Edges: []planner.TaskEdge{},
	}
	buildReplayableEventStream(t, eventStore, jobID, taskGraph, map[string][]byte{"n1": []byte(`"replayed"`)})
	replayBuilder := replay.NewReplayContextBuilder(eventStore)
	fakeJobStore := &fakeJobStoreForRunner{}
	cpStore := runtime.NewCheckpointStoreMem()
	mockLLM := &countingLLM{}
	adapters := map[string]NodeAdapter{
		planner.NodeLLM: &LLMNodeAdapter{LLM: mockLLM},
	}
	compiler := NewCompiler(adapters)
	runner := NewRunner(compiler)
	runner.SetCheckpointStores(cpStore, fakeJobStore)
	runner.SetReplayContextBuilder(replayBuilder)
	agent := &runtime.Agent{ID: "a1"}
	j := &JobForRunner{ID: jobID, AgentID: "a1", Goal: "g1", Cursor: ""}
	err := runner.RunForJob(ctx, agent, j)
	if err != nil {
		t.Fatalf("RunForJob: %v", err)
	}
	if mockLLM.Calls() != 0 {
		t.Errorf("Replay path must not call LLM: got %d calls", mockLLM.Calls())
	}
	gotJobID, gotStatus := fakeJobStore.getLast()
	if gotJobID != jobID {
		t.Errorf("UpdateStatus jobID = %q, want %q", gotJobID, jobID)
	}
	const statusCompleted = 2
	if gotStatus != statusCompleted {
		t.Errorf("UpdateStatus status = %d, want %d (Completed)", gotStatus, statusCompleted)
	}
}

// 已完成集合若仅包含确定性 step_id，也应被识别为已完成，避免 Advance 无进展循环。
func TestAdvance_StepIDCompleted_MarksDone(t *testing.T) {
	ctx := context.Background()
	jobID := "job-stepid-completed"
	graph := &planner.TaskGraph{
		Nodes: []planner.TaskNode{{ID: "n1", Type: planner.NodeLLM}},
		Edges: []planner.TaskEdge{},
	}
	graphBytes, err := graph.Marshal()
	if err != nil {
		t.Fatalf("marshal graph: %v", err)
	}
	stepID := DeterministicStepID(jobID, PlanDecisionID(graphBytes), 0, planner.NodeLLM)
	state := replay.NewExecutionState(&replay.ReplayContext{
		TaskGraphState:   graphBytes,
		CompletedNodeIDs: map[string]struct{}{stepID: {}},
	})

	store := &fakeJobStoreForRunner{}
	runner := NewRunner(NewCompiler(map[string]NodeAdapter{
		planner.NodeLLM: &LLMNodeAdapter{LLM: &countingLLM{}},
	}))
	runner.SetCheckpointStores(runtime.NewCheckpointStoreMem(), store)

	done, err := runner.Advance(ctx, jobID, state, &runtime.Agent{ID: "a1"}, &JobForRunner{ID: jobID, AgentID: "a1", Goal: "g1"})
	if err != nil {
		t.Fatalf("Advance err: %v", err)
	}
	if !done {
		t.Fatalf("Advance done = false, want true")
	}
	_, status := store.getLast()
	const statusCompleted = 2
	if status != statusCompleted {
		t.Fatalf("UpdateStatus = %d, want %d", status, statusCompleted)
	}
}

// 仅有 plan_generated（无 command/node 完成记录）时应走首次执行，而不是 replay 注入路径。
func TestRunForJob_PlanOnly_ExecutesLLM(t *testing.T) {
	ctx := context.Background()
	jobID := "job-plan-only-exec"
	eventStore := jobstore.NewMemoryStore()
	taskGraph := &planner.TaskGraph{
		Nodes: []planner.TaskNode{{ID: "n1", Type: planner.NodeLLM}},
		Edges: []planner.TaskEdge{},
	}
	graphBytes, err := taskGraph.Marshal()
	if err != nil {
		t.Fatalf("marshal graph: %v", err)
	}
	planPayload, _ := json.Marshal(map[string]interface{}{
		"task_graph": json.RawMessage(graphBytes),
		"goal":       "g1",
	})
	if _, err := eventStore.Append(ctx, jobID, 0, jobstore.JobEvent{JobID: jobID, Type: jobstore.PlanGenerated, Payload: planPayload}); err != nil {
		t.Fatalf("append plan_generated: %v", err)
	}

	fakeJobStore := &fakeJobStoreForRunner{}
	cpStore := runtime.NewCheckpointStoreMem()
	mockLLM := &countingLLM{}
	compiler := NewCompiler(map[string]NodeAdapter{
		planner.NodeLLM: &LLMNodeAdapter{LLM: mockLLM},
	})
	runner := NewRunner(compiler)
	runner.SetCheckpointStores(cpStore, fakeJobStore)
	runner.SetReplayContextBuilder(replay.NewReplayContextBuilder(eventStore))

	err = runner.RunForJob(ctx, &runtime.Agent{ID: "a1"}, &JobForRunner{ID: jobID, AgentID: "a1", Goal: "g1"})
	if err != nil {
		t.Fatalf("RunForJob: %v", err)
	}
	if mockLLM.Calls() == 0 {
		t.Fatalf("expected LLM to be executed for plan-only job")
	}
	_, status := fakeJobStore.getLast()
	const statusCompleted = 2
	if status != statusCompleted {
		t.Fatalf("UpdateStatus = %d, want %d", status, statusCompleted)
	}
}
