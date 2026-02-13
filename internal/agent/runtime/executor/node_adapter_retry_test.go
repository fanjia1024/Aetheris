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
	"testing"
	"time"
)

type sequenceToolExec struct {
	mu         sync.Mutex
	calls      int
	errs       []error
	successOut string
}

func (s *sequenceToolExec) Execute(ctx context.Context, toolName string, input map[string]any, state interface{}) (ToolResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	idx := s.calls - 1
	if idx < len(s.errs) && s.errs[idx] != nil {
		return ToolResult{}, s.errs[idx]
	}
	return ToolResult{Done: true, Output: s.successOut}, nil
}

func (s *sequenceToolExec) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// TestToolNodeAdapter_RetryPolicyBackoff 验证 Tool 执行失败后按 RetryPolicy 重试并执行退避。
func TestToolNodeAdapter_RetryPolicyBackoff(t *testing.T) {
	tools := &sequenceToolExec{
		errs: []error{
			fmt.Errorf("transient #1: %w", ErrRetryable),
			fmt.Errorf("transient #2: %w", ErrRetryable),
		},
		successOut: "ok",
	}
	adapter := &ToolNodeAdapter{
		Tools: tools,
		RetryPolicy: &RetryPolicy{
			MaxRetries: 2,
			Backoff:    25 * time.Millisecond,
		},
	}
	payload := &AgentDAGPayload{Goal: "g", Results: map[string]any{}}

	start := time.Now()
	out, err := adapter.runNode(context.Background(), "n1", "demo_tool", map[string]any{"k": "v"}, nil, payload)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("runNode should succeed after retries: %v", err)
	}
	if out == nil {
		t.Fatal("runNode output payload is nil")
	}
	if tools.Calls() != 3 {
		t.Fatalf("tool calls = %d, want 3", tools.Calls())
	}
	if elapsed < 45*time.Millisecond {
		t.Fatalf("backoff not applied, elapsed=%v (<45ms)", elapsed)
	}
	if _, ok := out.Results["n1"]; !ok {
		t.Fatalf("payload missing node result for %q", "n1")
	}
}

// TestToolNodeAdapter_RetryPolicyStopsOnNonRetryable 验证非可重试错误不会继续重试。
func TestToolNodeAdapter_RetryPolicyStopsOnNonRetryable(t *testing.T) {
	tools := &sequenceToolExec{
		errs: []error{fmt.Errorf("fatal input validation error")},
	}
	adapter := &ToolNodeAdapter{
		Tools: tools,
		RetryPolicy: &RetryPolicy{
			MaxRetries:      3,
			Backoff:         10 * time.Millisecond,
			RetryableErrors: []string{"rate-limit", "temporary"},
		},
	}
	payload := &AgentDAGPayload{Goal: "g", Results: map[string]any{}}

	_, err := adapter.runNode(context.Background(), "n1", "demo_tool", nil, nil, payload)
	if err == nil {
		t.Fatal("runNode should return non-retryable error")
	}
	if tools.Calls() != 1 {
		t.Fatalf("tool calls = %d, want 1 for non-retryable error", tools.Calls())
	}
}
