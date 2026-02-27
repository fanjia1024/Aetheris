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

package instance

import "context"

// AgentInstanceStore 持久化 Agent Instance；design/agent-instance-model.md
type AgentInstanceStore interface {
	Get(ctx context.Context, agentID string) (*AgentInstance, error)
	Create(ctx context.Context, instance *AgentInstance) error
	UpdateStatus(ctx context.Context, agentID, status string) error
	Update(ctx context.Context, instance *AgentInstance) error
	// UpdateCurrentJob 更新 Instance 的 current_job_id；Job 认领时设 jobID，完成/failed/挂起时清空（design/plan.md Phase B）
	UpdateCurrentJob(ctx context.Context, agentID, currentJobID string) error
	ListByTenant(ctx context.Context, tenantID string, limit int) ([]*AgentInstance, error)
}
