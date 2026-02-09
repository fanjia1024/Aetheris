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

package runtime

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Checkpoint 用于恢复的检查点：Agent/Session 与任务图状态
type Checkpoint struct {
	ID string

	AgentID   string
	SessionID string
	JobID     string // 可选，关联 Job 便于恢复

	TaskGraphState []byte // 序列化后的 TaskGraph（如 planner.TaskGraph.Marshal）
	MemoryState    []byte // 可选：记忆快照
	// CursorNode 最后执行完成的节点 ID；恢复时从下一节点继续
	CursorNode string
	// PayloadResults 当前 payload.Results 的 JSON；恢复时还原
	PayloadResults []byte

	CreatedAt time.Time
}

// CheckpointStore 检查点存储接口
type CheckpointStore interface {
	Save(ctx context.Context, cp *Checkpoint) (id string, err error)
	Load(ctx context.Context, id string) (*Checkpoint, error)
	ListByAgent(ctx context.Context, agentID string) ([]*Checkpoint, error)
}

// NewCheckpoint 创建检查点（ID 可在 Save 时生成）
func NewCheckpoint(agentID, sessionID string, taskGraphState, memoryState []byte) *Checkpoint {
	return &Checkpoint{
		AgentID:        agentID,
		SessionID:      sessionID,
		TaskGraphState: taskGraphState,
		MemoryState:    memoryState,
		CreatedAt:      time.Now(),
	}
}

// NewNodeCheckpoint 创建节点级检查点（恢复时从 CursorNode 下一节点继续）
func NewNodeCheckpoint(agentID, sessionID, jobID, cursorNode string, taskGraphState, payloadResults, memoryState []byte) *Checkpoint {
	return &Checkpoint{
		AgentID:        agentID,
		SessionID:      sessionID,
		JobID:          jobID,
		CursorNode:     cursorNode,
		TaskGraphState: taskGraphState,
		PayloadResults: payloadResults,
		MemoryState:    memoryState,
		CreatedAt:      time.Now(),
	}
}

// checkpointStoreMem 内存实现的 CheckpointStore
type checkpointStoreMem struct {
	mu   sync.RWMutex
	byID map[string]*Checkpoint
}

// NewCheckpointStoreMem 创建内存版 CheckpointStore
func NewCheckpointStoreMem() CheckpointStore {
	return &checkpointStoreMem{byID: make(map[string]*Checkpoint)}
}

func (s *checkpointStoreMem) Save(ctx context.Context, cp *Checkpoint) (string, error) {
	id := cp.ID
	if id == "" {
		id = "cp-" + uuid.New().String()
		cp.ID = id
	}
	cpCopy := *cp
	if len(cp.TaskGraphState) > 0 {
		cpCopy.TaskGraphState = make([]byte, len(cp.TaskGraphState))
		copy(cpCopy.TaskGraphState, cp.TaskGraphState)
	}
	if len(cp.MemoryState) > 0 {
		cpCopy.MemoryState = make([]byte, len(cp.MemoryState))
		copy(cpCopy.MemoryState, cp.MemoryState)
	}
	if len(cp.PayloadResults) > 0 {
		cpCopy.PayloadResults = make([]byte, len(cp.PayloadResults))
		copy(cpCopy.PayloadResults, cp.PayloadResults)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[id] = &cpCopy
	return id, nil
}

func (s *checkpointStoreMem) Load(ctx context.Context, id string) (*Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp, ok := s.byID[id]
	if !ok {
		return nil, nil
	}
	// 返回副本，避免外部修改
	out := *cp
	if len(cp.TaskGraphState) > 0 {
		out.TaskGraphState = make([]byte, len(cp.TaskGraphState))
		copy(out.TaskGraphState, cp.TaskGraphState)
	}
	if len(cp.MemoryState) > 0 {
		out.MemoryState = make([]byte, len(cp.MemoryState))
		copy(out.MemoryState, cp.MemoryState)
	}
	if len(cp.PayloadResults) > 0 {
		out.PayloadResults = make([]byte, len(cp.PayloadResults))
		copy(out.PayloadResults, cp.PayloadResults)
	}
	return &out, nil
}

func (s *checkpointStoreMem) ListByAgent(ctx context.Context, agentID string) ([]*Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var list []*Checkpoint
	for _, cp := range s.byID {
		if cp.AgentID == agentID {
			cpCopy := *cp
			if len(cp.TaskGraphState) > 0 {
				cpCopy.TaskGraphState = make([]byte, len(cp.TaskGraphState))
				copy(cpCopy.TaskGraphState, cp.TaskGraphState)
			}
			if len(cp.MemoryState) > 0 {
				cpCopy.MemoryState = make([]byte, len(cp.MemoryState))
				copy(cpCopy.MemoryState, cp.MemoryState)
			}
			if len(cp.PayloadResults) > 0 {
				cpCopy.PayloadResults = make([]byte, len(cp.PayloadResults))
				copy(cpCopy.PayloadResults, cp.PayloadResults)
			}
			list = append(list, &cpCopy)
		}
	}
	return list, nil
}
