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

// CompensationFunc 补偿回调：对已提交的 step 执行回滚/补偿（如取消预订、退款）；应幂等。
// 仅针对已写 command_committed / tool_invocation_finished 的步骤；调用次数由 Runtime 保证一次或幂等。
// 见 design/tool-contract.md。
type CompensationFunc func(ctx context.Context, jobID, nodeID, stepID, commandID string) error

// CompensationRegistry 可选：按 node 注册补偿回调；Runner 在 compensatable_failure 或显式补偿请求时调用。
type CompensationRegistry interface {
	// GetCompensation 返回 nodeID 对应的补偿函数，无则 nil
	GetCompensation(nodeID string) CompensationFunc
}
