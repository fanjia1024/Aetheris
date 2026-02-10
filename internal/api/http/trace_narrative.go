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
	"time"

	"rag-platform/internal/runtime/jobstore"
)

// TimelineSegment is one segment on the horizontal timeline (plan, step, retry, recover).
type TimelineSegment struct {
	Type       string     `json:"type"`        // plan | node | tool | recovery
	Label      string     `json:"label"`
	NodeID     string     `json:"node_id,omitempty"`
	StartTime  *time.Time `json:"start_time,omitempty"`
	EndTime    *time.Time `json:"end_time,omitempty"`
	DurationMs int64     `json:"duration_ms,omitempty"`
	Status     string     `json:"status,omitempty"` // ok | failed | retryable
	Attempt    int        `json:"attempt,omitempty"`
	WorkerID   string     `json:"worker_id,omitempty"`
}

// StepNarrative is the narrative view for one step (Temporal-style minimal debug unit).
type StepNarrative struct {
	SpanID         string                 `json:"span_id"`
	Type           string                 `json:"type"` // plan | node | tool
	Label          string                 `json:"label"`
	NodeID         string                 `json:"node_id,omitempty"`
	State          string                 `json:"state,omitempty"`
	ResultType     string                 `json:"result_type,omitempty"` // Phase A 世界语义: pure | success | side_effect_committed | retryable_failure | permanent_failure | compensatable_failure | compensated
	Reason         string                 `json:"reason,omitempty"`
	Attempts       int                    `json:"attempts,omitempty"`
	WorkerID       string                 `json:"worker_id,omitempty"`
	DurationMs     int64                  `json:"duration_ms,omitempty"`
	StartTime      *time.Time             `json:"start_time,omitempty"`
	EndTime        *time.Time             `json:"end_time,omitempty"`
	Reasoning      []ReasoningItem        `json:"reasoning,omitempty"`
	ToolInvocation *ToolInvocationSummary `json:"tool_invocation,omitempty"`
	StateDiff      *StateDiff             `json:"state_diff,omitempty"`
}

// ReasoningItem is one agent thought or decision (from agent_thought_recorded, decision_made, tool_selected).
type ReasoningItem struct {
	Role    string `json:"role,omitempty"`    // reasoning | decision | tool_selected
	Content string `json:"content"`
	Kind    string `json:"kind,omitempty"`
}

// ToolInvocationSummary is from tool_result_summarized (and tool_called/tool_returned).
type ToolInvocationSummary struct {
	ToolName   string          `json:"tool_name"`
	Summary    string          `json:"summary,omitempty"`
	Error      string          `json:"error,omitempty"`
	Idempotent bool            `json:"idempotent,omitempty"`
	Input      json.RawMessage `json:"input,omitempty"`
	Output     json.RawMessage `json:"output,omitempty"`
}

// StateChangeItem 单条外部资源变更（来自 state_changed 事件），供审计
type StateChangeItem struct {
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Operation    string `json:"operation"`
	StepID       string `json:"step_id,omitempty"`
	ToolName     string `json:"tool_name,omitempty"`
	Version      string `json:"version,omitempty"`
	Etag         string `json:"etag,omitempty"`
	ExternalRef  string `json:"external_ref,omitempty"`
}

// StateDiff is memory before/after step (from state_checkpointed).
type StateDiff struct {
	StateBefore     json.RawMessage   `json:"state_before,omitempty"`
	StateAfter      json.RawMessage   `json:"state_after,omitempty"`
	ChangedKeys     []string          `json:"changed_keys,omitempty"`
	ToolSideEffects []string          `json:"tool_side_effects,omitempty"`
	ResourceRefs    []string          `json:"resource_refs,omitempty"`
	StateChanges    []StateChangeItem `json:"state_changes,omitempty"`
}

// Narrative is the full narrative model for the Trace UI (timeline segments + step details).
type Narrative struct {
	TimelineSegments []TimelineSegment `json:"timeline_segments"`
	Steps            []StepNarrative   `json:"steps"`
}

