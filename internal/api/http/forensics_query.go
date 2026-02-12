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
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
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

	// TODO: 实现查询引擎集成
	queryEngine := forensics.NewQueryEngine()
	result, err := queryEngine.Query(c, req)
	if err != nil {
		hlog.CtxErrorf(c, "forensics query failed: %v", err)
		ctx.JSON(consts.StatusInternalServerError, map[string]string{
			"error": "query failed",
		})
		return
	}

	ctx.JSON(consts.StatusOK, result)
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

	// 异步处理（大量 jobs）
	taskID := "task-" + req.JobIDs[0] // 简化实现

	// TODO: 启动后台任务
	// go h.doBatchExport(taskID, req.JobIDs, req.Redaction)

	ctx.JSON(consts.StatusAccepted, map[string]string{
		"task_id":  taskID,
		"status":   "processing",
		"poll_url": "/api/forensics/export-status/" + taskID,
	})
}

// ForensicsExportStatus 查询批量导出状态（2.0-M3）
// GET /api/forensics/export-status/:task_id
func (h *Handler) ForensicsExportStatus(c context.Context, ctx *app.RequestContext) {
	taskID := ctx.Param("task_id")

	// TODO: 查询任务状态
	ctx.JSON(consts.StatusOK, map[string]interface{}{
		"task_id":      taskID,
		"status":       "completed",
		"progress":     100,
		"download_url": "/api/forensics/download/" + taskID,
	})
}

// ForensicsConsistencyCheck 证据链一致性检查（2.0-M3）
// GET /api/forensics/consistency/:job_id
func (h *Handler) ForensicsConsistencyCheck(c context.Context, ctx *app.RequestContext) {
	jobID := ctx.Param("job_id")

	queryEngine := forensics.NewQueryEngine()
	report, err := queryEngine.ConsistencyCheck(c, jobID)
	if err != nil {
		hlog.CtxErrorf(c, "consistency check failed: %v", err)
		ctx.JSON(consts.StatusInternalServerError, map[string]string{
			"error": "consistency check failed",
		})
		return
	}

	ctx.JSON(consts.StatusOK, report)
}

// GetJobEvidenceGraph 获取 Evidence Graph（2.0-M3）
// GET /api/jobs/:id/evidence-graph
func (h *Handler) GetJobEvidenceGraph(c context.Context, ctx *app.RequestContext) {
	jobID := ctx.Param("id")

	// 1. 获取事件流
	events, _, err := h.jobEventStore.ListEvents(c, jobID)
	if err != nil {
		hlog.CtxErrorf(c, "failed to list events: %v", err)
		ctx.JSON(consts.StatusInternalServerError, map[string]string{
			"error": "failed to load events",
		})
		return
	}

	// 2. 转换为 evidence.Event
	evidenceEvents := make([]struct {
		ID        string
		JobID     string
		Type      string
		Payload   []byte
		CreatedAt time.Time
	}, len(events))

	for i, e := range events {
		evidenceEvents[i].ID = e.ID
		evidenceEvents[i].JobID = e.JobID
		evidenceEvents[i].Type = string(e.Type)
		evidenceEvents[i].Payload = e.Payload
		evidenceEvents[i].CreatedAt = e.CreatedAt
	}

	// 3. 构建 Evidence Graph
	// TODO: 调用 evidence.Builder.BuildFromEvents

	ctx.JSON(consts.StatusOK, map[string]interface{}{
		"job_id": jobID,
		"graph": map[string]interface{}{
			"nodes": []interface{}{},
			"edges": []interface{}{},
		},
	})
}

// GetJobAuditLog 获取 Job 的访问审计日志（2.0-M3）
// GET /api/jobs/:id/audit-log
func (h *Handler) GetJobAuditLog(c context.Context, ctx *app.RequestContext) {
	jobID := ctx.Param("id")

	// TODO: 从 access_audit_log 表查询该 job 的访问记录

	ctx.JSON(consts.StatusOK, map[string]interface{}{
		"job_id":     jobID,
		"audit_logs": []interface{}{},
	})
}
