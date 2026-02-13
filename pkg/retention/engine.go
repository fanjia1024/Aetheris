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
	"fmt"
	"time"
)

// Engine 留存引擎（2.0-M2）
type Engine struct {
	config         RetentionConfig
	tombstoneStore TombstoneStore
	scanner        RetentionScanner
}

// TombstoneStore Tombstone 存储接口
type TombstoneStore interface {
	// CreateTombstone 创建 tombstone 记录
	CreateTombstone(ctx context.Context, tombstone Tombstone) error

	// GetTombstone 获取 tombstone 记录
	GetTombstone(ctx context.Context, jobID string) (*Tombstone, error)

	// ListTombstones 列出 tombstones
	ListTombstones(ctx context.Context, tenantID string, limit int) ([]Tombstone, error)
}

// Tombstone Job 删除的审计记录
type Tombstone struct {
	JobID         string
	TenantID      string
	AgentID       string
	DeletedAt     time.Time
	DeletedBy     string
	Reason        string
	EventCount    int
	RetentionDays int
	ArchiveRef    string
	MetadataJSON  []byte
}

// RetentionCandidate 留存扫描候选
type RetentionCandidate struct {
	JobID      string
	TenantID   string
	AgentID    string
	JobType    string
	CreatedAt  time.Time
	EventCount int
	Archived   bool
}

// RetentionScanner 过期扫描接口
type RetentionScanner interface {
	// ListCandidates 返回留存扫描候选列表
	ListCandidates(ctx context.Context) ([]RetentionCandidate, error)
}

// NewEngine 创建留存引擎
func NewEngine(config RetentionConfig, tombstoneStore TombstoneStore) *Engine {
	return &Engine{
		config:         config,
		tombstoneStore: tombstoneStore,
	}
}

// SetScanner 设置留存扫描数据源
func (e *Engine) SetScanner(scanner RetentionScanner) {
	e.scanner = scanner
}

// ArchiveJob 归档 job（导出证据包到冷存储）
func (e *Engine) ArchiveJob(ctx context.Context, jobID string, tenantID string) (string, error) {
	// TODO: 实际实现需要：
	// 1. 调用 proof.ExportEvidenceZip 导出证据包
	// 2. 上传到 S3/GCS/Azure Blob
	// 3. 返回存储 URL

	archiveRef := fmt.Sprintf("s3://archive/%s/%s.zip", tenantID, jobID)
	return archiveRef, nil
}

// DeleteJob 删除 job（写入 tombstone 事件）
func (e *Engine) DeleteJob(ctx context.Context, jobID string, tenantID string, agentID string, deletedBy string, reason string, eventCount int) error {
	// 创建 tombstone 记录
	policy := e.config.GetPolicyForJob(tenantID, "")

	tombstone := Tombstone{
		JobID:         jobID,
		TenantID:      tenantID,
		AgentID:       agentID,
		DeletedAt:     time.Now().UTC(),
		DeletedBy:     deletedBy,
		Reason:        reason,
		EventCount:    eventCount,
		RetentionDays: policy.RetentionDays,
		ArchiveRef:    "",
	}

	// 如果配置了归档，先归档
	if e.config.ArchiveAfterDays > 0 {
		archiveRef, err := e.ArchiveJob(ctx, jobID, tenantID)
		if err == nil {
			tombstone.ArchiveRef = archiveRef
		}
	}

	// 写入 tombstone
	return e.tombstoneStore.CreateTombstone(ctx, tombstone)
}

// RunRetentionScan 扫描并执行留存策略
func (e *Engine) RunRetentionScan(ctx context.Context) (int, error) {
	if !e.config.Enable {
		return 0, nil
	}
	if e.scanner == nil {
		return 0, nil
	}

	candidates, err := e.scanner.ListCandidates(ctx)
	if err != nil {
		return 0, err
	}

	processed := 0
	for _, c := range candidates {
		policy := e.config.GetPolicyForJob(c.TenantID, c.JobType)
		if e.ShouldDelete(c.CreatedAt, policy) && policy.AutoDelete {
			if err := e.DeleteJob(ctx, c.JobID, c.TenantID, c.AgentID, "retention-engine", "retention_policy_expired", c.EventCount); err != nil {
				return processed, err
			}
			processed++
			continue
		}

		if !c.Archived && e.ShouldArchive(c.CreatedAt, policy) {
			if _, err := e.ArchiveJob(ctx, c.JobID, c.TenantID); err != nil {
				return processed, err
			}
			processed++
		}
	}

	return processed, nil
}

// ShouldDelete 判断 job 是否应该删除
func (e *Engine) ShouldDelete(createdAt time.Time, policy RetentionPolicy) bool {
	if policy.RetentionDays == 0 {
		return false // 永久保留
	}

	expiryDate := createdAt.AddDate(0, 0, policy.RetentionDays)
	return time.Now().UTC().After(expiryDate)
}

// ShouldArchive 判断 job 是否应该归档
func (e *Engine) ShouldArchive(createdAt time.Time, policy RetentionPolicy) bool {
	if policy.ArchiveAfterDays == 0 {
		return false
	}

	archiveDate := createdAt.AddDate(0, 0, policy.ArchiveAfterDays)
	return time.Now().UTC().After(archiveDate)
}
