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

package job

import (
	"context"
	"time"
)

// ObservabilityReader 供运维可观测性：队列积压、卡住 Job 列表（2.0）。实现可选（如 JobStorePg）。
type ObservabilityReader interface {
	// CountPending 返回当前 Pending 状态的 Job 总数；queue 为空表示全部，非空表示该队列
	CountPending(ctx context.Context, queue string) (int, error)
	// ListStuckRunningJobIDs 返回 status=Running 且 updated_at 早于 (now - olderThan) 的 job_id 列表
	ListStuckRunningJobIDs(ctx context.Context, olderThan time.Duration) ([]string, error)
	// CountByStatus 返回各状态的 Job 数量，用于 job_state gauge（P0 SLO）；key 为 status 字符串（pending/running/waiting/parked/completed/failed/cancelled）
	CountByStatus(ctx context.Context) (map[string]int64, error)
}
