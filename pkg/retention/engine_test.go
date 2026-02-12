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

package retention

import (
	"context"
	"strings"
	"testing"
	"time"
)

// 内存 TombstoneStore 实现（用于测试）
type memTombstoneStore struct {
	tombstones map[string]Tombstone // jobID -> tombstone
}

func newMemTombstoneStore() *memTombstoneStore {
	return &memTombstoneStore{
		tombstones: make(map[string]Tombstone),
	}
}

func (m *memTombstoneStore) CreateTombstone(ctx context.Context, tombstone Tombstone) error {
	m.tombstones[tombstone.JobID] = tombstone
	return nil
}

func (m *memTombstoneStore) GetTombstone(ctx context.Context, jobID string) (*Tombstone, error) {
	if t, ok := m.tombstones[jobID]; ok {
		return &t, nil
	}
	return nil, nil
}

func (m *memTombstoneStore) ListTombstones(ctx context.Context, tenantID string, limit int) ([]Tombstone, error) {
	var result []Tombstone
	for _, t := range m.tombstones {
		if t.TenantID == tenantID {
			result = append(result, t)
		}
	}
	return result, nil
}

// TestRetention_ShouldDelete 测试过期检测
func TestRetention_ShouldDelete(t *testing.T) {
	config := RetentionConfig{
		Enable:               true,
		DefaultRetentionDays: 90,
	}

	engine := NewEngine(config, newMemTombstoneStore())

	policy := RetentionPolicy{
		RetentionDays: 90,
	}

	// 91 天前创建的 job 应该删除
	oldJob := time.Now().UTC().AddDate(0, 0, -91)
	if !engine.ShouldDelete(oldJob, policy) {
		t.Error("old job should be deleted")
	}

	// 1 天前创建的 job 不应该删除
	newJob := time.Now().UTC().AddDate(0, 0, -1)
	if engine.ShouldDelete(newJob, policy) {
		t.Error("new job should not be deleted")
	}
}

// TestRetention_TombstoneCreation 测试 tombstone 创建
func TestRetention_TombstoneCreation(t *testing.T) {
	config := DefaultRetentionConfig()
	config.Enable = true

	store := newMemTombstoneStore()
	engine := NewEngine(config, store)

	// 删除 job
	err := engine.DeleteJob(
		context.Background(),
		"job_123",
		"tenant_1",
		"agent_1",
		"user_admin",
		"retention_policy_expired",
		100,
	)
	if err != nil {
		t.Fatalf("delete job failed: %v", err)
	}

	// 验证 tombstone 创建
	tombstone, err := store.GetTombstone(context.Background(), "job_123")
	if err != nil {
		t.Fatalf("get tombstone failed: %v", err)
	}
	if tombstone == nil {
		t.Fatal("tombstone should be created")
	}

	if tombstone.JobID != "job_123" {
		t.Errorf("expected job_id job_123, got %s", tombstone.JobID)
	}
	if tombstone.DeletedBy != "user_admin" {
		t.Errorf("expected deleted_by user_admin, got %s", tombstone.DeletedBy)
	}
	if tombstone.Reason != "retention_policy_expired" {
		t.Errorf("expected reason retention_policy_expired, got %s", tombstone.Reason)
	}
}

// TestRetention_ArchiveJob 测试归档
func TestRetention_ArchiveJob(t *testing.T) {
	config := RetentionConfig{
		Enable:           true,
		ArchiveAfterDays: 30,
	}

	store := newMemTombstoneStore()
	engine := NewEngine(config, store)

	archiveRef, err := engine.ArchiveJob(context.Background(), "job_456", "tenant_1")
	if err != nil {
		t.Fatalf("archive failed: %v", err)
	}

	if archiveRef == "" {
		t.Error("archive ref should not be empty")
	}

	// 验证归档引用格式
	if !strings.Contains(archiveRef, "job_456") {
		t.Errorf("archive ref should contain job_id, got: %s", archiveRef)
	}
}
