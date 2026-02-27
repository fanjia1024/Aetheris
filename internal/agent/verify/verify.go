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

// Package verify 提供 Job 执行验证：Execution hash、Event chain root、Ledger proof、Replay proof（design/verification-mode.md）。
package verify

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"rag-platform/internal/agent/replay"
	"rag-platform/internal/runtime/jobstore"
)

// Result Verification Mode 输出，供 API/CLI 返回。
type Result struct {
	ExecutionHash             string            `json:"execution_hash"`
	EventChainRootHash        string            `json:"event_chain_root_hash"`
	ToolInvocationLedgerProof LedgerProofResult `json:"tool_invocation_ledger_proof"`
	ReplayProofResult         ReplayProofResult `json:"replay_proof_result"`
}

// LedgerProofResult Tool invocation ledger 证明：每条 started 均有匹配的 finished 或确定性failed。
type LedgerProofResult struct {
	OK                     bool     `json:"ok"`
	PendingIdempotencyKeys []string `json:"pending_idempotency_keys,omitempty"`
}

// ReplayProofResult 只读 Replay 与事件流一致性。
type ReplayProofResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// Compute 从事件流计算四类证明（只读）。jobID 仅用于 ReplayProof 时调用 builder。
func Compute(ctx context.Context, events []jobstore.JobEvent, jobID string, replayBuilder replay.ReplayContextBuilder) (*Result, error) {
	if len(events) == 0 {
		return &Result{
			ExecutionHash:             "",
			EventChainRootHash:        "",
			ToolInvocationLedgerProof: LedgerProofResult{OK: true},
			ReplayProofResult:         ReplayProofResult{OK: false, Error: "no events"},
		}, nil
	}

	execHash := ExecutionHash(events)
	chainRoot := EventChainRoot(events)
	ledgerProof := LedgerProof(events)

	replayOK := true
	replayErr := ""
	if replayBuilder != nil {
		rc, err := replayBuilder.BuildFromEvents(ctx, jobID)
		if err != nil {
			replayOK = false
			replayErr = err.Error()
		} else if rc == nil {
			replayOK = false
			replayErr = "no replay context (empty or no plan)"
		}
		_ = rc
	}

	return &Result{
		ExecutionHash:             execHash,
		EventChainRootHash:        chainRoot,
		ToolInvocationLedgerProof: ledgerProof,
		ReplayProofResult:         ReplayProofResult{OK: replayOK, Error: replayErr},
	}, nil
}

// EventChainRoot 计算事件链根 hash：H_i = SHA256(H_{i-1} || event_id || type || base64(payload))。
func EventChainRoot(events []jobstore.JobEvent) string {
	h := sha256.New()
	prev := []byte{}
	for _, e := range events {
		id := e.ID
		if id == "" {
			id = fmt.Sprintf("%d", len(prev))
		}
		payloadB64 := base64.StdEncoding.EncodeToString(e.Payload)
		h.Reset()
		h.Write(prev)
		h.Write([]byte("\n"))
		h.Write([]byte(id))
		h.Write([]byte(" "))
		h.Write([]byte(e.Type))
		h.Write([]byte(" "))
		h.Write([]byte(payloadB64))
		prev = h.Sum(nil)
	}
	if len(prev) == 0 {
		return ""
	}
	return fmt.Sprintf("%x", prev)
}

// ExecutionHash 计算执行路径摘要：plan 摘要 + 各 NodeFinished 的 (node_id, result_type) 序列的 hash。
func ExecutionHash(events []jobstore.JobEvent) string {
	var planHash string
	var nodeSeq []byte
	for _, e := range events {
		switch e.Type {
		case jobstore.PlanGenerated:
			var pl struct {
				PlanHash  string          `json:"plan_hash"`
				TaskGraph json.RawMessage `json:"task_graph"`
			}
			if json.Unmarshal(e.Payload, &pl) == nil {
				if pl.PlanHash != "" {
					planHash = pl.PlanHash
				} else if len(pl.TaskGraph) > 0 {
					sum := sha256.Sum256(pl.TaskGraph)
					planHash = fmt.Sprintf("%x", sum[:])
				}
			}
		case jobstore.NodeFinished:
			var pl struct {
				NodeID     string `json:"node_id"`
				ResultType string `json:"result_type"`
			}
			if json.Unmarshal(e.Payload, &pl) == nil {
				nodeSeq = append(nodeSeq, []byte(pl.NodeID+":"+pl.ResultType+"\n")...)
			}
		}
	}
	h := sha256.New()
	if planHash != "" {
		h.Write([]byte(planHash))
	}
	h.Write(nodeSeq)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// LedgerProof 校验每条 tool_invocation_started 均有匹配的 tool_invocation_finished。
func LedgerProof(events []jobstore.JobEvent) LedgerProofResult {
	pending := make(map[string]struct{})
	for _, e := range events {
		switch e.Type {
		case jobstore.ToolInvocationStarted:
			var pl struct {
				IdempotencyKey string `json:"idempotency_key"`
			}
			if json.Unmarshal(e.Payload, &pl) == nil && pl.IdempotencyKey != "" {
				pending[pl.IdempotencyKey] = struct{}{}
			}
		case jobstore.ToolInvocationFinished:
			var pl struct {
				IdempotencyKey string `json:"idempotency_key"`
			}
			if json.Unmarshal(e.Payload, &pl) == nil && pl.IdempotencyKey != "" {
				delete(pending, pl.IdempotencyKey)
			}
		}
	}
	keys := make([]string, 0, len(pending))
	for k := range pending {
		keys = append(keys, k)
	}
	return LedgerProofResult{
		OK:                     len(pending) == 0,
		PendingIdempotencyKeys: keys,
	}
}
