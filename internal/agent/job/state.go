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
	"rag-platform/internal/runtime/jobstore"
)

// DeriveStatusFromEvents 从事件流推导 Job 当前状态；事件流为权威来源（design/job-state-machine.md）。
// 顺序扫描 events，最后一条状态相关事件决定 status；无事件或仅有 job_created 时为 Pending（Queued）。
func DeriveStatusFromEvents(events []jobstore.JobEvent) JobStatus {
	if len(events) == 0 {
		return StatusPending
	}
	var status JobStatus
	for _, e := range events {
		switch e.Type {
		case jobstore.JobCreated, jobstore.JobQueued, jobstore.JobRequeued:
			status = StatusPending
		case jobstore.JobLeased, jobstore.JobRunning:
			status = StatusRunning
		case jobstore.JobWaiting:
			status = StatusWaiting
		case jobstore.WaitCompleted:
			// 收到 signal 后重新可被 Claim，视为 Pending；若实现细粒度可改为 Running
			status = StatusPending
		case jobstore.JobCompleted:
			status = StatusCompleted
		case jobstore.JobFailed:
			status = StatusFailed
		case jobstore.JobCancelled:
			status = StatusCancelled
		default:
			// 其他事件不改变状态
		}
	}
	return status
}
