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

	TaskGraphState []byte // 序列化后的 TaskGraph（如 planner.TaskGraph.Marshal）
	MemoryState    []byte // 可选：记忆快照

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
			list = append(list, &cpCopy)
		}
	}
	return list, nil
}
