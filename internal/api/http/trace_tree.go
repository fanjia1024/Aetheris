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
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"rag-platform/internal/runtime/jobstore"
)

// ExecutionNode is a node in the execution tree (see design/execution-trace.md).
// Input/Output are set for type=tool from tool_called/tool_returned payloads.
type ExecutionNode struct {
	SpanID         string           `json:"span_id"`
	ParentID       *string          `json:"parent_id,omitempty"`
	Type           string           `json:"type"` // job | plan | node | tool
	NodeID         string           `json:"node_id,omitempty"`
	ToolName       string           `json:"tool_name,omitempty"`
	StartTime      *time.Time       `json:"start_time,omitempty"`
	EndTime        *time.Time       `json:"end_time,omitempty"`
	StepIndex      int              `json:"step_index,omitempty"`
	Input          json.RawMessage  `json:"input,omitempty"`           // tool_called payload input (type=tool)
	Output         json.RawMessage  `json:"output,omitempty"`          // tool_returned payload output (type=tool)
	PayloadSummary string           `json:"payload_summary,omitempty"` // one-line summary for type=node (e.g. llm/workflow)
	Children       []*ExecutionNode `json:"children,omitempty"`
}

// BuildExecutionTree 从事件流推导执行树（兼容无 trace_span_id 的旧事件）
func BuildExecutionTree(events []jobstore.JobEvent) *ExecutionNode {
	root := &ExecutionNode{SpanID: "root", Type: "job"}
	byID := map[string]*ExecutionNode{"root": root}
	// 每个 node 下未闭合的 tool 调用（按顺序），用于 tool_returned 配对
	openToolByNode := map[string][]*ExecutionNode{}

	for i, e := range events {
		stepIndex := i + 1
		var pl map[string]interface{}
		if len(e.Payload) > 0 {
			_ = json.Unmarshal(e.Payload, &pl)
		}
		getStr := func(k string) string {
			if pl == nil {
				return ""
			}
			v, _ := pl[k].(string)
			return v
		}
		getInt := func(k string) int {
			if pl == nil {
				return 0
			}
			switch v := pl[k].(type) {
			case float64:
				return int(v)
			case int:
				return v
			}
			return 0
		}

		switch e.Type {
		case jobstore.PlanGenerated:
			spanID := getStr("trace_span_id")
			if spanID == "" {
				spanID = "plan"
			}
			parentID := getStr("parent_span_id")
			if parentID == "" {
				parentID = "root"
			}
			if byID[spanID] == nil {
				si := stepIndex
				if v := getInt("step_index"); v != 0 {
					si = v
				}
				n := &ExecutionNode{
					SpanID:    spanID,
					ParentID:  &parentID,
					Type:      "plan",
					StartTime: &e.CreatedAt,
					StepIndex: si,
				}
				byID[spanID] = n
				root.Children = append(root.Children, n)
			}
		case jobstore.NodeStarted:
			nodeID := getStr("node_id")
			if nodeID == "" {
				continue
			}
			spanID := getStr("trace_span_id")
			if spanID == "" {
				spanID = nodeID
			}
			parentID := getStr("parent_span_id")
			if parentID == "" {
				parentID = "plan"
			}
			if byID[spanID] == nil {
				si := stepIndex
				if v := getInt("step_index"); v != 0 {
					si = v
				}
				n := &ExecutionNode{
					SpanID:    spanID,
					ParentID:  &parentID,
					Type:      "node",
					NodeID:    nodeID,
					StartTime: &e.CreatedAt,
					StepIndex: si,
				}
				byID[spanID] = n
				if plan := byID["plan"]; plan != nil {
					plan.Children = append(plan.Children, n)
				}
			}
		case jobstore.NodeFinished:
			nodeID := getStr("node_id")
			if nodeID == "" {
				continue
			}
			spanID := getStr("trace_span_id")
			if spanID == "" {
				spanID = nodeID
			}
			if n := byID[spanID]; n != nil {
				t := e.CreatedAt
				n.EndTime = &t
				if raw := pl["payload_results"]; raw != nil {
					if b, err := json.Marshal(raw); err == nil && len(b) > 0 {
						const maxSummary = 120
						if len(b) > maxSummary {
							n.PayloadSummary = string(b[:maxSummary]) + "..."
						} else {
							n.PayloadSummary = string(b)
						}
					}
				}
			}
		case jobstore.ToolCalled:
			nodeID := getStr("node_id")
			toolName := getStr("tool_name")
			spanID := getStr("trace_span_id")
			if spanID == "" {
				spanID = nodeID + ":tool:" + toolName + ":" + strconv.Itoa(stepIndex)
			}
			parentID := getStr("parent_span_id")
			if parentID == "" {
				parentID = nodeID
			}
			if byID[spanID] == nil {
				si := stepIndex
				if v := getInt("step_index"); v != 0 {
					si = v
				}
				n := &ExecutionNode{
					SpanID:    spanID,
					ParentID:  &parentID,
					Type:      "tool",
					NodeID:    nodeID,
					ToolName:  toolName,
					StartTime: &e.CreatedAt,
					StepIndex: si,
				}
				if in := pl["input"]; in != nil {
					if b, err := json.Marshal(in); err == nil {
						n.Input = b
					}
				}
				byID[spanID] = n
				openToolByNode[nodeID] = append(openToolByNode[nodeID], n)
				if node := byID[nodeID]; node != nil {
					node.Children = append(node.Children, n)
				}
			}
		case jobstore.ToolReturned:
			nodeID := getStr("node_id")
			open := openToolByNode[nodeID]
			if len(open) > 0 {
				last := open[len(open)-1]
				t := e.CreatedAt
				last.EndTime = &t
				if out := pl["output"]; out != nil {
					if b, err := json.Marshal(out); err == nil {
						last.Output = b
					}
				}
				openToolByNode[nodeID] = open[:len(open)-1]
			}
		default:
			// job_created, job_completed, job_failed, job_cancelled 不建树节点
		}
	}

	return root
}

