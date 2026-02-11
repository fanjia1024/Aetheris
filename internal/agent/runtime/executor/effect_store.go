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
)

// EffectKind 副作用类型（Execution Effect Journal）
const (
	EffectKindTool   = "tool"
	EffectKindLLM    = "llm"
	EffectKindHTTP   = "http"
	EffectKindTime   = "time"
	EffectKindRandom = "random"
	EffectKindHuman  = "human"
)

// EffectRecord 单条副作用记录；用于两步提交：先持久化 effect，再 Append command_committed（design：强 Replay）
type EffectRecord struct {
	JobID          string
	CommandID      string
	IdempotencyKey string
	Kind           string // tool | llm | http | time | random | human
	Input          []byte
	Output         []byte
	Error          string
	Metadata       map[string]any // 如 model, temperature（LLM）
	CreatedAt      time.Time
}

// EffectStore 副作用存储；执行完成后先写此处，再写事件流，实现「执行重放」与崩溃后 catch-up（design/effect-system.md 强 Replay）
type EffectStore interface {
	// PutEffect 持久化一条 effect；Adapter 在 Execute 成功后、Append 前调用（两步提交 Phase 1）
	PutEffect(ctx context.Context, r *EffectRecord) error
	// GetEffectByJobAndIdempotencyKey 按 job_id + idempotency_key 查询；Tool 节点 catch-up 时使用
	GetEffectByJobAndIdempotencyKey(ctx context.Context, jobID, idempotencyKey string) (*EffectRecord, error)
	// GetEffectByJobAndCommandID 按 job_id + command_id 查询；LLM 节点 catch-up 时使用
	GetEffectByJobAndCommandID(ctx context.Context, jobID, commandID string) (*EffectRecord, error)
}
