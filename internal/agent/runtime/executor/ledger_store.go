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

	"github.com/google/uuid"
)

// AttemptValidator 校验当前 writer 是否仍为该 job 的租约持有者；用于 Ledger Commit 等写操作的 Lease fencing（design/scheduler-correctness.md）
type AttemptValidator interface {
	ValidateAttempt(ctx context.Context, jobID string) error
}

// ledgerStore 包装 ToolInvocationStore，实现 InvocationLedger（执行许可裁决）
type ledgerStore struct {
	store            ToolInvocationStore
	attemptValidator AttemptValidator
}

// NewInvocationLedgerFromStore 从现有 ToolInvocationStore 创建 InvocationLedger（无 attempt 校验）
func NewInvocationLedgerFromStore(store ToolInvocationStore) InvocationLedger {
	if store == nil {
		return nil
	}
	return &ledgerStore{store: store}
}

// NewInvocationLedger 创建带可选 AttemptValidator 的 InvocationLedger；Commit 时若 validator 非 nil 则校验当前 attempt 仍为 job 持有者（Lease fencing）
func NewInvocationLedger(store ToolInvocationStore, validator AttemptValidator) InvocationLedger {
	if store == nil {
		return nil
	}
	return &ledgerStore{store: store, attemptValidator: validator}
}

// Acquire 实现 InvocationLedger
func (l *ledgerStore) Acquire(ctx context.Context, jobID, stepID, toolName, argsHash, idempotencyKey string, replayResult []byte) (InvocationDecision, *ToolInvocationRecord, error) {
	if len(replayResult) > 0 {
		return InvocationDecisionReturnRecordedResult, &ToolInvocationRecord{
			JobID: jobID, StepID: stepID, ToolName: toolName, ArgsHash: argsHash, IdempotencyKey: idempotencyKey,
			Result: replayResult, Committed: true, Status: ToolInvocationStatusSuccess,
		}, nil
	}
	rec, err := l.store.GetByJobAndIdempotencyKey(ctx, jobID, idempotencyKey)
	if err != nil {
		return InvocationDecisionRejected, nil, err
	}
	if rec != nil && rec.Committed && (rec.Status == ToolInvocationStatusSuccess || rec.Status == ToolInvocationStatusConfirmed) && len(rec.Result) > 0 {
		return InvocationDecisionReturnRecordedResult, rec, nil
	}
	if rec != nil && !rec.Committed {
		return InvocationDecisionWaitOtherWorker, nil, nil
	}
	invocationID := uuid.New().String()
	r := &ToolInvocationRecord{
		InvocationID:   invocationID,
		JobID:          jobID,
		StepID:         stepID,
		ToolName:       toolName,
		ArgsHash:       argsHash,
		IdempotencyKey: idempotencyKey,
		Status:         ToolInvocationStatusStarted,
	}
	if err := l.store.SetStarted(ctx, r); err != nil {
		return InvocationDecisionRejected, nil, err
	}
	return InvocationDecisionAllowExecute, r, nil
}

// Commit 实现 InvocationLedger；若配置了 AttemptValidator 则先校验当前 attempt 仍为该 job 持有者（Lease fencing），避免失去租约的 Worker 写入 Ledger
func (l *ledgerStore) Commit(ctx context.Context, invocationID, idempotencyKey string, result []byte) error {
	if l.attemptValidator != nil {
		jobID := JobIDFromContext(ctx)
		if jobID != "" {
			if err := l.attemptValidator.ValidateAttempt(ctx, jobID); err != nil {
				return err
			}
		}
	}
	return l.store.SetFinished(ctx, idempotencyKey, ToolInvocationStatusSuccess, result, true)
}

// Recover 实现 InvocationLedger
func (l *ledgerStore) Recover(ctx context.Context, jobID, idempotencyKey string) ([]byte, bool) {
	rec, err := l.store.GetByJobAndIdempotencyKey(ctx, jobID, idempotencyKey)
	if err != nil || rec == nil || !rec.Committed || len(rec.Result) == 0 {
		return nil, false
	}
	return rec.Result, true
}
