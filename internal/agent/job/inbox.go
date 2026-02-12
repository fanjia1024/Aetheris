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

package job

import (
	"context"
	"encoding/json"

	"rag-platform/internal/agent/messaging"
	"rag-platform/internal/runtime/jobstore"
)

// CreateJobFromInbox 从 Agent 收件箱取第一条未消费消息，创建 Job 并 MarkConsumed；供 Worker inbox 轮询实现「message arrival → scheduler → run」（design/plan.md Phase A）
// 返回 jobID, created=true 表示已创建并绑定消息；created=false 表示该 agent 无未消费消息。
func CreateJobFromInbox(
	ctx context.Context,
	agentID string,
	inbox messaging.InboxReader,
	metadataStore JobStore,
	eventStore jobstore.JobStore,
) (jobID string, created bool, err error) {
	if inbox == nil || metadataStore == nil || eventStore == nil {
		return "", false, nil
	}
	msgs, err := inbox.PeekInbox(ctx, agentID, 1)
	if err != nil || len(msgs) == 0 {
		return "", false, err
	}
	msg := msgs[0]
	goal := extractGoalFromMessagePayload(msg.Payload)
	j := &Job{
		AgentID:   agentID,
		Goal:      goal,
		Status:    StatusPending,
		SessionID: agentID,
	}
	jobID, err = metadataStore.Create(ctx, j)
	if err != nil {
		return "", false, err
	}
	payload, _ := json.Marshal(map[string]string{"agent_id": agentID, "goal": goal})
	_, err = eventStore.Append(ctx, jobID, 0, jobstore.JobEvent{
		JobID: jobID, Type: jobstore.JobCreated, Payload: payload,
	})
	if err != nil {
		return jobID, true, err
	}
	_ = inbox.MarkConsumed(ctx, msg.ID, jobID)
	return jobID, true, nil
}

func extractGoalFromMessagePayload(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	if m, ok := payload["message"].(string); ok && m != "" {
		return m
	}
	if b, err := json.Marshal(payload); err == nil {
		return string(b)
	}
	return ""
}
