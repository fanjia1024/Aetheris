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
	"testing"
)

func TestNoOpValidator_AlwaysPasses(t *testing.T) {
	ctx := context.Background()
	req := StepValidationRequest{JobID: "j1", StepID: "s1", NodeID: "n1", NodeType: "tool"}
	if err := NoOpValidator.ValidateStep(ctx, req); err != nil {
		t.Errorf("NoOpValidator must always pass: %v", err)
	}
}

func TestSandboxRunValidator_NoSandbox_Passes(t *testing.T) {
	v := NewSandboxRunValidator()
	ctx := context.Background()
	req := StepValidationRequest{JobID: "j1", StepID: "s1", NodeID: "n1", NodeType: "llm"}
	if err := v.ValidateStep(ctx, req); err != nil {
		t.Errorf("SandboxRunValidator with nil RunInSandbox must pass: %v", err)
	}
}

func TestSandboxRunValidator_WithSandbox_ReturnsError(t *testing.T) {
	v := NewSandboxRunValidator()
	ctx := context.Background()
	wantErr := errors.New("sandbox detected violation")
	req := StepValidationRequest{
		JobID: "j1", StepID: "s1", NodeID: "n1", NodeType: "tool",
		RunInSandbox: func(context.Context) error { return wantErr },
	}
	err := v.ValidateStep(ctx, req)
	if err != wantErr {
		t.Errorf("got %v, want %v", err, wantErr)
	}
}

func TestSandboxRunValidator_WithSandbox_Passes(t *testing.T) {
	v := NewSandboxRunValidator()
	ctx := context.Background()
	req := StepValidationRequest{
		JobID: "j1", StepID: "s1", NodeID: "n1", NodeType: "tool",
		RunInSandbox: func(context.Context) error { return nil },
	}
	if err := v.ValidateStep(ctx, req); err != nil {
		t.Errorf("SandboxRunValidator with passing sandbox must pass: %v", err)
	}
}

func TestRunner_runStepValidators_NoValidators_Passes(t *testing.T) {
	c := NewCompiler(nil)
	r := NewRunner(c)
	ctx := context.Background()
	if err := r.runStepValidators(ctx, "j1", "s1", "n1", "tool", nil); err != nil {
		t.Errorf("runStepValidators with no validators must pass: %v", err)
	}
}

func TestRunner_runStepValidators_FailingValidator_ReturnsError(t *testing.T) {
	c := NewCompiler(nil)
	r := NewRunner(c)
	r.SetStepValidators(&failingValidator{err: ErrContractViolation})
	ctx := context.Background()
	err := r.runStepValidators(ctx, "j1", "s1", "n1", "tool", nil)
	if err == nil {
		t.Fatal("expected error from failing validator")
	}
	if !errors.Is(err, ErrContractViolation) {
		t.Errorf("got %v, want ErrContractViolation", err)
	}
}

type failingValidator struct{ err error }

func (f *failingValidator) ValidateStep(context.Context, StepValidationRequest) error {
	return f.err
}
