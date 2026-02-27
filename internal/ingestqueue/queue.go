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

package ingestqueue

import "context"

// IngestQueue 入库任务队列：API 入队，Worker 认领并执行 ingest_pipeline
type IngestQueue interface {
	// Enqueue 入队；payload 需含 content_base64（及可选 filename、metadata），返回 task_id
	Enqueue(ctx context.Context, payload map[string]interface{}) (taskID string, err error)
	// ClaimOne 原子认领一条 pending 任务，返回 task_id 与 payload；无任务时返回 "", nil, nil
	ClaimOne(ctx context.Context, workerID string) (taskID string, payload map[string]interface{}, err error)
	// MarkCompleted 标记任务完成
	MarkCompleted(ctx context.Context, taskID string, result interface{}) error
	// MarkFailed 标记任务failed
	MarkFailed(ctx context.Context, taskID string, errMsg string) error
	// GetStatus 查询任务状态（供 API 状态查询）；返回 status, result, errMsg, completedAt；not found返回 nil
	GetStatus(ctx context.Context, taskID string) (status string, result interface{}, errMsg string, completedAt interface{}, err error)
}
