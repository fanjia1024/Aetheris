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
	"strings"
	"time"
)

// QueryEngine 取证查询引擎（2.0-M3）
type QueryEngine struct{}

// NewQueryEngine 创建查询引擎
func NewQueryEngine() *QueryEngine {
	return &QueryEngine{}
}

// Query 执行取证查询
func (e *QueryEngine) Query(ctx context.Context, req QueryRequest) (*QueryResponse, error) {
	// TODO: 实际实现需要：
	// 1. 从 jobMetaStore 按时间范围和 tenant 查询 jobs
	// 2. 对每个 job 加载事件流
	// 3. 提取 tool_calls 和 key_events
	// 4. 应用 tool_filter 和 event_filter
	// 5. 分页返回

	return &QueryResponse{
		Jobs:       []JobSummary{},
		TotalCount: 0,
		Page:       req.Offset / req.Limit,
	}, nil
}

// BatchExport 批量导出证据包
func (e *QueryEngine) BatchExport(ctx context.Context, jobIDs []string) (map[string][]byte, error) {
	result := make(map[string][]byte)

	// TODO: 实际实现需要：
	// 1. 对每个 job_id 调用 proof.ExportEvidenceZip
	// 2. 收集所有 ZIP 文件
	// 3. 打包为大 ZIP（包含多个 job）

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

	// TODO: 实际实现需要：
	// 1. 验证 hash chain（调用 proof.ValidateChain）
	// 2. 验证 ledger 一致性（调用 proof.ValidateLedgerConsistency）
	// 3. 验证 evidence 完整性（所有引用的证据都存在）

	return report, nil
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

	events_list := []string{}
	for eventType := range eventSet {
		events_list = append(events_list, eventType)
	}

	return events_list
}

// Event 事件（简化结构）
type Event struct {
	ID        string    `json:"id"`
	JobID     string    `json:"job_id"`
	Type      string    `json:"type"`
	Payload   []byte    `json:"payload"`
	CreatedAt time.Time `json:"created_at"`
}
