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
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/google/uuid"

	"rag-platform/internal/agent/job"
	"rag-platform/internal/runtime/jobstore"
	"rag-platform/pkg/auth"
	"rag-platform/pkg/evidence"
	"rag-platform/pkg/forensics"
	"rag-platform/pkg/proof"
)

var (
	forensicsTaskMu sync.RWMutex
	forensicsTasks  = map[string]forensics.BatchExportTask{}
)

func setForensicsTask(task forensics.BatchExportTask) {
	forensicsTaskMu.Lock()
	defer forensicsTaskMu.Unlock()
	forensicsTasks[task.TaskID] = task
}

func getForensicsTask(taskID string) (forensics.BatchExportTask, bool) {
	forensicsTaskMu.RLock()
	defer forensicsTaskMu.RUnlock()
	t, ok := forensicsTasks[taskID]
	return t, ok
}

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
	if h.jobStore == nil || h.jobEventStore == nil {
		ctx.JSON(consts.StatusServiceUnavailable, map[string]string{
			"error": "forensics query requires job and event stores",
		})
		return
	}

	tenantID := strings.TrimSpace(req.TenantID)
	if tenantID == "" {
		tenantID = auth.GetTenantID(c)
	}
	if tenantID == "" {
		tenantID = "default"
	}

	if len(req.AgentFilter) == 0 {
		ctx.JSON(consts.StatusBadRequest, map[string]string{
			"error": "agent_filter is required in current implementation",
		})
		return
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	statusFilter := make(map[string]struct{}, len(req.StatusFilter))
	for _, s := range req.StatusFilter {
		statusFilter[strings.ToLower(strings.TrimSpace(s))] = struct{}{}
	}

	jobMap := make(map[string]*job.Job)
	for _, agentID := range req.AgentFilter {
		jobs, err := h.jobStore.ListByAgent(c, strings.TrimSpace(agentID), tenantID)
		if err != nil {
			ctx.JSON(consts.StatusInternalServerError, map[string]string{
				"error": fmt.Sprintf("list jobs failed for agent %s: %v", agentID, err),
			})
			return
		}
		for _, j := range jobs {
			if j != nil {
				jobMap[j.ID] = j
			}
		}
	}

	summaries := make([]forensics.JobSummary, 0, len(jobMap))
	for _, j := range jobMap {
		if !req.TimeRange.Start.IsZero() && j.CreatedAt.Before(req.TimeRange.Start) {
			continue
		}
		if !req.TimeRange.End.IsZero() && j.CreatedAt.After(req.TimeRange.End) {
			continue
		}
		if len(statusFilter) > 0 {
			if _, ok := statusFilter[strings.ToLower(j.Status.String())]; !ok {
				continue
			}
		}

		events, _, err := h.jobEventStore.ListEvents(c, j.ID)
		if err != nil {
			ctx.JSON(consts.StatusInternalServerError, map[string]string{
				"error": fmt.Sprintf("list events failed for job %s: %v", j.ID, err),
			})
			return
		}

		toolCalls := extractToolCallsFromJobEvents(events)
		keyEvents := extractKeyEventsFromJobEvents(events)
		if !matchAnyToolFilter(toolCalls, req.ToolFilter) {
			continue
		}
		if !matchAnyEventFilter(keyEvents, req.EventFilter) {
			continue
		}

		summaries = append(summaries, forensics.JobSummary{
			JobID:      j.ID,
			AgentID:    j.AgentID,
			TenantID:   j.TenantID,
			CreatedAt:  j.CreatedAt,
			Status:     j.Status.String(),
			EventCount: len(events),
			ToolCalls:  toolCalls,
			KeyEvents:  keyEvents,
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].CreatedAt.After(summaries[j].CreatedAt)
	})

	total := len(summaries)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}

	ctx.JSON(consts.StatusOK, forensics.QueryResponse{
		Jobs:       summaries[offset:end],
		TotalCount: total,
		Page:       offset / limit,
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
	if h.jobEventStore == nil {
		ctx.JSON(consts.StatusServiceUnavailable, map[string]string{
			"error": "forensics export requires job event store",
		})
		return
	}

	taskID := "task_" + uuid.NewString()
	now := time.Now().UTC()
	task := forensics.BatchExportTask{
		TaskID:    taskID,
		JobIDs:    req.JobIDs,
		Status:    "processing",
		Progress:  0,
		CreatedAt: now,
		UpdatedAt: now,
	}
	setForensicsTask(task)

	go func(jobIDs []string, id string) {
		var err error
		defer func() {
			t, ok := getForensicsTask(id)
			if !ok {
				return
			}
			t.UpdatedAt = time.Now().UTC()
			if err != nil {
				t.Status = "failed"
				t.Error = err.Error()
			} else {
				t.Status = "completed"
				t.Progress = 100
			}
			setForensicsTask(t)
		}()

		if _, err = h.buildBatchForensicsPackage(c, jobIDs); err != nil {
			return
		}
	}(append([]string(nil), req.JobIDs...), taskID)

	ctx.JSON(consts.StatusAccepted, map[string]interface{}{
		"task_id":  taskID,
		"status":   "processing",
		"poll_url": fmt.Sprintf("/api/forensics/export-status/%s", taskID),
	})
}

// ForensicsExportStatus 查询批量导出状态（2.0-M3）
// GET /api/forensics/export-status/:task_id
func (h *Handler) ForensicsExportStatus(c context.Context, ctx *app.RequestContext) {
	taskID := strings.TrimSpace(ctx.Param("task_id"))
	if taskID == "" {
		ctx.JSON(consts.StatusBadRequest, map[string]string{"error": "task_id is required"})
		return
	}
	task, ok := getForensicsTask(taskID)
	if !ok {
		ctx.JSON(consts.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}
	ctx.JSON(consts.StatusOK, task)
}

// ForensicsConsistencyCheck 证据链一致性检查（2.0-M3）
// GET /api/forensics/consistency/:job_id
func (h *Handler) ForensicsConsistencyCheck(c context.Context, ctx *app.RequestContext) {
	jobID := strings.TrimSpace(ctx.Param("job_id"))
	if jobID == "" {
		ctx.JSON(consts.StatusBadRequest, map[string]string{"error": "job_id is required"})
		return
	}
	if h.jobStore != nil {
		if _, ok := h.getJobAndCheckTenant(c, ctx, jobID); !ok {
			return
		}
	}

	zipBytes, err := h.buildForensicsPackage(c, jobID)
	if err != nil {
		ctx.JSON(consts.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("build forensics package failed: %v", err),
		})
		return
	}

	verifyResult := proof.VerifyEvidenceZip(zipBytes)
	report := forensics.ConsistencyReport{
		JobID:            jobID,
		HashChainValid:   verifyResult.HashChainValid,
		LedgerConsistent: verifyResult.LedgerValid,
		EvidenceComplete: verifyResult.ManifestValid && verifyResult.EventsValid,
		Issues:           append([]string(nil), verifyResult.Errors...),
	}
	if verifyResult.OK {
		report.Issues = []string{}
	}
	ctx.JSON(consts.StatusOK, report)
}

// GetJobEvidenceGraph 获取 Evidence Graph（2.0-M3）
// GET /api/jobs/:id/evidence-graph
func (h *Handler) GetJobEvidenceGraph(c context.Context, ctx *app.RequestContext) {
	jobID := strings.TrimSpace(ctx.Param("id"))
	if jobID == "" {
		ctx.JSON(consts.StatusBadRequest, map[string]string{"error": "job_id is required"})
		return
	}
	if h.jobEventStore == nil {
		ctx.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "job event store is not configured"})
		return
	}
	if h.jobStore != nil {
		if _, ok := h.getJobAndCheckTenant(c, ctx, jobID); !ok {
			return
		}
	}

	events, _, err := h.jobEventStore.ListEvents(c, jobID)
	if err != nil {
		ctx.JSON(consts.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("list events failed: %v", err),
		})
		return
	}

	ev := make([]evidence.Event, 0, len(events))
	for _, e := range events {
		ev = append(ev, evidence.Event{
			ID:        e.ID,
			JobID:     e.JobID,
			Type:      string(e.Type),
			Payload:   append([]byte(nil), e.Payload...),
			CreatedAt: e.CreatedAt,
		})
	}
	graph, err := evidence.NewBuilder().BuildFromEvents(ev)
	if err != nil {
		ctx.JSON(consts.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("build evidence graph failed: %v", err),
		})
		return
	}

	ctx.JSON(consts.StatusOK, graph)
}

