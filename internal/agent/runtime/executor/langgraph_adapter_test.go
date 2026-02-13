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
	"errors"
	"sync"
	"testing"

	"rag-platform/internal/agent/planner"
	"rag-platform/internal/agent/replay"
	"rag-platform/internal/agent/runtime"
	"rag-platform/internal/runtime/jobstore"
)

type fakeLangGraphClient struct {
	mu         sync.Mutex
	invokeCnt  int
	invokeFunc func(ctx context.Context, input map[string]any) (map[string]any, error)
}

func (f *fakeLangGraphClient) Invoke(ctx context.Context, input map[string]any) (map[string]any, error) {
	f.mu.Lock()
	f.invokeCnt++
	fn := f.invokeFunc
	f.mu.Unlock()
	if fn == nil {
		return map[string]any{"ok": true}, nil
	}
	return fn(ctx, input)
}

func (f *fakeLangGraphClient) Stream(ctx context.Context, input map[string]any, onChunk func(chunk map[string]any) error) error {
	if onChunk != nil {
		return onChunk(map[string]any{"chunk": "x"})
	}
	return nil
}

func (f *fakeLangGraphClient) State(ctx context.Context, threadID string) (map[string]any, error) {
	return map[string]any{"thread_id": threadID, "status": "ok"}, nil
}

func (f *fakeLangGraphClient) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.invokeCnt
}

func TestLangGraphNodeAdapter_ErrorMapping(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		assertErr func(t *testing.T, err error)
	}{
		{
			name: "retryable",
			err:  &LangGraphError{Code: LangGraphErrorRetryable, Message: "temporary"},
			assertErr: func(t *testing.T, err error) {
				var sf *StepFailure
				if !errors.As(err, &sf) || sf.Type != StepResultRetryableFailure {
					t.Fatalf("error = %v, want StepResultRetryableFailure", err)
				}
			},
		},
		{
			name: "permanent",
			err:  &LangGraphError{Code: LangGraphErrorPermanent, Message: "bad graph"},
			assertErr: func(t *testing.T, err error) {
				var sf *StepFailure
				if !errors.As(err, &sf) || sf.Type != StepResultPermanentFailure {
					t.Fatalf("error = %v, want StepResultPermanentFailure", err)
				}
			},
		},
		{
			name: "signal wait",
			err:  &LangGraphError{Code: LangGraphErrorWait, CorrelationKey: "lg-approval-1", Reason: "human_approval"},
			assertErr: func(t *testing.T, err error) {
				var sw *SignalWaitRequired
				if !errors.As(err, &sw) || sw.CorrelationKey != "lg-approval-1" {
					t.Fatalf("error = %v, want SignalWaitRequired", err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeLangGraphClient{invokeFunc: func(ctx context.Context, input map[string]any) (map[string]any, error) {
				return nil, tt.err
			}}
			adapter := &LangGraphNodeAdapter{Client: client}
			payload := &AgentDAGPayload{Goal: "g", Results: map[string]any{}}
			_, err := adapter.runNode(context.Background(), "lg1", map[string]any{}, payload)
			if err == nil {
				t.Fatalf("expected error")
			}
			tt.assertErr(t, err)
		})
	}
}

func appendPlanGeneratedForLangGraph(t *testing.T, store jobstore.JobStore, jobID string, g *planner.TaskGraph) {
	t.Helper()
	graphBytes, err := g.Marshal()
	if err != nil {
		t.Fatalf("marshal graph: %v", err)
	}
	payload, _ := json.Marshal(map[string]any{"task_graph": json.RawMessage(graphBytes), "goal": "langgraph"})
	if _, err := store.Append(context.Background(), jobID, 0, jobstore.JobEvent{JobID: jobID, Type: jobstore.PlanGenerated, Payload: payload}); err != nil {
		t.Fatalf("append plan_generated: %v", err)
	}
}

func TestLangGraphAdapter_ReplayAndSignalResume(t *testing.T) {
	ctx := context.Background()
	jobID := "job-langgraph-signal"
	eventStore := jobstore.NewMemoryStore()
	graph := &planner.TaskGraph{Nodes: []planner.TaskNode{{ID: "lg1", Type: planner.NodeLangGraph}}}
	appendPlanGeneratedForLangGraph(t, eventStore, jobID, graph)

	client := &fakeLangGraphClient{invokeFunc: func(ctx context.Context, input map[string]any) (map[string]any, error) {
		return nil, &LangGraphError{Code: LangGraphErrorWait, CorrelationKey: "lg-approval-1", Reason: "human_approval"}
	}}
	compiler := NewCompiler(map[string]NodeAdapter{planner.NodeLangGraph: &LangGraphNodeAdapter{Client: client}})
	runner := NewRunner(compiler)
	runner.SetCheckpointStores(runtime.NewCheckpointStoreMem(), &fakeJobStoreForRunner{})
	runner.SetReplayContextBuilder(replay.NewReplayContextBuilder(eventStore))

	err := runner.RunForJob(ctx, &runtime.Agent{ID: "a1"}, &JobForRunner{ID: jobID, AgentID: "a1", Goal: "approve", Cursor: ""})
	if !errors.Is(err, ErrJobWaiting) {
		t.Fatalf("first run err = %v, want ErrJobWaiting", err)
	}
	if client.Calls() != 1 {
		t.Fatalf("langgraph invoke calls = %d, want 1", client.Calls())
	}

	_, ver, err := eventStore.ListEvents(ctx, jobID)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	waitPayload, _ := json.Marshal(map[string]any{
		"node_id":         "lg1",
		"payload":         map[string]any{"approved": true},
		"correlation_key": "lg-approval-1",
	})
	if _, err := eventStore.Append(ctx, jobID, ver, jobstore.JobEvent{JobID: jobID, Type: jobstore.WaitCompleted, Payload: waitPayload}); err != nil {
		t.Fatalf("append wait_completed: %v", err)
	}

	err = runner.RunForJob(ctx, &runtime.Agent{ID: "a1"}, &JobForRunner{ID: jobID, AgentID: "a1", Goal: "approve", Cursor: ""})
	if err != nil {
		t.Fatalf("second run should complete, got err: %v", err)
	}
	if client.Calls() != 1 {
		t.Fatalf("langgraph invoke should not be re-executed after signal replay, got %d calls", client.Calls())
	}
}
