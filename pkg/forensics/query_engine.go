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

package forensics

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"rag-platform/pkg/proof"
)

// JobSource 提供可查询的 Job 元数据。
type JobSource interface {
	ListJobs(ctx context.Context, req QueryRequest) ([]JobSummary, error)
}

// EventSource 提供 Job 的事件流。
type EventSource interface {
	ListEvents(ctx context.Context, jobID string) ([]Event, error)
}

// EvidenceExporter 提供证据包导出能力。
type EvidenceExporter interface {
	ExportEvidenceZip(ctx context.Context, jobID string) ([]byte, error)
}

// QueryEngine 取证查询引擎（2.0-M3）
type QueryEngine struct {
	jobSource   JobSource
	eventSource EventSource
	exporter    EvidenceExporter
}

// NewQueryEngine 创建查询引擎
func NewQueryEngine() *QueryEngine {
	return &QueryEngine{}
}

// WithJobSource 设置 Job 数据源。
func (e *QueryEngine) WithJobSource(source JobSource) *QueryEngine {
	e.jobSource = source
	return e
}

// WithEventSource 设置事件数据源。
func (e *QueryEngine) WithEventSource(source EventSource) *QueryEngine {
	e.eventSource = source
	return e
}

// WithExporter 设置证据导出器。
func (e *QueryEngine) WithExporter(exporter EvidenceExporter) *QueryEngine {
	e.exporter = exporter
	return e
}

// Query 执行取证查询
func (e *QueryEngine) Query(ctx context.Context, req QueryRequest) (*QueryResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	if e.jobSource == nil {
		return &QueryResponse{Jobs: []JobSummary{}, TotalCount: 0, Page: offset / limit}, nil
	}

	jobs, err := e.jobSource.ListJobs(ctx, req)
	if err != nil {
		return nil, err
	}

	statusFilter := make(map[string]struct{}, len(req.StatusFilter))
	for _, s := range req.StatusFilter {
		statusFilter[strings.ToLower(strings.TrimSpace(s))] = struct{}{}
	}

	filtered := make([]JobSummary, 0, len(jobs))
	for _, j := range jobs {
		if !req.TimeRange.Start.IsZero() && j.CreatedAt.Before(req.TimeRange.Start) {
			continue
		}
		if !req.TimeRange.End.IsZero() && j.CreatedAt.After(req.TimeRange.End) {
			continue
		}
		if len(statusFilter) > 0 {
			if _, ok := statusFilter[strings.ToLower(j.Status)]; !ok {
				continue
			}
		}

		toolCalls := append([]string(nil), j.ToolCalls...)
		keyEvents := append([]string(nil), j.KeyEvents...)
		eventCount := j.EventCount
		if e.eventSource != nil && (len(req.ToolFilter) > 0 || len(req.EventFilter) > 0 || eventCount == 0) {
			events, err := e.eventSource.ListEvents(ctx, j.JobID)
			if err != nil {
				return nil, err
			}
			eventCount = len(events)
			if len(req.ToolFilter) > 0 || len(toolCalls) == 0 {
				toolCalls = extractToolCalls(events)
			}
			if len(req.EventFilter) > 0 || len(keyEvents) == 0 {
				keyEvents = extractKeyEvents(events)
			}
		}

		if len(req.ToolFilter) > 0 && !matchAnyToolFilter(toolCalls, req.ToolFilter) {
			continue
		}
		if len(req.EventFilter) > 0 && !matchAnyEventFilter(keyEvents, req.EventFilter) {
			continue
		}

		j.ToolCalls = toolCalls
		j.KeyEvents = keyEvents
		j.EventCount = eventCount
		filtered = append(filtered, j)
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})

	total := len(filtered)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}

	return &QueryResponse{
		Jobs:       filtered[offset:end],
		TotalCount: total,
		Page:       offset / limit,
	}, nil
}

