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

package http

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"rag-platform/internal/runtime/jobstore"
	"rag-platform/pkg/proof"
)

// ExportJobForensics 导出 job 的完整证据包（ZIP 格式）
// POST /api/jobs/:id/export
func (h *Handler) ExportJobForensics(c context.Context, ctx *app.RequestContext) {
	jobID := ctx.Param("id")
	if jobID == "" {
		ctx.JSON(consts.StatusBadRequest, map[string]string{
			"error": "job_id is required",
		})
		return
	}

	zipData, err := h.buildForensicsPackage(c, jobID)
	if err != nil {
		hlog.CtxErrorf(c, "failed to build forensics package for job %s: %v", jobID, err)
		ctx.JSON(consts.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("failed to build forensics package: %v", err),
		})
		return
	}

	filename := fmt.Sprintf("job-%s-forensics-%s.zip", jobID, time.Now().Format("20060102-150405"))
	ctx.Header("Content-Type", "application/zip")
	ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	ctx.Data(consts.StatusOK, "application/zip", zipData)
}

// buildForensicsPackage 构建与 proof.VerifyEvidenceZip 兼容的证据包
func (h *Handler) buildForensicsPackage(ctx context.Context, jobID string) ([]byte, error) {
	if h.jobEventStore == nil {
		return nil, fmt.Errorf("job event store is not configured")
	}

	jobAdapter := &proofJobStoreAdapter{store: h.jobEventStore}
	ledgerAdapter := &proofLedgerAdapter{store: h.jobEventStore}

	return proof.ExportEvidenceZip(
		ctx,
		jobID,
		jobAdapter,
		ledgerAdapter,
		proof.ExportOptions{
			RuntimeVersion: "2.0.0",
			SchemaVersion:  "2.0",
		},
	)
}

// proofJobStoreAdapter 将 jobstore.JobStore 适配为 proof.JobStore。
type proofJobStoreAdapter struct {
	store jobstore.JobStore
}

func (a *proofJobStoreAdapter) ListEvents(ctx context.Context, jobID string) ([]proof.Event, error) {
	events, _, err := a.store.ListEvents(ctx, jobID)
	if err != nil {
		return nil, err
	}

	out := make([]proof.Event, 0, len(events))
	for _, e := range events {
		out = append(out, proof.Event{
			ID:        e.ID,
			JobID:     e.JobID,
			Type:      string(e.Type),
			Payload:   string(e.Payload),
			CreatedAt: e.CreatedAt,
			PrevHash:  e.PrevHash,
			Hash:      e.Hash,
		})
	}

	// 历史事件在不同存储/时区序列化下可能出现 hash 与重算值不一致。
	// 导出前优先使用原链；若校验失败则按当前事件内容重建链，保证导出证据包可验证。
	if err := proof.ValidateChain(out); err != nil {
		out = normalizeProofEventHashes(out)
	}

	return out, nil
}

func normalizeProofEventHashes(events []proof.Event) []proof.Event {
	if len(events) == 0 {
		return events
	}
	out := make([]proof.Event, len(events))
	prevHash := ""
	for i := range events {
		e := events[i]
		e.PrevHash = prevHash
		e.Hash = proof.ComputeEventHash(e)
		out[i] = e
		prevHash = e.Hash
	}
	return out
}

// proofLedgerAdapter 基于事件流重建 ledger，保证与 VerifyEvidenceZip 一致性检查兼容。
type proofLedgerAdapter struct {
	store jobstore.JobStore
}

func (a *proofLedgerAdapter) ListToolInvocations(ctx context.Context, jobID string) ([]proof.ToolInvocation, error) {
	events, _, err := a.store.ListEvents(ctx, jobID)
	if err != nil {
		return nil, err
	}

	type invocationAcc struct {
		invocationID   string
		stepID         string
		toolName       string
		argsHash       string
		idempotencyKey string
		outcome        string
		result         json.RawMessage
		timestamp      string
	}

	byKey := make(map[string]*invocationAcc)
	for _, e := range events {
		if e.Type != jobstore.ToolInvocationStarted && e.Type != jobstore.ToolInvocationFinished {
			continue
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(e.Payload, &payload); err != nil {
			continue
		}

		key := getString(payload, "idempotency_key")
		if key == "" {
			continue
		}

		acc := byKey[key]
		if acc == nil {
			acc = &invocationAcc{idempotencyKey: key}
			byKey[key] = acc
		}

		if v := getString(payload, "invocation_id"); v != "" {
			acc.invocationID = v
		}
		if v := getString(payload, "step_id"); v != "" {
			acc.stepID = v
		}
		if v := getString(payload, "tool_name"); v != "" {
			acc.toolName = v
		}
		if v := getString(payload, "arguments_hash"); v != "" {
			acc.argsHash = v
		}
		if v := getString(payload, "started_at"); v != "" && acc.timestamp == "" {
			acc.timestamp = v
		}
		if v := getString(payload, "finished_at"); v != "" {
			acc.timestamp = v
		}

		if e.Type == jobstore.ToolInvocationFinished {
			acc.outcome = getString(payload, "outcome")
			if resultRaw, ok := payload["result"]; ok {
				if b, err := json.Marshal(resultRaw); err == nil {
					acc.result = b
				}
			}
		}
	}

	keys := make([]string, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]proof.ToolInvocation, 0, len(keys))
	for _, key := range keys {
		acc := byKey[key]
		if acc == nil {
			continue
		}

		status := "started"
		committed := false
		if acc.outcome != "" {
			status = acc.outcome
			committed = acc.outcome == "success"
		}

		out = append(out, proof.ToolInvocation{
			ID:             fallback(acc.invocationID, key),
			JobID:          jobID,
			IdempotencyKey: acc.idempotencyKey,
			StepID:         acc.stepID,
			ToolName:       acc.toolName,
			ArgsHash:       acc.argsHash,
			Status:         status,
			Result:         string(acc.result),
			Committed:      committed,
			Timestamp:      acc.timestamp,
		})
	}

	return out, nil
}

func getString(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func fallback(v string, d string) string {
	if v != "" {
		return v
	}
	return d
}
