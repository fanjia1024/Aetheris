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
	"errors"
	"sync"
	"testing"
	"time"

	"rag-platform/internal/agent/planner"
)

type timeoutNodeSink struct {
	mu                 sync.Mutex
	lastNodeID         string
	lastResultType     StepResultType
	lastReason         string
	lastFinishedCalled bool
}

func (s *timeoutNodeSink) AppendNodeStarted(ctx context.Context, jobID string, nodeID string, attempt int, workerID string) error {
	return nil
}

func (s *timeoutNodeSink) AppendNodeFinished(ctx context.Context, jobID string, nodeID string, payloadResults []byte, durationMs int64, state string, attempt int, resultType StepResultType, reason string, stepID string, inputHash string) error {
	s.mu.Lock()
	s.lastNodeID = nodeID
	s.lastResultType = resultType
	s.lastReason = reason
	s.lastFinishedCalled = true
	s.mu.Unlock()
	return nil
}

func (s *timeoutNodeSink) AppendStepCommitted(ctx context.Context, jobID string, nodeID string, stepID string, commandID string, idempotencyKey string) error {
	return nil
}

func (s *timeoutNodeSink) AppendStateCheckpointed(ctx context.Context, jobID string, nodeID string, stateBefore, stateAfter []byte, opts *StateCheckpointOpts) error {
	return nil
}

func (s *timeoutNodeSink) AppendJobWaiting(ctx context.Context, jobID string, nodeID string, waitKind, reason string, expiresAt time.Time, correlationKey string, resumptionContext []byte) error {
	return nil
}

func (s *timeoutNodeSink) AppendReasoningSnapshot(ctx context.Context, jobID string, payload []byte) error {
	return nil
}

func (s *timeoutNodeSink) AppendStepCompensated(ctx context.Context, jobID string, nodeID string, stepID string, commandID string, reason string) error {
	return nil
}

func (s *timeoutNodeSink) AppendMemoryRead(ctx context.Context, jobID string, nodeID string, stepIndex int, memoryType, keyOrScope, summary string) error {
	return nil
}

func (s *timeoutNodeSink) AppendMemoryWrite(ctx context.Context, jobID string, nodeID string, stepIndex int, memoryType, keyOrScope, summary string) error {
	return nil
}

func (s *timeoutNodeSink) AppendPlanEvolution(ctx context.Context, jobID string, planVersion int, diffSummary string) error {
	return nil
}

func (s *timeoutNodeSink) snapshot() (string, StepResultType, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastNodeID, s.lastResultType, s.lastReason, s.lastFinishedCalled
}

// TestRunnerParallelLevel_TimeoutClassifiedAsRetryableFailure 验证 step timeout 被映射为 retryable_failure，并写入 step timeout reason。
func TestRunnerParallelLevel_TimeoutClassifiedAsRetryableFailure(t *testing.T) {
	r := NewRunner(nil)
	r.SetStepTimeout(20 * time.Millisecond)
	jobStore := &fakeJobStoreForRunner{}
	r.jobStore = jobStore
	sink := &timeoutNodeSink{}
	r.SetNodeEventSink(sink)

	steps := []SteppableStep{{
		NodeID:   "n-timeout",
		NodeType: planner.NodeWorkflow,
		Run: func(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}}
	batch := []int{0}
	g := &planner.TaskGraph{Nodes: []planner.TaskNode{{ID: "n-timeout", Type: planner.NodeWorkflow}}}
	payload := &AgentDAGPayload{Goal: "timeout-case", Results: map[string]any{}}
	j := &JobForRunner{ID: "job-timeout", AgentID: "a1"}

	err := r.runParallelLevel(context.Background(), j, steps, batch, g, payload, nil, nil, map[string]struct{}{}, nil, "d1", "")
	if err == nil {
		t.Fatal("runParallelLevel should fail on step timeout")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error should wrap context deadline exceeded, got: %v", err)
	}

	nodeID, resultType, reason, called := sink.snapshot()
	if !called {
		t.Fatal("node_finished should be emitted on timeout")
	}
	if nodeID != "n-timeout" {
		t.Fatalf("node_finished node_id = %q, want %q", nodeID, "n-timeout")
	}
	if resultType != StepResultRetryableFailure {
		t.Fatalf("result_type = %q, want %q", resultType, StepResultRetryableFailure)
	}
	if reason != "step timeout" {
		t.Fatalf("reason = %q, want %q", reason, "step timeout")
	}
	gotJobID, gotStatus := jobStore.getLast()
	if gotJobID != "job-timeout" {
		t.Fatalf("UpdateStatus jobID = %q, want %q", gotJobID, "job-timeout")
	}
	const statusFailed = 3
	if gotStatus != statusFailed {
		t.Fatalf("UpdateStatus status = %d, want %d (Failed)", gotStatus, statusFailed)
	}
}
