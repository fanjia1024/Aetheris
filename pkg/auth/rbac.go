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
	"context"
)

// Permission 权限
type Permission string

const (
	PermissionJobView     Permission = "job:view"
	PermissionJobCreate   Permission = "job:create"
	PermissionJobStop     Permission = "job:stop"
	PermissionJobExport   Permission = "job:export" // 导出证据包
	PermissionTraceView   Permission = "trace:view"
	PermissionToolExecute Permission = "tool:execute"
	PermissionAgentManage Permission = "agent:manage"
	PermissionAuditView   Permission = "audit:view" // 查看审计日志
)

// Role 角色
type Role string

const (
	RoleAdmin    Role = "admin"    // 全部权限
	RoleOperator Role = "operator" // 查看 + 导出 + 停止
	RoleAuditor  Role = "auditor"  // 只读 + 导出 + 审计查看（不能创建/停止）
	RoleUser     Role = "user"     // 基本操作（不能导出）
)

// RolePermissions 角色与权限映射
var RolePermissions = map[Role][]Permission{
	RoleAdmin: {
		PermissionJobView,
		PermissionJobCreate,
		PermissionJobStop,
		PermissionJobExport,
		PermissionTraceView,
		PermissionToolExecute,
		PermissionAgentManage,
		PermissionAuditView,
	},
	RoleOperator: {
		PermissionJobView,
		PermissionJobStop,
		PermissionJobExport,
		PermissionTraceView,
		PermissionToolExecute,
	},
	RoleAuditor: {
		PermissionJobView,
		PermissionJobExport,
		PermissionTraceView,
		PermissionAuditView,
	},
	RoleUser: {
		PermissionJobView,
		PermissionJobCreate,
		PermissionTraceView,
	},
}

// RBACChecker RBAC 权限检查器接口
type RBACChecker interface {
	// CheckPermission 检查用户是否有权限访问资源
	CheckPermission(ctx context.Context, tenantID string, userID string, permission Permission, resourceID string) (bool, error)

	// GetUserRole 获取用户在租户中的角色
	GetUserRole(ctx context.Context, tenantID string, userID string) (Role, error)

	// AssignRole 分配角色给用户
	AssignRole(ctx context.Context, tenantID string, userID string, role Role) error
}

// HasPermission 检查角色是否包含指定权限
func HasPermission(role Role, permission Permission) bool {
	permissions, ok := RolePermissions[role]
	if !ok {
		return false
	}

	for _, p := range permissions {
		if p == permission {
			return true
		}
	}
	return false
}

// SimpleRBACChecker 简单的 RBAC 实现（基于内存或数据库）
type SimpleRBACChecker struct {
	roleStore RoleStore
}

// RoleStore 角色存储接口
type RoleStore interface {
	GetUserRole(ctx context.Context, tenantID string, userID string) (Role, error)
	SetUserRole(ctx context.Context, tenantID string, userID string, role Role) error
}

// NewSimpleRBACChecker 创建简单 RBAC 检查器
func NewSimpleRBACChecker(roleStore RoleStore) *SimpleRBACChecker {
	return &SimpleRBACChecker{roleStore: roleStore}
}

// CheckPermission 实现 RBACChecker 接口
func (c *SimpleRBACChecker) CheckPermission(ctx context.Context, tenantID string, userID string, permission Permission, resourceID string) (bool, error) {
	role, err := c.roleStore.GetUserRole(ctx, tenantID, userID)
	if err != nil {
		return false, err
	}

	return HasPermission(role, permission), nil
}

// GetUserRole 实现 RBACChecker 接口
func (c *SimpleRBACChecker) GetUserRole(ctx context.Context, tenantID string, userID string) (Role, error) {
	return c.roleStore.GetUserRole(ctx, tenantID, userID)
}

// AssignRole 实现 RBACChecker 接口
func (c *SimpleRBACChecker) AssignRole(ctx context.Context, tenantID string, userID string, role Role) error {
	return c.roleStore.SetUserRole(ctx, tenantID, userID, role)
}
