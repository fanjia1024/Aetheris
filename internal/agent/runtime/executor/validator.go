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
)

// StepValidator optionally checks that a step complies with the Step Contract
// (design/step-contract.md): no direct time.Now, rand.*, or net/http in step code.
// Validation can be run before step execution (e.g. in tests or dev) or skipped in production.
// Implementations may run the step in a sandbox, use static analysis, or no-op.
type StepValidator interface {
	// ValidateStep checks the step for contract violations. jobID, stepID, nodeID, nodeType
	// identify the step; the runner may pass a runnable closure to execute in a sandbox.
	// If the step violates the contract (e.g. calls time.Now directly), return a non-nil error.
	ValidateStep(ctx context.Context, req StepValidationRequest) error
}

// StepValidationRequest carries the identifiers and optional sandbox runner for a step.
type StepValidationRequest struct {
	JobID    string
	StepID   string
	NodeID   string
	NodeType string
	// RunInSandbox, if non-nil, runs the step in a context where forbidden calls
	// (time.Now, rand.*, net/http) are detected. Used by test-time or optional runtime validators.
	RunInSandbox func(ctx context.Context) error
}

// ErrContractViolation is returned by validators when a step violates the Step Contract.
var ErrContractViolation = errors.New("step contract violation: forbidden use of time.Now, rand.*, or net/http")

// NoOpValidator is a StepValidator that always passes. Use when validation is disabled.
var NoOpValidator StepValidator = noOpValidator{}

type noOpValidator struct{}

func (noOpValidator) ValidateStep(context.Context, StepValidationRequest) error {
	return nil
}

// SandboxRunValidator runs req.RunInSandbox(ctx) when non-nil; useful for test-time validators
// that run the step in a detecting context. If RunInSandbox is nil, it returns nil.
// Note: when used from the Runner, the Runner passes RunInSandbox so the step runs inside
// the validator; the Runner then skips step.Run when validators are present and one of them
// runs the step. Currently the Runner does not pass RunInSandbox, so this validator no-ops
// unless tests wire it. For test-time detection of time.Now/rand/http, tests can replace
// those in the step's package or use a validator that runs the step with a custom context.
func NewSandboxRunValidator() StepValidator {
	return &sandboxRunValidator{}
}

type sandboxRunValidator struct{}

func (s *sandboxRunValidator) ValidateStep(ctx context.Context, req StepValidationRequest) error {
	if req.RunInSandbox == nil {
		return nil
	}
	return req.RunInSandbox(ctx)
}
