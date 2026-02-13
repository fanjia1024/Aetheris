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

package http

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"rag-platform/pkg/forensics"
)

// ForensicsQuery 取证查询（2.0-M3）
// POST /api/forensics/query
func (h *Handler) ForensicsQuery(c context.Context, ctx *app.RequestContext) {
	var req forensics.QueryRequest
	if err := ctx.BindJSON(&req); err != nil {
		ctx.JSON(consts.StatusBadRequest, map[string]string{
			"error": "invalid request",
		})
		return
	}

	ctx.JSON(consts.StatusNotImplemented, map[string]string{
		"error": "forensics query is not implemented yet",
	})
}

// ForensicsBatchExport 批量导出证据包（2.0-M3）
// POST /api/forensics/batch-export
func (h *Handler) ForensicsBatchExport(c context.Context, ctx *app.RequestContext) {
	var req struct {
		JobIDs    []string `json:"job_ids"`
		Redaction bool     `json:"redaction"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		ctx.JSON(consts.StatusBadRequest, map[string]string{
			"error": "invalid request",
		})
		return
	}

	if len(req.JobIDs) == 0 {
		ctx.JSON(consts.StatusBadRequest, map[string]string{
			"error": "job_ids is required",
		})
		return
	}

	ctx.JSON(consts.StatusNotImplemented, map[string]string{
		"error": "forensics batch export is not implemented yet",
	})
}

// ForensicsExportStatus 查询批量导出状态（2.0-M3）
// GET /api/forensics/export-status/:task_id
func (h *Handler) ForensicsExportStatus(c context.Context, ctx *app.RequestContext) {
	ctx.JSON(consts.StatusNotImplemented, map[string]string{
		"error": "forensics export status is not implemented yet",
	})
}

// ForensicsConsistencyCheck 证据链一致性检查（2.0-M3）
// GET /api/forensics/consistency/:job_id
func (h *Handler) ForensicsConsistencyCheck(c context.Context, ctx *app.RequestContext) {
	ctx.JSON(consts.StatusNotImplemented, map[string]string{
		"error": "forensics consistency check is not implemented yet",
	})
}

// GetJobEvidenceGraph 获取 Evidence Graph（2.0-M3）
// GET /api/jobs/:id/evidence-graph
func (h *Handler) GetJobEvidenceGraph(c context.Context, ctx *app.RequestContext) {
	ctx.JSON(consts.StatusNotImplemented, map[string]string{
		"error": "job evidence graph is not implemented yet",
	})
}

// GetJobAuditLog 获取 Job 的访问审计日志（2.0-M3）
// GET /api/jobs/:id/audit-log
func (h *Handler) GetJobAuditLog(c context.Context, ctx *app.RequestContext) {
	ctx.JSON(consts.StatusNotImplemented, map[string]string{
		"error": "job audit log is not implemented yet",
	})
}
