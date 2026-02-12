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

package middleware

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"rag-platform/pkg/auth"
)

// AuthZMiddleware 授权中间件（2.0-M2 RBAC）
type AuthZMiddleware struct {
	rbac auth.RBACChecker
}

// NewAuthZMiddleware 创建授权中间件
func NewAuthZMiddleware(rbac auth.RBACChecker) *AuthZMiddleware {
	return &AuthZMiddleware{rbac: rbac}
}

// RequirePermission 返回权限检查中间件
func (a *AuthZMiddleware) RequirePermission(permission auth.Permission) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		userID := auth.GetUserID(ctx)
		tenantID := auth.GetTenantID(ctx)

		if userID == "" || tenantID == "" {
			c.JSON(consts.StatusUnauthorized, map[string]string{
				"error": "authentication required",
			})
			c.Abort()
			return
		}

		allowed, err := a.rbac.CheckPermission(ctx, tenantID, userID, permission, "")
		if err != nil || !allowed {
			c.JSON(consts.StatusForbidden, map[string]string{
				"error": "permission denied",
			})
			c.Abort()
			return
		}

		c.Next(ctx)
	}
}

// TenantIsolation Tenant 隔离中间件（确保用户只能访问自己 tenant 的资源）
func (a *AuthZMiddleware) TenantIsolation() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		tenantID := auth.GetTenantID(ctx)

		if tenantID == "" {
			c.JSON(consts.StatusUnauthorized, map[string]string{
				"error": "tenant context required",
			})
			c.Abort()
			return
		}

		c.Next(ctx)
	}
}