// BatchExport 批量导出证据包
func (e *QueryEngine) BatchExport(ctx context.Context, jobIDs []string) (map[string][]byte, error) {
	result := make(map[string][]byte)
	if e.exporter == nil {
		return result, nil
	}

	seen := make(map[string]struct{}, len(jobIDs))
	for _, raw := range jobIDs {
		jobID := strings.TrimSpace(raw)
		if jobID == "" {
			continue
		}
		if _, ok := seen[jobID]; ok {
			continue
		}
		seen[jobID] = struct{}{}

		zipBytes, err := e.exporter.ExportEvidenceZip(ctx, jobID)
		if err != nil {
			return nil, err
		}
		result[jobID] = zipBytes
	}

	return result, nil
}

// ConsistencyCheck 检查证据链一致性
func (e *QueryEngine) ConsistencyCheck(ctx context.Context, jobID string) (*ConsistencyReport, error) {
	report := &ConsistencyReport{
		JobID:            jobID,
		HashChainValid:   true,
		LedgerConsistent: true,
		EvidenceComplete: true,
		Issues:           []string{},
	}

	if e.exporter == nil || strings.TrimSpace(jobID) == "" {
		return report, nil
	}

	zipBytes, err := e.exporter.ExportEvidenceZip(ctx, jobID)
	if err != nil {
		report.HashChainValid = false
		report.LedgerConsistent = false
		report.EvidenceComplete = false
		report.Issues = []string{err.Error()}
		return report, nil
	}

	verify := proof.VerifyEvidenceZip(zipBytes)
	report.HashChainValid = verify.HashChainValid
	report.LedgerConsistent = verify.LedgerValid
	report.EvidenceComplete = verify.ManifestValid && verify.EventsValid
	if verify.OK {
		report.Issues = []string{}
	} else {
		report.Issues = append([]string(nil), verify.Errors...)
	}
	return report, nil
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

// matchToolFilter 检查 tool 名称是否匹配过滤器
func matchToolFilter(toolName string, filters []string) bool {
	if len(filters) == 0 {
		return true
	}

	for _, filter := range filters {
		// 支持通配符（简化实现）
		if strings.HasSuffix(filter, "*") {
			prefix := strings.TrimSuffix(filter, "*")
			if strings.HasPrefix(toolName, prefix) {
				return true
			}
		} else if toolName == filter {
			return true
		}
	}

	return false
}

// matchEventFilter 检查事件类型是否匹配过滤器
func matchEventFilter(eventType string, filters []string) bool {
	if len(filters) == 0 {
		return true
	}

	for _, filter := range filters {
		if eventType == filter {
			return true
		}
	}

	return false
}

// extractToolCalls 从事件流提取调用过的 tools
func extractToolCalls(events []Event) []string {
	toolSet := make(map[string]bool)

	for _, event := range events {
		if event.Type == "tool_invocation_finished" {
			var payload map[string]interface{}
			if err := json.Unmarshal(event.Payload, &payload); err == nil {
				if toolName, ok := payload["tool_name"].(string); ok && toolName != "" {
					toolSet[toolName] = true
				}
			}
		}
	}

	tools := []string{}
	for tool := range toolSet {
		tools = append(tools, tool)
	}

	return tools
}

// extractKeyEvents 从事件流提取关键事件
func extractKeyEvents(events []Event) []string {
	keyEventTypes := map[string]bool{
		"critical_decision_made": true,
		"human_approval_given":   true,
		"payment_executed":       true,
		"email_sent":             true,
	}

	eventSet := make(map[string]bool)

	for _, event := range events {
		if keyEventTypes[event.Type] {
			eventSet[event.Type] = true
		}
	}

	eventsList := []string{}
	for eventType := range eventSet {
		eventsList = append(eventsList, eventType)
	}

	return eventsList
}

// Event 事件（简化结构）
type Event struct {
	ID        string    `json:"id"`
	JobID     string    `json:"job_id"`
	Type      string    `json:"type"`
	Payload   []byte    `json:"payload"`
	CreatedAt time.Time `json:"created_at"`
}
