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

package memory

import (
	"context"
	"sync"
	"time"
)

// EpisodicEntry 情景记忆单条；design/durable-memory-layer.md
type EpisodicEntry struct {
	ID        string
	AgentID   string
	SessionID string
	JobID     string
	Summary   string
	Payload   map[string]any
	CreatedAt time.Time
}

// EpisodicMemoryStore 情景记忆存储
type EpisodicMemoryStore interface {
	Append(ctx context.Context, entry *EpisodicEntry) error
	ListByAgent(ctx context.Context, agentID string, limit int) ([]*EpisodicEntry, error)
	ListBySession(ctx context.Context, agentID, sessionID string, limit int) ([]*EpisodicEntry, error)
}

// Episodic 实现 Memory 接口的简单情景记忆（内存、按条数限制）；供 CompositeMemory 与 Agent 使用
type Episodic struct {
	items []MemoryItem
	limit int
	mu    sync.RWMutex
}

// NewEpisodic 创建 Episodic Memory，limit 为最大条数（FIFO 淘汰）
func NewEpisodic(limit int) *Episodic {
	if limit <= 0 {
		limit = 1000
	}
	return &Episodic{items: nil, limit: limit}
}

// Recall 返回最近存储的条（与 query 无关，简单实现）
func (e *Episodic) Recall(ctx context.Context, query string) ([]MemoryItem, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]MemoryItem, 0, len(e.items))
	for _, it := range e.items {
		out = append(out, it)
	}
	return out, nil
}

// Store 追加一条，超过 limit 时丢弃最旧
func (e *Episodic) Store(ctx context.Context, item MemoryItem) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.items = append(e.items, item)
	for len(e.items) > e.limit {
		e.items = e.items[1:]
	}
	return nil
}
