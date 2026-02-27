// Copyright 2026 fanjia1024
// Effect Store lifecycle management (GC/Archive/TTL)

package jobstore

import (
	"context"
	"fmt"
	"time"
)

// GCConfig Effect Store GC 配置
type GCConfig struct {
	Enable         bool          `yaml:"enable"`
	TTLDays        int           `yaml:"ttl_days"`
	ArchiveEnabled bool          `yaml:"archive_enabled"`
	RunInterval    time.Duration `yaml:"run_interval"`
	BatchSize      int           `yaml:"batch_size"`
}

// ToolInvocationRef 需要归档/删除的调用记录引用（复合主键：job_id + idempotency_key）
type ToolInvocationRef struct {
	JobID          string
	IdempotencyKey string
}

// EffectLifecycleStore 可选扩展接口：支持按 TTL 管理 tool_invocations 生命周期
type EffectLifecycleStore interface {
	// ListExpiredToolInvocations 列出早于 cutoff 的调用记录
	ListExpiredToolInvocations(ctx context.Context, cutoff time.Time, limit int) ([]ToolInvocationRef, error)
	// ArchiveToolInvocations 归档调用记录（可选）
	ArchiveToolInvocations(ctx context.Context, refs []ToolInvocationRef) error
	// DeleteToolInvocations 删除调用记录
	DeleteToolInvocations(ctx context.Context, refs []ToolInvocationRef) error
}

// SnapshotJobStore 扩展接口：支持查询高事件量的 Job 以触发自动快照（2.0 performance）
type SnapshotJobStore interface {
	JobStore
	// ListJobsWithHighEventCount 列出事件数 >= minEvents 的 job_id；用于 Worker 定时快照触发
	ListJobsWithHighEventCount(ctx context.Context, minEvents int, limit int) ([]string, error)
}

// DefaultGCConfig 默认 GC 配置
func DefaultGCConfig() GCConfig {
	return GCConfig{
		Enable:         false,
		TTLDays:        90,
		ArchiveEnabled: false,
		RunInterval:    24 * time.Hour,
		BatchSize:      1000,
	}
}

// GC 执行 tool_invocations 表的垃圾回收
func GC(ctx context.Context, store JobStore, config GCConfig) error {
	if !config.Enable {
		return nil
	}

	lifecycleStore, ok := store.(EffectLifecycleStore)
	if !ok {
		// 兼容unsupported effect lifecycle 的 JobStore（memory/legacy）
		return nil
	}

	ttlDays := config.TTLDays
	if ttlDays <= 0 {
		ttlDays = 90
	}
	batchSize := config.BatchSize
	if batchSize <= 0 {
		batchSize = 1000
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -ttlDays)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		refs, err := lifecycleStore.ListExpiredToolInvocations(ctx, cutoff, batchSize)
		if err != nil {
			return fmt.Errorf("list expired tool invocations: %w", err)
		}
		if len(refs) == 0 {
			return nil
		}

		if config.ArchiveEnabled {
			if err := lifecycleStore.ArchiveToolInvocations(ctx, refs); err != nil {
				return fmt.Errorf("archive expired tool invocations: %w", err)
			}
		}
		if err := lifecycleStore.DeleteToolInvocations(ctx, refs); err != nil {
			return fmt.Errorf("delete expired tool invocations: %w", err)
		}

		if len(refs) < batchSize {
			return nil
		}
	}
}
