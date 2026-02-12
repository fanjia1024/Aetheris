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
	"time"

	agenteffects "rag-platform/internal/agent/runtime/effects"
	"rag-platform/pkg/agent/sdk"
)

// runtimeContextAdapter 实现 sdk.RuntimeContext，委托 effects 包并固定 jobID/stepID（2.0 Step Contract）
type runtimeContextAdapter struct {
	jobID  string
	stepID string
}

// Ensure runtimeContextAdapter implements sdk.RuntimeContext
var _ sdk.RuntimeContext = (*runtimeContextAdapter)(nil)

func newRuntimeContextAdapter(jobID, stepID string) sdk.RuntimeContext {
	return &runtimeContextAdapter{jobID: jobID, stepID: stepID}
}

func (a *runtimeContextAdapter) Now(ctx context.Context) time.Time {
	return agenteffects.Now(ctx)
}

func (a *runtimeContextAdapter) UUID(ctx context.Context) string {
	return agenteffects.UUID(ctx)
}

func (a *runtimeContextAdapter) HTTP(ctx context.Context, effectID string, doRequest func() (reqJSON, respJSON []byte, err error)) (reqJSON, respJSON []byte, err error) {
	return agenteffects.HTTP(ctx, effectID, doRequest)
}

func (a *runtimeContextAdapter) JobID(ctx context.Context) string {
	return a.jobID
}

func (a *runtimeContextAdapter) StepID(ctx context.Context) string {
	return a.stepID
}
