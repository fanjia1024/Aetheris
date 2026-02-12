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

// Package signal 提供持久化 signal 收件箱，保证 at-least-once 送达：先写 inbox 再 Append wait_completed。参见 design/runtime-contract.md Durable External Interaction Model。
package signal

import (
	"context"
	"time"
)

// SignalInbox 持久化 signal 收件箱；JobSignal API 先 Append 再写事件流，保证「人类点击一次 → 至少一次送达」
type SignalInbox interface {
	// Append 将一条 signal 持久化到 inbox，返回唯一 id；在 Append wait_completed 之前调用，避免 API 崩溃丢 signal
	Append(ctx context.Context, jobID, correlationKey string, payload []byte) (id string, err error)
	// MarkAcked 在 wait_completed 已成功写入且 Job 已置为 Pending 后标记该 signal 已送达，避免重复消费
	MarkAcked(ctx context.Context, jobID, id string) error
}

// SignalRecord 单条 signal 记录（可选查询/重试用）
type SignalRecord struct {
	ID             string
	JobID          string
	CorrelationKey string
	Payload        []byte
	CreatedAt      time.Time
	AckedAt        *time.Time
}
