// Copyright 2026 fanjia1024
// Effect Store lifecycle management (GC/Archive/TTL)

package jobstore

import (
	"context"
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
	// TODO: Implement GC logic
	// 1. Find tool_invocations older than TTL
	// 2. If archive enabled, move to archive table
	// 3. Otherwise, delete directly
	return nil
}
