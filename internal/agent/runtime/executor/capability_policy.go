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

import "context"

// CapabilityPolicyChecker 执行前校验：按能力/工具返回 allow / require_approval / deny（design/capability-policy.md）
type CapabilityPolicyChecker interface {
	// Check 返回 allowed、requiredApproval；approvedKeys 为事件流中已批准的 correlation_key，用于「审批后再次执行」时放行
	Check(ctx context.Context, jobID, toolName, capability, idempotencyKey string, approvedKeys map[string]struct{}) (allowed bool, requiredApproval bool, err error)
}

// PolicyAllowAll 默认实现：全部放行，不配置时行为与现有一致
type PolicyAllowAll struct{}

// Check 实现 CapabilityPolicyChecker；始终返回 allowed
func (PolicyAllowAll) Check(_ context.Context, _, _, _, _ string, _ map[string]struct{}) (bool, bool, error) {
	return true, false, nil
}

// CapabilityRequiresApproval 表示该步需人工/系统审批后才可执行；Runner 应写 job_waiting 并返回 ErrJobWaiting
type CapabilityRequiresApproval struct {
	CorrelationKey string
}

func (e *CapabilityRequiresApproval) Error() string {
	return "capability requires approval: " + e.CorrelationKey
}
