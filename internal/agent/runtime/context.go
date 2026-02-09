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

package runtime

import (
	"context"
	"time"
)

// RunContext Agent 执行上下文：请求级数据、cancel、deadline、checkpoint 引用，在 Wake/Resume 时注入
type RunContext struct {
	Context context.Context

	AgentID    string
	SessionID  string
	Checkpoint string // 若从恢复点启动，指向 LastCheckpoint

	DeadlineAt time.Time
}

// WithRunContext 在标准 context 上附加 Agent 执行信息
func WithRunContext(ctx context.Context, agentID, sessionID, checkpoint string, deadline time.Time) *RunContext {
	return &RunContext{
		Context:    ctx,
		AgentID:    agentID,
		SessionID:  sessionID,
		Checkpoint: checkpoint,
		DeadlineAt: deadline,
	}
}

// Deadline 实现 context.Context
func (r *RunContext) Deadline() (time.Time, bool) {
	if !r.DeadlineAt.IsZero() {
		return r.DeadlineAt, true
	}
	return r.Context.Deadline()
}

// Done 实现 context.Context
func (r *RunContext) Done() <-chan struct{} {
	return r.Context.Done()
}

// Err 实现 context.Context
func (r *RunContext) Err() error {
	return r.Context.Err()
}

// Value 实现 context.Context
func (r *RunContext) Value(key interface{}) interface{} {
	return r.Context.Value(key)
}

// WithDeadline 返回带截止时间的 RunContext
func (r *RunContext) WithDeadline(d time.Time) *RunContext {
	return &RunContext{
		Context:    r.Context,
		AgentID:    r.AgentID,
		SessionID:  r.SessionID,
		Checkpoint: r.Checkpoint,
		DeadlineAt: d,
	}
}
