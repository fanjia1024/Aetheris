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
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"

	"rag-platform/pkg/auth"
)

// AuditMiddleware 访问审计中间件（2.0-M2）
type AuditMiddleware struct {
	auditStore AuditStore
}

// AuditStore 审计日志存储接口
type AuditStore interface {
	LogAccess(ctx context.Context, log AuditLog) error
}

// AuditLog 审计日志记录
type AuditLog struct {
	TenantID     string
	UserID       string
	Action       string
	ResourceType string
	ResourceID   string
	Success      bool
	DurationMS   int64
	CreatedAt    time.Time
}

// NewAuditMiddleware 创建审计中间件
func NewAuditMiddleware(auditStore AuditStore) *AuditMiddleware {
	return &AuditMiddleware{auditStore: auditStore}
}

// AuditAccess 记录 API 访问
func (a *AuditMiddleware) AuditAccess() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		start := time.Now()
		userID := auth.GetUserID(ctx)
		tenantID := auth.GetTenantID(ctx)

		c.Next(ctx)

		// 记录访问日志（异步，不阻塞请求）
		go func() {
			action := determineAction(string(c.Method()), string(c.Path()))
			resourceType, resourceID := extractResource(string(c.Path()))

			_ = a.auditStore.LogAccess(context.Background(), AuditLog{
				TenantID:     tenantID,
				UserID:       userID,
				Action:       action,
				ResourceType: resourceType,
				ResourceID:   resourceID,
				Success:      c.Response.StatusCode() < 400,
				DurationMS:   time.Since(start).Milliseconds(),
				CreatedAt:    time.Now().UTC(),
			})
		}()
	}
}

// determineAction 根据 HTTP 方法和路径确定操作类型
func determineAction(method string, path string) string {
	if strings.Contains(path, "/export") {
		return "export_evidence"
	}
	if strings.Contains(path, "/trace") {
		return "view_trace"
	}
	if strings.Contains(path, "/jobs/") {
		switch method {
		case "GET":
			return "view_job"
		case "POST":
			if strings.Contains(path, "/stop") {
				return "stop_job"
			}
			if strings.Contains(path, "/signal") {
				return "signal_job"
			}
			return "create_job"
		case "DELETE":
			return "delete_job"
		}
	}
	return "unknown"
}

// extractResource 从路径提取资源类型和 ID
func extractResource(path string) (resourceType string, resourceID string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")

	if len(parts) >= 3 {
		// /api/jobs/:id -> resourceType=job, resourceID=:id
		if parts[1] == "jobs" {
			return "job", parts[2]
		}
		if parts[1] == "agents" {
			return "agent", parts[2]
		}
	}

	return "unknown", ""
}
