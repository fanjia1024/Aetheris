package api

import (
	"context"
	"encoding/json"

	"rag-platform/internal/agent/runtime/executor"
	"rag-platform/internal/runtime/jobstore"
)

// PlanGeneratedSinkImpl 将 Plan 结果写入事件流，供 Trace/Replay 使用
type PlanGeneratedSinkImpl struct {
	store jobstore.JobStore
}

// NewPlanGeneratedSink 创建 PlanGenerated 事件写入器；store 为 nil 时不写入
func NewPlanGeneratedSink(store jobstore.JobStore) executor.PlanGeneratedSink {
	return &PlanGeneratedSinkImpl{store: store}
}

// AppendPlanGenerated 实现 executor.PlanGeneratedSink
func (s *PlanGeneratedSinkImpl) AppendPlanGenerated(ctx context.Context, jobID string, taskGraphJSON []byte, goal string) error {
	if s.store == nil {
		return nil
	}
	_, ver, err := s.store.ListEvents(ctx, jobID)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]interface{}{
		"task_graph": json.RawMessage(taskGraphJSON),
		"goal":       goal,
	})
	if err != nil {
		return err
	}
	_, err = s.store.Append(ctx, jobID, ver, jobstore.JobEvent{
		JobID: jobID, Type: jobstore.PlanGenerated, Payload: payload,
	})
	return err
}
