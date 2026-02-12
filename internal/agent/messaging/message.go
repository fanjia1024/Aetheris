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

package messaging

import "time"

// Message Agent 级消息；design/agent-messaging-bus.md；design/plan.md Phase C 增加 CausationID 用于追踪与多 Agent 链
type Message struct {
	ID              string
	FromAgentID     string
	ToAgentID       string
	Channel         string
	Kind            string
	Payload         map[string]any
	CausationID     string // 可选：触发本消息的 message_id 或 job_id，供 Trace 与多 Agent 链（design/plan.md Phase C）
	ScheduledAt     *time.Time
	ExpiresAt       *time.Time
	CreatedAt       time.Time
	DeliveredAt     *time.Time
	ConsumedByJobID string
	ConsumedAt      *time.Time
}

// Message kind 常量（与 design 一致）
const (
	KindUser    = "user"
	KindSignal  = "signal"
	KindTimer   = "timer"
	KindWebhook = "webhook"
	KindAgent   = "agent"
)

// SendOptions 发送选项
type SendOptions struct {
	Channel        string
	Kind           string
	CausationID    string // 可选：上游 message_id 或 job_id（design/plan.md Phase C）
	ScheduledAt    *time.Time
	ExpiresAt      *time.Time
	IdempotencyKey string
}
