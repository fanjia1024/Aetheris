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
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"rag-platform/internal/agent/job"
)

// ExportJobForensics 导出 job 的完整证据包（ZIP 格式）
// POST /api/jobs/:id/export
func (h *Handler) ExportJobForensics(c context.Context, ctx *app.RequestContext) {
	jobID := ctx.Param("id")
	if jobID == "" {
		ctx.JSON(consts.StatusBadRequest, map[string]string{
			"error": "job_id is required",
		})
		return
	}

	// 导出证据包
	zipData, err := h.buildForensicsPackage(c, jobID)
	if err != nil {
		hlog.CtxErrorf(c, "failed to build forensics package for job %s: %v", jobID, err)
		ctx.JSON(consts.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("failed to build forensics package: %v", err),
		})
		return
	}

	// 返回 ZIP 文件
	filename := fmt.Sprintf("job-%s-forensics-%s.zip", jobID, time.Now().Format("20060102-150405"))
	ctx.Header("Content-Type", "application/zip")
	ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	ctx.Data(consts.StatusOK, "application/zip", zipData)
}

// buildForensicsPackage 构建证据包 ZIP 数据
func (h *Handler) buildForensicsPackage(ctx context.Context, jobID string) ([]byte, error) {
	// 创建 ZIP buffer
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)
	defer zipWriter.Close()

	// 1. 导出事件流 (events.jsonl)
	if err := h.exportEvents(ctx, zipWriter, jobID); err != nil {
		return nil, fmt.Errorf("export events failed: %w", err)
	}

	// 2. 导出 Tool Ledger (tool_ledger.json)
	if err := h.exportToolLedger(ctx, zipWriter, jobID); err != nil {
		return nil, fmt.Errorf("export tool ledger failed: %w", err)
	}

	// 3. 导出 LLM Calls (llm_calls.json)
	if err := h.exportLLMCalls(ctx, zipWriter, jobID); err != nil {
		return nil, fmt.Errorf("export llm calls failed: %w", err)
	}

	// 4. 导出 Evidence Graph (evidence_graph.json)
	if err := h.exportEvidenceGraph(ctx, zipWriter, jobID); err != nil {
		return nil, fmt.Errorf("export evidence graph failed: %w", err)
	}

	// 5. 导出 Metadata (metadata.json)
	if err := h.exportMetadata(ctx, zipWriter, jobID); err != nil {
		return nil, fmt.Errorf("export metadata failed: %w", err)
	}

	// 关闭 ZIP writer
	if err := zipWriter.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// exportEvents 导出事件流为 JSONL 格式
func (h *Handler) exportEvents(ctx context.Context, zipWriter *zip.Writer, jobID string) error {
	events, _, err := h.jobEventStore.ListEvents(ctx, jobID)
	if err != nil {
		return err
	}

	file, err := zipWriter.Create("events.jsonl")
	if err != nil {
		return err
	}

	// 每个事件一行 JSON
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			continue
		}
		if _, err := file.Write(append(data, '\n')); err != nil {
			return err
		}
	}

	return nil
}

// exportToolLedger 导出 Tool 调用记录
func (h *Handler) exportToolLedger(ctx context.Context, zipWriter *zip.Writer, jobID string) error {
	// 从事件流提取 tool invocations（简化实现）
	// TODO: 连接到真实的 ToolInvocationStore
	var toolInvocations []map[string]interface{}

	file, err := zipWriter.Create("tool_ledger.json")
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(toolInvocations, "", "  ")
	if err != nil {
		return err
	}

	_, err = file.Write(data)
	return err
}

// exportLLMCalls 导出 LLM 调用元信息
func (h *Handler) exportLLMCalls(ctx context.Context, zipWriter *zip.Writer, jobID string) error {
	// 从事件流提取 llm_called / llm_returned 事件
	events, _, err := h.jobEventStore.ListEvents(ctx, jobID)
	if err != nil {
		return err
	}

	var llmCalls []map[string]interface{}

	for _, event := range events {
		if event.Type == "llm_called" || event.Type == "llm_returned" {
			var payload map[string]interface{}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				continue
			}

			llmCall := map[string]interface{}{
				"event_type": string(event.Type),
				"created_at": event.CreatedAt,
				"payload":    payload,
			}

			// 提取关键信息
			if model, ok := payload["model"].(string); ok {
				llmCall["model"] = model
			}
			if provider, ok := payload["provider"].(string); ok {
				llmCall["provider"] = provider
			}
			if tokens, ok := payload["tokens"].(map[string]interface{}); ok {
				llmCall["tokens"] = tokens
			}

			llmCalls = append(llmCalls, llmCall)
		}
	}

	file, err := zipWriter.Create("llm_calls.json")
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(llmCalls, "", "  ")
	if err != nil {
		return err
	}

	_, err = file.Write(data)
	return err
}

