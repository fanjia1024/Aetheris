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

package sdk

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrNoRuntimeContext = errors.New("sdk: runtime context not set")

// RuntimeContext 供 Step 内使用的安全 API；由 Runner 注入，Replay 时从事件注入。Step 内禁止直接使用 time.Now/uuid.New/http。
type RuntimeContext interface {
	Now(ctx context.Context) time.Time
	UUID(ctx context.Context) string
	HTTP(ctx context.Context, effectID string, doRequest func() (reqJSON, respJSON []byte, err error)) (reqJSON, respJSON []byte, err error)
	JobID(ctx context.Context) string
	StepID(ctx context.Context) string
}

type contextKey struct{}

// WithRuntimeContext 注入 RuntimeContext；Runner 在调用 Step 前调用，内部实现可委托 internal/agent/runtime/effects
func WithRuntimeContext(ctx context.Context, rc RuntimeContext) context.Context {
	return context.WithValue(ctx, contextKey{}, rc)
}

// FromRuntimeContext 从 context 取出 RuntimeContext
func FromRuntimeContext(ctx context.Context) RuntimeContext {
	if ctx == nil {
		return nil
	}
	v := ctx.Value(contextKey{})
	if v == nil {
		return nil
	}
	rc, _ := v.(RuntimeContext)
	return rc
}

// Now 返回当前时间；Replay 时从事件注入。Step 内必须经此 API，禁止 time.Now()
func Now(ctx context.Context) time.Time {
	if rc := FromRuntimeContext(ctx); rc != nil {
		return rc.Now(ctx)
	}
	return time.Now()
}

// UUID 返回新 UUID；Replay 时从事件注入。Step 内必须经此 API，禁止 uuid.New()
func UUID(ctx context.Context) string {
	if rc := FromRuntimeContext(ctx); rc != nil {
		return rc.UUID(ctx)
	}
	return uuid.New().String()
}

// HTTP 执行 HTTP 并记录；Replay 时只读已记录结果。Step 内必须经此 API。未注入 RuntimeContext 时返回 ErrNoRuntimeContext
func HTTP(ctx context.Context, effectID string, doRequest func() (reqJSON, respJSON []byte, err error)) (reqJSON, respJSON []byte, err error) {
	if rc := FromRuntimeContext(ctx); rc != nil {
		return rc.HTTP(ctx, effectID, doRequest)
	}
	return nil, nil, ErrNoRuntimeContext
}

// JobID 从 context 取当前 job_id（Runner 注入）
func JobID(ctx context.Context) string {
	if rc := FromRuntimeContext(ctx); rc != nil {
		return rc.JobID(ctx)
	}
	return ""
}

// StepID 从 context 取当前 step_id（Runner 注入）
func StepID(ctx context.Context) string {
	if rc := FromRuntimeContext(ctx); rc != nil {
		return rc.StepID(ctx)
	}
	return ""
}