// GetJobAuditLog 获取 Job 的访问审计日志（2.0-M3）
// GET /api/jobs/:id/audit-log
func (h *Handler) GetJobAuditLog(c context.Context, ctx *app.RequestContext) {
	jobID := strings.TrimSpace(ctx.Param("id"))
	if jobID == "" {
		ctx.JSON(consts.StatusBadRequest, map[string]string{"error": "job_id is required"})
		return
	}
	if h.jobEventStore == nil {
		ctx.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "job event store is not configured"})
		return
	}
	if h.jobStore != nil {
		if _, ok := h.getJobAndCheckTenant(c, ctx, jobID); !ok {
			return
		}
	}

	events, _, err := h.jobEventStore.ListEvents(c, jobID)
	if err != nil {
		ctx.JSON(consts.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("list events failed: %v", err),
		})
		return
	}

	auditLogs := make([]map[string]interface{}, 0)
	for _, e := range events {
		if e.Type != jobstore.AccessAudited {
			continue
		}
		entry := map[string]interface{}{
			"event_id":   e.ID,
			"job_id":     e.JobID,
			"created_at": e.CreatedAt,
			"type":       e.Type,
		}
		if len(e.Payload) > 0 {
			var payload map[string]interface{}
			if err := json.Unmarshal(e.Payload, &payload); err == nil {
				entry["payload"] = payload
			} else {
				entry["payload_raw"] = string(e.Payload)
			}
		}
		auditLogs = append(auditLogs, entry)
	}

	ctx.JSON(consts.StatusOK, map[string]interface{}{
		"job_id": jobID,
		"count":  len(auditLogs),
		"items":  auditLogs,
	})
}

