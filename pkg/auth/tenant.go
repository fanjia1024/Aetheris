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

package auth

import (
	"time"
)

// Tenant 租户模型（2.0-M2 多租户隔离）
type Tenant struct {
	ID        string
	Name      string
	Status    TenantStatus
	Quota     TenantQuota
	Metadata  map[string]string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TenantStatus 租户状态
type TenantStatus string

const (
	TenantStatusActive    TenantStatus = "active"
	TenantStatusSuspended TenantStatus = "suspended"
	TenantStatusDeleted   TenantStatus = "deleted"
)

// TenantQuota 租户配额
type TenantQuota struct {
	MaxJobs    int   // 最大 job 数（0=无限制）
	MaxStorage int64 // 最大存储（bytes，0=无限制）
	MaxExports int   // 每天最大导出次数（0=无限制）
	MaxAgents  int   // 最大 agent 实例数（0=无限制）
}

// DefaultTenantQuota 默认租户配额
func DefaultTenantQuota() TenantQuota {
	return TenantQuota{
		MaxJobs:    0,   // 无限制
		MaxStorage: 0,   // 无限制
		MaxExports: 100, // 每天 100 次导出
		MaxAgents:  100, // 100 个 agents
	}
}