// StepInfo is one row in the step timeline (plan, node, or tool).
type StepInfo struct {
	SpanID     string          `json:"span_id"`
	Type       string          `json:"type"`
	Label      string          `json:"label"`
	StartTime  *time.Time      `json:"start_time,omitempty"`
	EndTime    *time.Time      `json:"end_time,omitempty"`
	DurationMs int64           `json:"duration_ms,omitempty"`
	Input      json.RawMessage `json:"input,omitempty"`
	Output     json.RawMessage `json:"output,omitempty"`
}

// FlattenSteps returns steps in DFS order for the step timeline UI.
func FlattenSteps(root *ExecutionNode) []StepInfo {
	var out []StepInfo
	if root == nil {
		return out
	}
	var walk func(*ExecutionNode)
	walk = func(n *ExecutionNode) {
		if n.Type == "job" {
			for _, c := range n.Children {
				walk(c)
			}
			return
		}
		label := n.SpanID
		switch n.Type {
		case "plan":
			label = "Plan"
		case "node":
			label = "Node " + n.NodeID
		case "tool":
			label = "Tool " + n.ToolName
		}
		var durMs int64
		if n.StartTime != nil && n.EndTime != nil {
			durMs = n.EndTime.Sub(*n.StartTime).Milliseconds()
		}
		out = append(out, StepInfo{
			SpanID:     n.SpanID,
			Type:       n.Type,
			Label:      label,
			StartTime:  n.StartTime,
			EndTime:    n.EndTime,
			DurationMs: durMs,
			Input:      n.Input,
			Output:     n.Output,
		})
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)
	return out
}

// ExecutionTreeToHTML renders the execution tree as nested HTML (User → Plan → Node → Tool).
func ExecutionTreeToHTML(root *ExecutionNode) string {
	if root == nil {
		return ""
	}
	return renderNodeHTML(root, 0)
}

func renderNodeHTML(n *ExecutionNode, depth int) string {
	label := n.SpanID
	switch n.Type {
	case "job":
		label = "Job (root)"
	case "plan":
		label = "Plan"
	case "node":
		label = fmt.Sprintf("Node %s", n.NodeID)
		if n.StartTime != nil {
			label += " " + n.StartTime.Format("15:04:05")
		}
		if n.EndTime != nil && n.StartTime != nil {
			label += fmt.Sprintf(" (%dms)", n.EndTime.Sub(*n.StartTime).Milliseconds())
		}
	case "tool":
		label = fmt.Sprintf("Tool %s", n.ToolName)
		if n.StartTime != nil {
			label += " " + n.StartTime.Format("15:04:05")
		}
		if n.EndTime != nil && n.StartTime != nil {
			label += fmt.Sprintf(" (%dms)", n.EndTime.Sub(*n.StartTime).Milliseconds())
		}
	}
	out := fmt.Sprintf("<li><b>%s</b> <code>%s</code>", label, n.Type)
	if len(n.Children) > 0 {
		out += "<ul>"
		for _, c := range n.Children {
			out += renderNodeHTML(c, depth+1)
		}
		out += "</ul>"
	}
	out += "</li>"
	return out
}