func (h *Handler) buildBatchForensicsPackage(ctx context.Context, jobIDs []string) ([]byte, error) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	for i, jobID := range jobIDs {
		zipBytes, err := h.buildForensicsPackage(ctx, jobID)
		if err != nil {
			return nil, fmt.Errorf("export job %s failed: %w", jobID, err)
		}
		name := fmt.Sprintf("job_%03d_%s.zip", i+1, sanitizeFileName(jobID))
		w, err := zw.Create(name)
		if err != nil {
			return nil, fmt.Errorf("create zip entry %s failed: %w", name, err)
		}
		if _, err := w.Write(zipBytes); err != nil {
			return nil, fmt.Errorf("write zip entry %s failed: %w", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("close batch zip failed: %w", err)
	}
	return buf.Bytes(), nil
}

func sanitizeFileName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	if s == "" {
		return "unknown"
	}
	return s
}

func extractToolCallsFromJobEvents(events []jobstore.JobEvent) []string {
	toolSet := make(map[string]struct{})
	for _, event := range events {
		if event.Type != jobstore.ToolInvocationFinished {
			continue
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			continue
		}
		if toolName, ok := payload["tool_name"].(string); ok && toolName != "" {
			toolSet[toolName] = struct{}{}
		}
	}
	tools := make([]string, 0, len(toolSet))
	for tool := range toolSet {
		tools = append(tools, tool)
	}
	sort.Strings(tools)
	return tools
}

func extractKeyEventsFromJobEvents(events []jobstore.JobEvent) []string {
	keyEventTypes := map[jobstore.EventType]struct{}{
		jobstore.CriticalDecisionMade: {},
		jobstore.HumanApprovalGiven:   {},
		jobstore.PaymentExecuted:      {},
		jobstore.EmailSent:            {},
	}
	eventSet := make(map[string]struct{})
	for _, event := range events {
		if _, ok := keyEventTypes[event.Type]; ok {
			eventSet[string(event.Type)] = struct{}{}
		}
	}
	out := make([]string, 0, len(eventSet))
	for eventType := range eventSet {
		out = append(out, eventType)
	}
	sort.Strings(out)
	return out
}

func matchAnyToolFilter(toolNames []string, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	for _, toolName := range toolNames {
		if matchToolFilter(toolName, filters) {
			return true
		}
	}
	return false
}

func matchAnyEventFilter(eventTypes []string, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	for _, eventType := range eventTypes {
		if matchEventFilter(eventType, filters) {
			return true
		}
	}
	return false
}

func matchToolFilter(toolName string, filters []string) bool {
	for _, filter := range filters {
		filter = strings.TrimSpace(filter)
		if filter == "" {
			continue
		}
		if strings.HasSuffix(filter, "*") {
			prefix := strings.TrimSuffix(filter, "*")
			if strings.HasPrefix(toolName, prefix) {
				return true
			}
			continue
		}
		if toolName == filter {
			return true
		}
	}
	return false
}

func matchEventFilter(eventType string, filters []string) bool {
	for _, filter := range filters {
		if strings.TrimSpace(filter) == eventType {
			return true
		}
	}
	return false
}
