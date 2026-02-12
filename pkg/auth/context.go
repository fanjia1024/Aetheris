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

type contextKey string

const (
	tenantIDKey contextKey = "auth.tenant_id"
	userIDKey   contextKey = "auth.user_id"
	roleKey     contextKey = "auth.role"
)

// WithTenantID 将 tenant_id 注入 context
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantIDKey, tenantID)
}

// GetTenantID 从 context 获取 tenant_id
func GetTenantID(ctx context.Context) string {
	if v, ok := ctx.Value(tenantIDKey).(string); ok {
		return v
	}
	return ""
}

// WithUserID 将 user_id 注入 context
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// GetUserID 从 context 获取 user_id
func GetUserID(ctx context.Context) string {
	if v, ok := ctx.Value(userIDKey).(string); ok {
		return v
	}
	return ""
}

// WithRole 将 role 注入 context
func WithRole(ctx context.Context, role Role) context.Context {
	return context.WithValue(ctx, roleKey, role)
}

// GetRole 从 context 获取 role
func GetRole(ctx context.Context) Role {
	if v, ok := ctx.Value(roleKey).(Role); ok {
		return v
	}
	return RoleUser // 默认 user 角色
}