// BuildNarrative builds timeline segments and step narratives from the event stream (v0.9 semantic + existing events).
func BuildNarrative(events []jobstore.JobEvent) *Narrative {
	out := &Narrative{
		TimelineSegments: make([]TimelineSegment, 0),
		Steps:            make([]StepNarrative, 0),
	}
	nodeStartTime := make(map[string]time.Time)
	nodeStartPayload := make(map[string]map[string]interface{})
	// step index by node_id for attaching reasoning/tool/state to the right step
	spanToStepIndex := make(map[string]int)
	var stepIndex int

	for _, e := range events {
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
		getInt64 := func(k string) int64 {
			if pl == nil {
				return 0
			}
			switch v := pl[k].(type) {
			case float64:
				return int64(v)
			case int64:
				return v
			case int:
				return int64(v)
			}
			return 0
		}

		switch e.Type {
		case jobstore.PlanGenerated:
			startAt := e.CreatedAt
			out.TimelineSegments = append(out.TimelineSegments, TimelineSegment{
				Type:      "plan",
				Label:     "Plan",
				StartTime: &startAt,
				EndTime:   &startAt,
				Status:    "ok",
			})
			out.Steps = append(out.Steps, StepNarrative{
				SpanID: "plan",
				Type:   "plan",
				Label:  "Plan",
				StartTime: &startAt,
				EndTime:   &startAt,
			})
			spanToStepIndex["plan"] = len(out.Steps) - 1
			stepIndex++

		case jobstore.NodeStarted:
			nodeID := getStr("node_id")
			if nodeID == "" {
				continue
			}
			nodeStartTime[nodeID] = e.CreatedAt
			nodeStartPayload[nodeID] = pl
			seg := TimelineSegment{
				Type:      "node",
				Label:     "Node " + nodeID,
				NodeID:    nodeID,
				StartTime: ptrTime(e.CreatedAt),
				Attempt:   getInt("attempt"),
				WorkerID:  getStr("worker_id"),
			}
			if seg.Attempt == 0 {
				seg.Attempt = 1
			}
			out.TimelineSegments = append(out.TimelineSegments, seg)
			out.Steps = append(out.Steps, StepNarrative{
				SpanID:   nodeID,
				Type:     "node",
				Label:    "Node " + nodeID,
				NodeID:   nodeID,
				Attempts: seg.Attempt,
				WorkerID: seg.WorkerID,
				StartTime: &e.CreatedAt,
			})
			spanToStepIndex[nodeID] = len(out.Steps) - 1
			stepIndex++

		case jobstore.NodeFinished:
			nodeID := getStr("node_id")
			if nodeID == "" {
				continue
			}
			endAt := e.CreatedAt
			durMs := getInt64("duration_ms")
			if durMs == 0 {
				if startAt, ok := nodeStartTime[nodeID]; ok {
					durMs = endAt.Sub(startAt).Milliseconds()
				}
			}
			state := getStr("state")
			if state == "" {
				state = "ok"
			}
			resultType := getStr("result_type")
			reason := getStr("reason")
			attempt := getInt("attempt")
			if attempt == 0 {
				attempt = 1
			}
			// update last segment for this node (the one we added at NodeStarted)
			for i := len(out.TimelineSegments) - 1; i >= 0; i-- {
				if out.TimelineSegments[i].NodeID == nodeID && out.TimelineSegments[i].EndTime == nil {
					out.TimelineSegments[i].EndTime = &endAt
					out.TimelineSegments[i].DurationMs = durMs
					out.TimelineSegments[i].Status = state
					out.TimelineSegments[i].Attempt = attempt
					break
				}
			}
			if idx, ok := spanToStepIndex[nodeID]; ok && idx < len(out.Steps) {
				out.Steps[idx].EndTime = &endAt
				out.Steps[idx].DurationMs = durMs
				out.Steps[idx].State = state
				out.Steps[idx].ResultType = resultType
				out.Steps[idx].Reason = reason
				out.Steps[idx].Attempts = attempt
			}
			delete(nodeStartTime, nodeID)
			delete(nodeStartPayload, nodeID)

		case jobstore.ToolCalled:
			nodeID := getStr("node_id")
			toolName := getStr("tool_name")
			spanID := nodeID + ":tool:" + toolName
			var inputRaw json.RawMessage
			if pl["input"] != nil {
				if b, err := json.Marshal(pl["input"]); err == nil {
					inputRaw = b
				}
			}
			out.TimelineSegments = append(out.TimelineSegments, TimelineSegment{
				Type:      "tool",
				Label:     "Tool " + toolName,
				NodeID:    nodeID,
				StartTime: ptrTime(e.CreatedAt),
			})
			toolInv := &ToolInvocationSummary{ToolName: toolName, Input: inputRaw}
			out.Steps = append(out.Steps, StepNarrative{
				SpanID:         spanID,
				Type:           "tool",
				Label:          "Tool " + toolName,
				NodeID:         nodeID,
				StartTime:      &e.CreatedAt,
				ToolInvocation: toolInv,
			})
			spanToStepIndex[spanID] = len(out.Steps) - 1
			stepIndex++

		case jobstore.ToolReturned:
			nodeID := getStr("node_id")
			var outputRaw json.RawMessage
			if pl["output"] != nil {
				if b, err := json.Marshal(pl["output"]); err == nil {
					outputRaw = b
				}
			}
			// match last open tool segment for this node and set end time
			for i := len(out.TimelineSegments) - 1; i >= 0; i-- {
				if out.TimelineSegments[i].Type == "tool" && out.TimelineSegments[i].NodeID == nodeID && out.TimelineSegments[i].EndTime == nil {
					out.TimelineSegments[i].EndTime = ptrTime(e.CreatedAt)
					if out.TimelineSegments[i].StartTime != nil {
						out.TimelineSegments[i].DurationMs = e.CreatedAt.Sub(*out.TimelineSegments[i].StartTime).Milliseconds()
					}
					out.TimelineSegments[i].Status = "ok"
					break
				}
			}
			for i := len(out.Steps) - 1; i >= 0; i-- {
				if out.Steps[i].Type == "tool" && out.Steps[i].NodeID == nodeID && out.Steps[i].EndTime == nil {
					out.Steps[i].EndTime = ptrTime(e.CreatedAt)
					if out.Steps[i].StartTime != nil {
						out.Steps[i].DurationMs = e.CreatedAt.Sub(*out.Steps[i].StartTime).Milliseconds()
					}
					out.Steps[i].State = "ok"
					if out.Steps[i].ToolInvocation != nil {
						out.Steps[i].ToolInvocation.Output = outputRaw
					}
					break
				}
			}

		case jobstore.StateCheckpointed:
			nodeID := getStr("node_id")
			idx, ok := spanToStepIndex[nodeID]
			if !ok || idx >= len(out.Steps) {
				continue
			}
			var stateBefore, stateAfter json.RawMessage
			if pl["state_before"] != nil {
				if b, err := json.Marshal(pl["state_before"]); err == nil {
					stateBefore = b
				}
			}
			if pl["state_after"] != nil {
				if b, err := json.Marshal(pl["state_after"]); err == nil {
					stateAfter = b
				}
			}
			changed := diffKeys(stateBefore, stateAfter)
			if payloadChanged, ok := pl["changed_keys"]; ok {
				if arr, ok := payloadChanged.([]interface{}); ok {
					changed = make([]string, 0, len(arr))
					for _, v := range arr {
						if s, ok := v.(string); ok {
							changed = append(changed, s)
						}
					}
				}
			}
			toolSideEffects := parseStringSlice(pl, "tool_side_effects")
			resourceRefs := parseStringSlice(pl, "resource_refs")
			stateChanges := parseStateChanges(pl["state_changes"])
			out.Steps[idx].StateDiff = &StateDiff{
				StateBefore:     stateBefore,
				StateAfter:      stateAfter,
				ChangedKeys:     changed,
				ToolSideEffects: toolSideEffects,
				ResourceRefs:    resourceRefs,
				StateChanges:    stateChanges,
			}
		case jobstore.StateChanged:
			nodeID := getStr("node_id")
			idx, ok := spanToStepIndex[nodeID]
			if !ok || idx >= len(out.Steps) {
				continue
			}
			stateChanges := parseStateChanges(pl["state_changes"])
			if len(stateChanges) == 0 {
				continue
			}
			if out.Steps[idx].StateDiff == nil {
				out.Steps[idx].StateDiff = &StateDiff{}
			}
			out.Steps[idx].StateDiff.StateChanges = append(out.Steps[idx].StateDiff.StateChanges, stateChanges...)
		case jobstore.AgentThoughtRecorded, jobstore.DecisionMade:
			nodeID := getStr("node_id")
			content := getStr("content")
			role := getStr("role")
			if role == "" {
				role = "reasoning"
			}
			if e.Type == jobstore.DecisionMade {
				role = "decision"
			}
			kind := getStr("kind")
			// attach to most recent step for this node_id
			if nodeID != "" {
				if idx, ok := spanToStepIndex[nodeID]; ok && idx < len(out.Steps) {
					out.Steps[idx].Reasoning = append(out.Steps[idx].Reasoning, ReasoningItem{Role: role, Content: content, Kind: kind})
				}
			}
		case jobstore.ToolSelected:
			nodeID := getStr("node_id")
			toolName := getStr("tool_name")
			reason := getStr("reason")
			content := "Tool selected: " + toolName
			if reason != "" {
				content += " — " + reason
			}
			if nodeID != "" {
				if idx, ok := spanToStepIndex[nodeID]; ok && idx < len(out.Steps) {
					out.Steps[idx].Reasoning = append(out.Steps[idx].Reasoning, ReasoningItem{Role: "tool_selected", Content: content})
				}
			}
		case jobstore.ToolResultSummarized:
			nodeID := getStr("node_id")
			toolName := getStr("tool_name")
			summary := getStr("summary")
			errMsg := getStr("error")
			idempotent := false
			if pl != nil {
				if v, ok := pl["idempotent"].(bool); ok {
					idempotent = v
				}
			}
			spanID := nodeID + ":tool:" + toolName
			if idx, ok := spanToStepIndex[spanID]; ok && idx < len(out.Steps) {
				inv := out.Steps[idx].ToolInvocation
				if inv == nil {
					inv = &ToolInvocationSummary{ToolName: toolName}
					out.Steps[idx].ToolInvocation = inv
				}
				inv.Summary = summary
				inv.Error = errMsg
				inv.Idempotent = idempotent
			}
		case jobstore.RecoveryStarted:
			reason := getStr("reason")
			label := "Recovery"
			if reason != "" {
				label += " (" + reason + ")"
			}
			out.TimelineSegments = append(out.TimelineSegments, TimelineSegment{
				Type:      "recovery",
				Label:     label,
				NodeID:    getStr("node_id"),
				StartTime: ptrTime(e.CreatedAt),
				Status:    "running",
			})
		case jobstore.RecoveryCompleted:
			for i := len(out.TimelineSegments) - 1; i >= 0; i-- {
				if out.TimelineSegments[i].Type == "recovery" && out.TimelineSegments[i].EndTime == nil {
					out.TimelineSegments[i].EndTime = ptrTime(e.CreatedAt)
					if out.TimelineSegments[i].StartTime != nil {
						out.TimelineSegments[i].DurationMs = e.CreatedAt.Sub(*out.TimelineSegments[i].StartTime).Milliseconds()
					}
					success := false
					if pl != nil {
						if v, ok := pl["success"].(bool); ok {
							success = v
						}
					}
					if success {
						out.TimelineSegments[i].Status = "ok"
					} else {
						out.TimelineSegments[i].Status = "failed"
					}
					break
				}
			}
		case jobstore.StepCompensated:
			out.TimelineSegments = append(out.TimelineSegments, TimelineSegment{
				Type:      "recovery",
				Label:     "Compensated",
				NodeID:    getStr("node_id"),
				StartTime: ptrTime(e.CreatedAt),
				EndTime:   ptrTime(e.CreatedAt),
				Status:    "ok",
			})
		default:
			// other events (job_created, command_*, job_completed, etc.) do not add segments
		}
	}
	return out
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func parseStringSlice(pl map[string]interface{}, key string) []string {
	if pl == nil {
		return nil
	}
	v, ok := pl[key]
	if !ok {
		return nil
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func parseStateChanges(v interface{}) []StateChangeItem {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]StateChangeItem, 0, len(arr))
	for _, x := range arr {
		m, ok := x.(map[string]interface{})
		if !ok {
			continue
		}
		item := StateChangeItem{}
		if s, _ := m["resource_type"].(string); s != "" {
			item.ResourceType = s
		}
		if s, _ := m["resource_id"].(string); s != "" {
			item.ResourceID = s
		}
		if s, _ := m["operation"].(string); s != "" {
			item.Operation = s
		}
		if s, _ := m["step_id"].(string); s != "" {
			item.StepID = s
		}
		if s, _ := m["tool_name"].(string); s != "" {
			item.ToolName = s
		}
		if s, _ := m["version"].(string); s != "" {
			item.Version = s
		}
		if s, _ := m["etag"].(string); s != "" {
			item.Etag = s
		}
		if s, _ := m["external_ref"].(string); s != "" {
			item.ExternalRef = s
		}
		if item.ResourceType != "" || item.ResourceID != "" || item.Operation != "" {
			out = append(out, item)
		}
	}
	return out
}

// diffKeys returns keys that differ between two JSON objects (new or value changed in after).
func diffKeys(before, after json.RawMessage) []string {
	var mBefore, mAfter map[string]interface{}
	_ = json.Unmarshal(before, &mBefore)
	_ = json.Unmarshal(after, &mAfter)
	if mAfter == nil {
		return nil
	}
	var changed []string
	for k := range mAfter {
		var vBefore, vAfter interface{}
		if mBefore != nil {
			vBefore = mBefore[k]
		}
		vAfter = mAfter[k]
		if !jsonEqual(vBefore, vAfter) {
			changed = append(changed, k)
		}
	}
	return changed
}

func jsonEqual(a, b interface{}) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}
