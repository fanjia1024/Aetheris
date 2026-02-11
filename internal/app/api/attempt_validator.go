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

package api

import (
	"context"

	agentexec "rag-platform/internal/agent/runtime/executor"
	"rag-platform/internal/runtime/jobstore"
)

// attemptValidator 使用 event store 的当前租约校验 context 中的 attempt_id，供 Ledger Commit 等写操作 Lease fencing（design/scheduler-correctness.md）
type attemptValidator struct {
	store jobstore.JobStore
}

// NewAttemptValidator 创建 AttemptValidator；store 为 nil 时返回 nil（不校验）
func NewAttemptValidator(store jobstore.JobStore) agentexec.AttemptValidator {
	if store == nil {
		return nil
	}
	return &attemptValidator{store: store}
}

// ValidateAttempt 实现 executor.AttemptValidator；ctx 中无 attempt_id 时放行；有则校验与 store 当前 claim 一致，否则返回 ErrStaleAttempt
func (v *attemptValidator) ValidateAttempt(ctx context.Context, jobID string) error {
	want := jobstore.AttemptIDFromContext(ctx)
	if want == "" {
		return nil
	}
	current, err := v.store.GetCurrentAttemptID(ctx, jobID)
	if err != nil {
		return err
	}
	if current != want {
		return jobstore.ErrStaleAttempt
	}
	return nil
}
