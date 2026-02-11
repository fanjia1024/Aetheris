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
	"time"
)

// WakeupQueue 唤醒队列：当 Job 因 signal/message 变为 Pending 时 API 调用 NotifyReady；Scheduler/Worker 在等待时优先从 Receive 唤醒，从而立即 Claim 而非仅靠轮询（design/wakeup-index、agent-process-model）
type WakeupQueue interface {
	// NotifyReady 通知指定 job 已变为可运行（如 wait_completed 后 UpdateStatus(Pending)）；API 在 JobSignal/JobMessage 中调用
	NotifyReady(ctx context.Context, jobID string) error
	// Receive 阻塞最多 timeout，若有 NotifyReady 则返回 (jobID, true)，否则超时返回 ("", false)；Worker 在无 job 时调用以替代固定 sleep，实现事件驱动唤醒
	Receive(ctx context.Context, timeout time.Duration) (jobID string, ok bool)
}

// WakeupQueueMem 内存实现：带缓冲 channel；单进程内 API 与 Worker 共享同一实例时有效，多进程需 Redis/PG 等实现
type WakeupQueueMem struct {
	ch chan string
}

// NewWakeupQueueMem 创建内存唤醒队列；bufSize 建议 256 以上，避免 API 写阻塞
func NewWakeupQueueMem(bufSize int) *WakeupQueueMem {
	if bufSize <= 0 {
		bufSize = 256
	}
	return &WakeupQueueMem{ch: make(chan string, bufSize)}
}

// NotifyReady 实现 WakeupQueue；非阻塞发送，channel 满时丢弃（避免 API 阻塞）
func (q *WakeupQueueMem) NotifyReady(ctx context.Context, jobID string) error {
	if jobID == "" {
		return nil
	}
	select {
	case q.ch <- jobID:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		// 队列满，不阻塞 API
		return nil
	}
}

// Receive 实现 WakeupQueue
func (q *WakeupQueueMem) Receive(ctx context.Context, timeout time.Duration) (string, bool) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case id := <-q.ch:
		return id, true
	case <-timer.C:
		return "", false
	case <-ctx.Done():
		return "", false
	}
}
