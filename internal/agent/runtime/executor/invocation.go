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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"
)

// ToolInvocationOutcome 工具调用结果：success | failure | timeout
const (
	ToolInvocationOutcomeSuccess = "success"
	ToolInvocationOutcomeFailure = "failure"
	ToolInvocationOutcomeTimeout = "timeout"
)

// IdempotencyKey 根据 job_id、node_id、tool_name、args 生成幂等键，Replay 时用于查找已完成的调用
func IdempotencyKey(jobID, nodeID, toolName string, args map[string]any) string {
	h := sha256.New()
	h.Write([]byte(jobID))
	h.Write([]byte("\x00"))
	h.Write([]byte(nodeID))
	h.Write([]byte("\x00"))
	h.Write([]byte(toolName))
	h.Write([]byte("\x00"))
	if len(args) > 0 {
		canon, _ := json.Marshal(canonicalizeMap(args))
		h.Write(canon)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func canonicalizeMap(m map[string]any) map[string]any {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]any, len(m))
	for _, k := range keys {
		v := m[k]
		if vm, ok := v.(map[string]any); ok {
			v = canonicalizeMap(vm)
		}
		out[k] = v
	}
	return out
}

// ToolInvocationStartedPayload tool_invocation_started 事件 payload
type ToolInvocationStartedPayload struct {
	InvocationID   string `json:"invocation_id"`
	ToolName       string `json:"tool_name"`
	ArgumentsHash  string `json:"arguments_hash,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
	StartedAt      string `json:"started_at"` // RFC3339
}

// ToolInvocationFinishedPayload tool_invocation_finished 事件 payload
type ToolInvocationFinishedPayload struct {
	InvocationID   string          `json:"invocation_id"`
	IdempotencyKey string          `json:"idempotency_key"`
	Outcome        string          `json:"outcome"` // success | failure | timeout
	Result         json.RawMessage  `json:"result,omitempty"`
	Error          string          `json:"error,omitempty"`
	FinishedAt     string          `json:"finished_at"` // RFC3339
}

// ArgumentsHash 对 args 做规范化 JSON 后取 sha256 前 16 字符，用于审计
func ArgumentsHash(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	b, _ := json.Marshal(canonicalizeMap(args))
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])[:16]
}

// FormatStartedAt 返回 RFC3339 时间戳
func FormatStartedAt(t time.Time) string { return t.UTC().Format(time.RFC3339) }
