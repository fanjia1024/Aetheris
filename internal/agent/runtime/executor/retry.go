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
	"errors"
	"strings"
	"time"
)

// RetryPolicy Tool 或节点级重试策略（2.0 Tool Contract）；可选配置，Runner/Adapter 失败时按此重试。
type RetryPolicy struct {
	// MaxRetries 最大重试次数（不含首次）
	MaxRetries int
	// Backoff 退避时间（固定或首次退避；后续可乘性递增由实现决定）
	Backoff time.Duration
	// RetryableErrors 可重试错误类型或消息子串匹配；空表示按 Step 失败类型（如 retryable_failure）决定
	RetryableErrors []string
}

// IsRetryable 判断错误是否可按 policy 重试；Adapter 在 Tool 执行失败时据此决定是否重试。
func IsRetryable(err error, policy *RetryPolicy) bool {
	if policy == nil || err == nil {
		return false
	}
	var sf *StepFailure
	if errors.As(err, &sf) && sf.Type == StepResultRetryableFailure {
		return true
	}
	if errors.Is(err, ErrRetryable) {
		return true
	}
	if len(policy.RetryableErrors) > 0 {
		msg := err.Error()
		for _, sub := range policy.RetryableErrors {
			if strings.Contains(msg, sub) {
				return true
			}
		}
		return false
	}
	return false
}

// ToolInvocationID 对外统一标识一次 Tool 调用；与 idempotency_key 或 invocation_id 对应，Trace/API 暴露此 ID。
// 从 ToolInvocationStartedPayload.InvocationID 或 IdempotencyKey 取得；见 design/tool-contract.md。
func ToolInvocationID(invocationID, idempotencyKey string) string {
	if invocationID != "" {
		return invocationID
	}
	return idempotencyKey
}
