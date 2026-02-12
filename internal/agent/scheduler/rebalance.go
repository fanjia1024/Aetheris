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

package scheduler

import (
	"context"
)

// Rebalance 根据租约过期情况返回可回收的 job_id 列表；不执行 metadata 更新，由调用方（如 Worker 或独立 Reclaim 循环）对 metadata 置回 Pending。
// 初期实现仅依赖 LeaseManager.ListJobIDsWithExpiredClaim；后续可扩展为按队列积压或 Worker 负载做主动 rebalance。
func Rebalance(ctx context.Context, manager LeaseManager) (expiredJobIDs []string, err error) {
	return manager.ListJobIDsWithExpiredClaim(ctx)
}
