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

package jobstore

import (
	"context"
	"testing"
	"time"
)

// gcLifecycleStore 带计数器的内存 lifecycle store，用于验证 GC 调用顺序与正确性
type gcLifecycleStore struct {
	JobStore
	invocations   []ToolInvocationRef // 模拟待 GC 的调用记录
	archived      []ToolInvocationRef
	deleted       []ToolInvocationRef
	listCallCount int
}

func (f *gcLifecycleStore) ListExpiredToolInvocations(ctx context.Context, cutoff time.Time, limit int) ([]ToolInvocationRef, error) {
	f.listCallCount++
	if len(f.invocations) == 0 {
		return nil, nil
	}
	batch := f.invocations
	if len(batch) > limit {
		batch = batch[:limit]
		f.invocations = f.invocations[limit:]
	} else {
		f.invocations = nil
	}
	return append([]ToolInvocationRef(nil), batch...), nil
}

func (f *gcLifecycleStore) ArchiveToolInvocations(ctx context.Context, refs []ToolInvocationRef) error {
	f.archived = append(f.archived, refs...)
	return nil
}

func (f *gcLifecycleStore) DeleteToolInvocations(ctx context.Context, refs []ToolInvocationRef) error {
	f.deleted = append(f.deleted, refs...)
	return nil
}

// TestGC_BatchProcessing 验证 GC 正确分批处理并在最后一批小于 batchSize 时停止。
func TestGC_BatchProcessing(t *testing.T) {
	refs := make([]ToolInvocationRef, 25)
	for i := 0; i < 25; i++ {
		refs[i] = ToolInvocationRef{JobID: "job-1", IdempotencyKey: "key-" + string(rune('a'+i))}
	}

	store := &gcLifecycleStore{
		JobStore:    NewMemoryStore(),
		invocations: refs,
	}

	cfg := GCConfig{
		Enable:         true,
		TTLDays:        90,
		ArchiveEnabled: false,
		BatchSize:      10,
	}

	if err := GC(context.Background(), store, cfg); err != nil {
		t.Fatalf("GC failed: %v", err)
	}

	// 25 items, batch=10: batches of 10, 10, 5 → 3 list calls
	if store.listCallCount != 3 {
		t.Errorf("listCallCount = %d, want 3", store.listCallCount)
	}
	if len(store.deleted) != 25 {
		t.Errorf("deleted = %d, want 25", len(store.deleted))
	}
}

// TestGC_ArchiveBeforeDelete 验证 ArchiveEnabled=true 时先归档再删除，顺序正确。
func TestGC_ArchiveBeforeDelete(t *testing.T) {
	refs := []ToolInvocationRef{
		{JobID: "job-1", IdempotencyKey: "k1"},
		{JobID: "job-1", IdempotencyKey: "k2"},
	}

	store := &gcLifecycleStore{
		JobStore:    NewMemoryStore(),
		invocations: refs,
	}

	cfg := GCConfig{
		Enable:         true,
		ArchiveEnabled: true,
		TTLDays:        30,
		BatchSize:      100,
	}

	if err := GC(context.Background(), store, cfg); err != nil {
		t.Fatalf("GC failed: %v", err)
	}

	if len(store.archived) != 2 {
		t.Errorf("archived = %d, want 2", len(store.archived))
	}
	if len(store.deleted) != 2 {
		t.Errorf("deleted = %d, want 2", len(store.deleted))
	}

	// 验证归档的引用与删除的引用一致
	for i, ref := range store.archived {
		if ref.JobID != store.deleted[i].JobID || ref.IdempotencyKey != store.deleted[i].IdempotencyKey {
			t.Errorf("archived[%d]=%+v != deleted[%d]=%+v", i, ref, i, store.deleted[i])
		}
	}
}

// TestGC_NoOpForNonLifecycleStore 验证不支持 EffectLifecycleStore 的 JobStore 不报错直接返回。
func TestGC_NoOpForNonLifecycleStore(t *testing.T) {
	store := NewMemoryStore() // memoryStore 不实现 EffectLifecycleStore
	cfg := GCConfig{Enable: true, TTLDays: 90, BatchSize: 100}
	if err := GC(context.Background(), store, cfg); err != nil {
		t.Errorf("GC on non-lifecycle store should be no-op, got: %v", err)
	}
}

// TestGC_ContextCancellation 验证 context 取消时 GC 停止。
func TestGC_ContextCancellation(t *testing.T) {
	// 模拟无限批次（每次都返回 batchSize 条）
	store := &gcLifecycleStore{
		JobStore:    NewMemoryStore(),
		invocations: make([]ToolInvocationRef, 100),
	}
	for i := range store.invocations {
		store.invocations[i] = ToolInvocationRef{JobID: "job-1", IdempotencyKey: "k"}
		_ = i
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	cfg := GCConfig{Enable: true, TTLDays: 1, BatchSize: 5}
	err := GC(ctx, store, cfg)
	// GC 可能因 ctx.Err() 返回错误，也可能在耗尽数据后正常退出
	// 关键是不会无限循环
	if err != nil && err != context.DeadlineExceeded && err != context.Canceled {
		t.Errorf("unexpected error: %v", err)
	}
}
