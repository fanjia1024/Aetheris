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
// WITHOUT WARRANTIES OR ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package determinism

import (
	"context"
	"fmt"
)

// ReplayGuard 在 Replay 模式下对非确定性行为进行检测；若启用且检测到forbidden操作则 panic。
// 实现方式 A（推荐）：Step 仅能通过 runtime 注入的 Clock/UUID/HTTP 访问副作用，Runner 在 Replay 时注入只读实现；
// 本 Guard 用于可选的「严格模式」：当 Replay 且 StrictReplay 为 true 时，若 step 触发了未记录的 effect 路径则 panic。
type ReplayGuard struct {
	// StrictReplay 为 true 时，Replay 路径下任何未通过 Recorded Effects API 的副作用应触发 panic（若可检测）。
	StrictReplay bool
}

// CheckEffectAllowed 在 Replay 模式下检查是否允许执行某类 effect；若not allowed则 panic。
// jobID、stepID 用于error信息；op 为触发的forbidden操作类型。
// 仅在 Replay 且 StrictReplay 时进行严格检查；否则 no-op。
func (g *ReplayGuard) CheckEffectAllowed(ctx context.Context, jobID, stepID string, op ForbiddenOp) {
	if ctx == nil {
		return
	}
	if !IsReplay(ctx) {
		return
	}
	if !g.StrictReplay {
		return
	}
	panic(fmt.Sprintf("determinism: replay 模式下forbidden未记录的非确定性操作 job_id=%s step_id=%s op=%s: %s",
		jobID, stepID, op, op.Description()))
}

// contextKey 用于标记 context 是否处于 Replay 模式。
type contextKey string

const replayModeKey contextKey = "determinism.replay_mode"

// WithReplay 标记 context 处于 Replay 模式；Runner 在从事件流恢复执行时注入。
func WithReplay(ctx context.Context, replay bool) context.Context {
	return context.WithValue(ctx, replayModeKey, replay)
}

// IsReplay 返回当前 context 是否处于 Replay 模式。
func IsReplay(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v := ctx.Value(replayModeKey)
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}
