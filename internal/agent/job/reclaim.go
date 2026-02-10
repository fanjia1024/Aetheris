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

	"rag-platform/internal/runtime/jobstore"
)

// ReclaimOrphanedFromEventStore 以 event store 租约为准回收孤儿（design/runtime-contract.md §3）：
// 仅当租约已过期且 Job 未处于 Blocked（JobWaiting，§2）时，将 metadata 中该 job 置回 Pending。返回回收数量。
func ReclaimOrphanedFromEventStore(ctx context.Context, metadata JobStore, eventStore jobstore.JobStore) (int, error) {
	if metadata == nil || eventStore == nil {
		return 0, nil
	}
	ids, err := eventStore.ListJobIDsWithExpiredClaim(ctx)
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	var reclaimed int
	for _, jobID := range ids {
		events, _, err := eventStore.ListEvents(ctx, jobID)
		if err != nil {
			continue
		}
		if IsJobBlocked(events) {
			continue
		}
		j, err := metadata.Get(ctx, jobID)
		if err != nil || j == nil {
			continue
		}
		if j.Status != StatusRunning {
			continue
		}
		if err := metadata.UpdateStatus(ctx, jobID, StatusPending); err != nil {
			continue
		}
		reclaimed++
	}
	return reclaimed, nil
}
