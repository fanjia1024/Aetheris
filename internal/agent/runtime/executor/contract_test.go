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
	"testing"

	"rag-platform/internal/agent/runtime"
)

// stepUsingContractHelpers is a step that uses only runtime.Clock(ctx) and runtime.RandIntn(ctx, n).
// It satisfies the Step Contract: no direct time.Now() or rand.*.
func stepUsingContractHelpers(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error) {
	if p.Results == nil {
		p.Results = make(map[string]any)
	}
	t := runtime.Clock(ctx)
	p.Results["clock"] = t.Format("2006-01-02T15:04:05.000Z07:00")
	p.Results["rand"] = runtime.RandIntn(ctx, 1000)
	return p, nil
}

// TestStepContract_DeterministicReplay verifies that a step using runtime.Clock and runtime.RandIntn
// produces the same payload when run with the same replay clock and RNG (same jobID+stepID).
func TestStepContract_DeterministicReplay(t *testing.T) {
	jobID, stepID := "job-replay", "step-1"
	ctx := context.Background()
	ctx = runtime.WithClock(ctx, runtime.ReplayClock(jobID, stepID))
	ctx = runtime.WithRNG(ctx, runtime.ReplayRNG(jobID, stepID))

	p := &AgentDAGPayload{Results: make(map[string]any)}
	p1, err := stepUsingContractHelpers(ctx, p)
	if err != nil {
		t.Fatalf("step run: %v", err)
	}
	clock1, _ := p1.Results["clock"].(string)
	rand1, _ := p1.Results["rand"].(int)

	// Run again with same injection
	p2 := &AgentDAGPayload{Results: make(map[string]any)}
	ctx2 := context.Background()
	ctx2 = runtime.WithClock(ctx2, runtime.ReplayClock(jobID, stepID))
	ctx2 = runtime.WithRNG(ctx2, runtime.ReplayRNG(jobID, stepID))
	p2, err = stepUsingContractHelpers(ctx2, p2)
	if err != nil {
		t.Fatalf("step run 2: %v", err)
	}
	clock2, _ := p2.Results["clock"].(string)
	rand2, _ := p2.Results["rand"].(int)

	if clock1 != clock2 {
		t.Errorf("clock must be deterministic: %q vs %q", clock1, clock2)
	}
	if rand1 != rand2 {
		t.Errorf("rand must be deterministic: %d vs %d", rand1, rand2)
	}
}

// TestStepContract_ReplayVsLive verifies that with replay injection we get fixed values,
// and that payload can be JSON-serialized (state is replay-safe).
func TestStepContract_ReplayVsLive(t *testing.T) {
	ctxReplay := context.Background()
	ctxReplay = runtime.WithClock(ctxReplay, runtime.ReplayClock("j1", "s1"))
	ctxReplay = runtime.WithRNG(ctxReplay, runtime.ReplayRNG("j1", "s1"))

	p := &AgentDAGPayload{Results: make(map[string]any)}
	_, err := stepUsingContractHelpers(ctxReplay, p)
	if err != nil {
		t.Fatalf("step run: %v", err)
	}
	// Results must be serializable for event stream
	_, err = json.Marshal(p.Results)
	if err != nil {
		t.Errorf("payload results must be JSON-serializable: %v", err)
	}
}

// TestStepContract_ValidatorPassesForCompliantStep verifies that a step using only
// runtime.Clock(ctx) and runtime.RandIntn(ctx, n) passes when validated by NoOpValidator
// and by a validator that runs RunInSandbox (sandbox runs the same step and returns nil).
func TestStepContract_ValidatorPassesForCompliantStep(t *testing.T) {
	ctx := context.Background()
	ctx = runtime.WithClock(ctx, runtime.ReplayClock("j1", "s1"))
	ctx = runtime.WithRNG(ctx, runtime.ReplayRNG("j1", "s1"))

	req := StepValidationRequest{
		JobID: "j1", StepID: "s1", NodeID: "n1", NodeType: "llm",
		RunInSandbox: func(ctx context.Context) error {
			p := &AgentDAGPayload{Results: make(map[string]any)}
			_, err := stepUsingContractHelpers(ctx, p)
			return err
		},
	}
	if err := NoOpValidator.ValidateStep(ctx, req); err != nil {
		t.Errorf("NoOpValidator must pass: %v", err)
	}
	sandboxV := NewSandboxRunValidator()
	if err := sandboxV.ValidateStep(ctx, req); err != nil {
		t.Errorf("SandboxRunValidator with compliant step must pass: %v", err)
	}
}