// exportEvidenceGraph 导出决策依赖图
func (h *Handler) exportEvidenceGraph(ctx context.Context, zipWriter *zip.Writer, jobID string) error {
	// 从事件流构建决策依赖图
	events, _, err := h.jobEventStore.ListEvents(ctx, jobID)
	if err != nil {
		return err
	}

	// 构建节点关系图：reasoning_snapshot -> tool_invocation -> state_change
	graph := map[string]interface{}{
		"nodes": []map[string]interface{}{},
		"edges": []map[string]interface{}{},
	}

	nodes := []map[string]interface{}{}
	edges := []map[string]interface{}{}

	// 解析事件构建图
	for _, event := range events {
		var payload map[string]interface{}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			continue
		}

		switch event.Type {
		case "reasoning_snapshot_recorded":
			// 推理节点
			node := map[string]interface{}{
				"id":   fmt.Sprintf("reasoning-%s", event.ID),
				"type": "reasoning",
				"data": payload,
			}
			nodes = append(nodes, node)

		case "tool_invocation_finished":
			// Tool 调用节点
			idempotencyKey, _ := payload["idempotency_key"].(string)
			node := map[string]interface{}{
				"id":   fmt.Sprintf("tool-%s", idempotencyKey),
				"type": "tool",
				"data": payload,
			}
			nodes = append(nodes, node)

			// 如果有 step_id，创建边
			if stepID, ok := payload["step_id"].(string); ok && stepID != "" {
				edge := map[string]interface{}{
					"from": fmt.Sprintf("reasoning-%s", stepID),
					"to":   fmt.Sprintf("tool-%s", idempotencyKey),
					"type": "invokes",
				}
				edges = append(edges, edge)
			}

		case "state_changed":
			// 状态变更节点
			nodeID, _ := payload["node_id"].(string)
			node := map[string]interface{}{
				"id":   fmt.Sprintf("state-%s", event.ID),
				"type": "state_change",
				"data": payload,
			}
			nodes = append(nodes, node)

			// 创建从节点到状态变更的边
			if nodeID != "" {
				edge := map[string]interface{}{
					"from": fmt.Sprintf("reasoning-%s", nodeID),
					"to":   fmt.Sprintf("state-%s", event.ID),
					"type": "causes",
				}
				edges = append(edges, edge)
			}
		}
	}

	graph["nodes"] = nodes
	graph["edges"] = edges

	file, err := zipWriter.Create("evidence_graph.json")
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		return err
	}

	_, err = file.Write(data)
	return err
}

// exportMetadata 导出 job 元信息
func (h *Handler) exportMetadata(ctx context.Context, zipWriter *zip.Writer, jobID string) error {
	// 从 job store 获取 job 元信息
	jobInfo, err := h.jobStore.Get(ctx, jobID)
	if err != nil {
		// 如果找不到 job，仍然创建空的 metadata 文件
		jobInfo = &job.Job{
			ID:     jobID,
			Status: 0, // StatusPending
		}
	}

	metadata := map[string]interface{}{
		"job_id":                jobInfo.ID,
		"agent_id":              jobInfo.AgentID,
		"goal":                  jobInfo.Goal,
		"status":                jobInfo.Status,
		"cursor":                jobInfo.Cursor,
		"retry_count":           jobInfo.RetryCount,
		"session_id":            jobInfo.SessionID,
		"created_at":            jobInfo.CreatedAt,
		"updated_at":            jobInfo.UpdatedAt,
		"idempotency_key":       jobInfo.IdempotencyKey,
		"required_capabilities": jobInfo.RequiredCapabilities,
	}

	if !jobInfo.CancelRequestedAt.IsZero() {
		metadata["cancel_requested_at"] = jobInfo.CancelRequestedAt
	}

	file, err := zipWriter.Create("metadata.json")
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}

	_, err = file.Write(data)
	return err
}
