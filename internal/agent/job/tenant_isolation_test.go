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
	"testing"
)

// TestTenantIsolation_ListByAgent 验证 ListByAgent 按 tenantID 隔离：tenantA 的 Job 对 tenantB 不可见。
func TestTenantIsolation_ListByAgent(t *testing.T) {
	ctx := context.Background()
	store := NewJobStoreMem()

	// tenantA 创建两个 job
	_, err := store.Create(ctx, &Job{AgentID: "agent-1", TenantID: "tenantA", Goal: "task A1"})
	if err != nil {
		t.Fatalf("create job A1: %v", err)
	}
	_, err = store.Create(ctx, &Job{AgentID: "agent-1", TenantID: "tenantA", Goal: "task A2"})
	if err != nil {
		t.Fatalf("create job A2: %v", err)
	}

	// tenantB 创建一个 job（同一 agentID）
	_, err = store.Create(ctx, &Job{AgentID: "agent-1", TenantID: "tenantB", Goal: "task B1"})
	if err != nil {
		t.Fatalf("create job B1: %v", err)
	}

	// tenantA 列表应只有 2 个 job
	jobsA, err := store.ListByAgent(ctx, "agent-1", "tenantA")
	if err != nil {
		t.Fatalf("list jobs for tenantA: %v", err)
	}
	if len(jobsA) != 2 {
		t.Errorf("tenantA 应有 2 个 job，实际 %d", len(jobsA))
	}
	for _, j := range jobsA {
		if j.TenantID != "tenantA" {
			t.Errorf("tenantA 列表中出现了其他租户的 job: tenantID=%s", j.TenantID)
		}
	}

	// tenantB 列表应只有 1 个 job
	jobsB, err := store.ListByAgent(ctx, "agent-1", "tenantB")
	if err != nil {
		t.Fatalf("list jobs for tenantB: %v", err)
	}
	if len(jobsB) != 1 {
		t.Errorf("tenantB 应有 1 个 job，实际 %d", len(jobsB))
	}
	for _, j := range jobsB {
		if j.TenantID != "tenantB" {
			t.Errorf("tenantB 列表中出现了其他租户的 job: tenantID=%s", j.TenantID)
		}
	}
}

// TestTenantIsolation_ClaimNextPendingForWorker 验证 ClaimNextPendingForWorker 按租户隔离 Claim：
// workerB 只能认领 tenantB 的 Job，不会获取 tenantA 的 Job。
func TestTenantIsolation_ClaimNextPendingForWorker(t *testing.T) {
	ctx := context.Background()
	store := NewJobStoreMem()

	idA, _ := store.Create(ctx, &Job{AgentID: "a", TenantID: "tenantA", Goal: "task A"})
	_, _ = store.Create(ctx, &Job{AgentID: "a", TenantID: "tenantB", Goal: "task B"})

	// workerA 只认领 tenantA 的 job
	claimedA, err := store.ClaimNextPendingForWorker(ctx, "", nil, "tenantA")
	if err != nil {
		t.Fatalf("claim tenantA: %v", err)
	}
	if claimedA == nil {
		t.Fatal("tenantA 应有可 claim 的 job")
	}
	if claimedA.TenantID != "tenantA" {
		t.Errorf("claimed job 应属于 tenantA，got %s", claimedA.TenantID)
	}
	if claimedA.ID != idA {
		t.Errorf("claimed job ID 不匹配，expected %s got %s", idA, claimedA.ID)
	}

	// 现在 tenantA 没有 pending job 了，再次 claim 应返回 nil
	claimedA2, err := store.ClaimNextPendingForWorker(ctx, "", nil, "tenantA")
	if err != nil {
		t.Fatalf("second claim tenantA: %v", err)
	}
	if claimedA2 != nil {
		t.Errorf("tenantA 已无 pending job，不应再 claim 到 job")
	}

	// workerB 仍然可以认领 tenantB 的 job
	claimedB, err := store.ClaimNextPendingForWorker(ctx, "", nil, "tenantB")
	if err != nil {
		t.Fatalf("claim tenantB: %v", err)
	}
	if claimedB == nil {
		t.Fatal("tenantB 应有可 claim 的 job")
	}
	if claimedB.TenantID != "tenantB" {
		t.Errorf("claimed job 应属于 tenantB，got %s", claimedB.TenantID)
	}
}
