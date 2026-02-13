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
	"time"
)

// RetentionPolicy 留存策略（2.0-M2）
type RetentionPolicy struct {
	TenantID         string
	JobType          string // 按 job 类型配置不同策略（可选）
	RetentionDays    int    // 留存天数（0=永久）
	ArchiveAfterDays int    // 归档天数（移到冷存储）
	AutoDelete       bool   // 过期自动删除
}

// RetentionConfig 留存配置
type RetentionConfig struct {
	Enable               bool           `yaml:"enable"`
	DefaultRetentionDays int            `yaml:"default_retention_days"`
	ArchiveAfterDays     int            `yaml:"archive_after_days"`
	AutoDelete           bool           `yaml:"auto_delete"`
	ScanInterval         time.Duration  `yaml:"scan_interval"`
	Policies             []PolicyConfig `yaml:"policies"`
}

// PolicyConfig 单个策略配置（YAML）
type PolicyConfig struct {
	JobType       string `yaml:"job_type"`
	RetentionDays int    `yaml:"retention_days"`
}

// DefaultRetentionConfig 默认留存配置
func DefaultRetentionConfig() RetentionConfig {
	return RetentionConfig{
		Enable:               false,
		DefaultRetentionDays: 90,
		ArchiveAfterDays:     30,
		AutoDelete:           false,
		ScanInterval:         24 * time.Hour,
		Policies:             []PolicyConfig{},
	}
}

// GetPolicyForJob 获取 job 的留存策略
func (c *RetentionConfig) GetPolicyForJob(tenantID string, jobType string) RetentionPolicy {
	// 查找匹配的策略
	for _, p := range c.Policies {
		if p.JobType == jobType {
			return RetentionPolicy{
				TenantID:         tenantID,
				JobType:          jobType,
				RetentionDays:    p.RetentionDays,
				ArchiveAfterDays: c.ArchiveAfterDays,
				AutoDelete:       c.AutoDelete,
			}
		}
	}

	// 使用默认策略
	return RetentionPolicy{
		TenantID:         tenantID,
		JobType:          jobType,
		RetentionDays:    c.DefaultRetentionDays,
		ArchiveAfterDays: c.ArchiveAfterDays,
		AutoDelete:       c.AutoDelete,
	}
}
