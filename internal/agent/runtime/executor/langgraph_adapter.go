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

package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cloudwego/eino/compose"

	"rag-platform/internal/agent/planner"
	"rag-platform/internal/agent/runtime"
)

// LangGraphClient LangGraph 桥接接口，按最新调用形态暴露 invoke/stream/state。
type LangGraphClient interface {
	Invoke(ctx context.Context, input map[string]any) (map[string]any, error)
	Stream(ctx context.Context, input map[string]any, onChunk func(chunk map[string]any) error) error
	State(ctx context.Context, threadID string) (map[string]any, error)
}

// LangGraphErrorCode LangGraph error分类。
type LangGraphErrorCode string

const (
	LangGraphErrorRetryable LangGraphErrorCode = "retryable"
	LangGraphErrorPermanent LangGraphErrorCode = "permanent"
	LangGraphErrorWait      LangGraphErrorCode = "wait"
)

// LangGraphError LangGraph 适配层error；Runner/Adapter 可据此映射到 StepResultType 或等待 signal。
type LangGraphError struct {
	Code           LangGraphErrorCode
	Message        string
	CorrelationKey string
	Reason         string
	Err            error
}

func (e *LangGraphError) Error() string {
	if e == nil {
		return "langgraph error"
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "langgraph error"
}

func (e *LangGraphError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// LangGraphNodeAdapter 将 langgraph 型 TaskNode 转为 DAG 节点；支持 command 事件、effect 存储与error分类映射。
type LangGraphNodeAdapter struct {
	Client           LangGraphClient
	CommandEventSink CommandEventSink
	EffectStore      EffectStore
}

func (a *LangGraphNodeAdapter) runNode(ctx context.Context, taskID string, cfg map[string]any, p *AgentDAGPayload) (*AgentDAGPayload, error) {
	if a.Client == nil {
		return nil, fmt.Errorf("LangGraphNodeAdapter: Client not configured")
	}
	if p == nil {
		p = &AgentDAGPayload{}
	}
	if p.Results == nil {
		p.Results = make(map[string]any)
	}
	jobID := JobIDFromContext(ctx)

	if a.EffectStore != nil && jobID != "" {
		eff, err := a.EffectStore.GetEffectByJobAndCommandID(ctx, jobID, taskID)
		if err == nil && eff != nil && len(eff.Output) > 0 {
			var out map[string]any
			if json.Unmarshal(eff.Output, &out) == nil {
				p.Results[taskID] = out
				return p, nil
			}
		}
	}

	input := map[string]any{
		"goal":   p.Goal,
		"config": cfg,
	}
	if len(p.Results) > 0 {
		input["results"] = p.Results
	}
	if cfg != nil {
		if v, ok := cfg["input"].(map[string]any); ok {
			for k, val := range v {
				input[k] = val
			}
		}
	}
	if a.CommandEventSink != nil && jobID != "" {
		inputBytes, _ := json.Marshal(input)
		_ = a.CommandEventSink.AppendCommandEmitted(ctx, jobID, taskID, taskID, "langgraph", inputBytes)
	}

	out, err := a.Client.Invoke(ctx, input)
	if err != nil {
		if mapped := mapLangGraphError(taskID, err); mapped != nil {
			return nil, mapped
		}
		return nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	outputBytes, _ := json.Marshal(out)
	if a.EffectStore != nil && jobID != "" {
		inputBytes, _ := json.Marshal(input)
		_ = a.EffectStore.PutEffect(ctx, &EffectRecord{
			JobID:     jobID,
			CommandID: taskID,
			Kind:      EffectKindTool,
			Input:     inputBytes,
			Output:    outputBytes,
			Metadata:  map[string]any{"adapter": "langgraph"},
		})
	}
	if a.CommandEventSink != nil && jobID != "" {
		_ = a.CommandEventSink.AppendCommandCommitted(ctx, jobID, taskID, taskID, outputBytes, "")
	}
	p.Results[taskID] = out
	return p, nil
}

func mapLangGraphError(taskID string, err error) error {
	var lgErr *LangGraphError
	if !errors.As(err, &lgErr) || lgErr == nil {
		return nil
	}
	switch lgErr.Code {
	case LangGraphErrorRetryable:
		return &StepFailure{Type: StepResultRetryableFailure, Inner: err, NodeID: taskID}
	case LangGraphErrorPermanent:
		return &StepFailure{Type: StepResultPermanentFailure, Inner: err, NodeID: taskID}
	case LangGraphErrorWait:
		return &SignalWaitRequired{CorrelationKey: lgErr.CorrelationKey, Reason: lgErr.Reason}
	default:
		return nil
	}
}

// ToDAGNode 实现 NodeAdapter。
func (a *LangGraphNodeAdapter) ToDAGNode(task *planner.TaskNode, _ *runtime.Agent) (*compose.Lambda, error) {
	taskID := task.ID
	cfg := task.Config
	if cfg == nil {
		cfg = map[string]any{}
	}
	return compose.InvokableLambda[*AgentDAGPayload, *AgentDAGPayload](func(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error) {
		return a.runNode(ctx, taskID, cfg, p)
	}), nil
}

// ToNodeRunner 实现 NodeAdapter。
func (a *LangGraphNodeAdapter) ToNodeRunner(task *planner.TaskNode, _ *runtime.Agent) (NodeRunner, error) {
	taskID := task.ID
	cfg := task.Config
	if cfg == nil {
		cfg = map[string]any{}
	}
	return func(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error) {
		return a.runNode(ctx, taskID, cfg, p)
	}, nil
}
