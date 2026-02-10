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

package replay

import (
	"context"

	"rag-platform/internal/runtime/jobstore"
)

// History 事件流迭代器：按序消费 Job 事件，供 Runner 以「事件驱动」方式决定下一步（plan 3.1 B）
type History interface {
	// Next 返回下一条事件并推进游标；无则返回 (nil, false)
	Next() (*jobstore.JobEvent, bool)
	// Peek 返回当前游标处事件但不推进；无则返回 (nil, false)
	Peek() (*jobstore.JobEvent, bool)
	// Index 当前游标位置（0-based）；无事件时为 0，消费完为 len(events)
	Index() int
	// Len 事件总数
	Len() int
}

// JobHistory 基于 JobStore 的 History 实现：从 ListEvents 加载后按序迭代
type JobHistory struct {
	events []jobstore.JobEvent
	idx    int
}

// NewJobHistory 从 store 加载 jobID 的事件流并返回可迭代的 History
func NewJobHistory(ctx context.Context, store jobstore.JobStore, jobID string) (History, error) {
	if store == nil || jobID == "" {
		return &JobHistory{events: nil, idx: 0}, nil
	}
	events, _, err := store.ListEvents(ctx, jobID)
	if err != nil {
		return nil, err
	}
	return &JobHistory{events: events, idx: 0}, nil
}

// Next 实现 History
func (h *JobHistory) Next() (*jobstore.JobEvent, bool) {
	if h.idx >= len(h.events) {
		return nil, false
	}
	e := &h.events[h.idx]
	h.idx++
	return e, true
}

// Peek 实现 History
func (h *JobHistory) Peek() (*jobstore.JobEvent, bool) {
	if h.idx >= len(h.events) {
		return nil, false
	}
	return &h.events[h.idx], true
}

// Index 实现 History
func (h *JobHistory) Index() int {
	return h.idx
}

// Len 实现 History
func (h *JobHistory) Len() int {
	return len(h.events)
}
