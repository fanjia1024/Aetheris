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
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"rag-platform/internal/agent/planner"
	"rag-platform/internal/agent/replay"
	"rag-platform/internal/agent/runtime"
	"rag-platform/internal/runtime/jobstore"
)

// mockLLMWithCallCount LLM mock，记录 Generate 调用次数（用于断言 Replay 不调用 LLM）
type mockLLMWithCallCount struct {
	callCount *int32
	response  string
}

func (m *mockLLMWithCallCount) Generate(ctx context.Context, prompt string) (string, error) {
	atomic.AddInt32(m.callCount, 1)
	return m.response, nil
}

// TestReplay_NeverCallsLLM 契约：配置 Effect Store 时，Replay **绝不**调用 LLM（design/execution-guarantees.md § Replay 绝不调用 LLM）
func TestReplay_NeverCallsLLM(t *testing.T) {
	ctx := context.Background()
	jobID := "job-llm-replay"

	var callCount int32
	mockLLM := &mockLLMWithCallCount{
		callCount: &callCount,
		response:  "LLM response for test",
	}

	// Setup: Effect Store + Event Store
	effectStore := NewEffectStoreMem()
	eventStore := jobstore.NewMemoryStore()

	// 构造 TaskGraph：单个 LLM 节点
	taskGraph := &planner.TaskGraph{
		Nodes: []planner.TaskNode{
			{ID: "llm1", Type: planner.NodeLLM, Config: map[string]any{"goal": "Summarize"}},
		},
		Edges: []planner.TaskEdge{},
	}

	// 使用 buildReplayableEventStream 构造完整事件流（含 PlanGenerated + command_committed + NodeFinished）
	nodeResults := map[string][]byte{
		"llm1": []byte(`{"output":"LLM response for test"}`),
	}
	buildReplayableEventStream(t, eventStore, jobID, taskGraph, nodeResults)

	// 同时写入 Effect Store（模拟第一次执行后的状态）
	respBytes, _ := json.Marshal("LLM response for test")
	_ = effectStore.PutEffect(ctx, &EffectRecord{
		JobID:     jobID,
		CommandID: "llm1",
		Kind:      EffectKindLLM,
		Input:     []byte(`{"prompt":"Summarize"}`),
		Output:    respBytes,
	})

	// Compiler + Runner with Effect Store
	llmAdapter := &LLMNodeAdapter{LLM: mockLLM, EffectStore: effectStore}
	compiler := NewCompiler(map[string]NodeAdapter{planner.NodeLLM: llmAdapter})
	runner := NewRunner(compiler)
	cpStore := runtime.NewCheckpointStoreMem()
	fakeJobStore := &fakeJobStoreForRunner{}
	runner.SetCheckpointStores(cpStore, fakeJobStore)
	runner.SetReplayContextBuilder(replay.NewReplayContextBuilder(eventStore))

	// Replay 执行：事件流已有完整记录，callCount 应保持 0
	agent := &runtime.Agent{ID: "agent-1"}
	job := &JobForRunner{ID: jobID, Goal: "test goal"}

	err := runner.RunForJob(ctx, agent, job)
	require.NoError(t, err, "Replay should succeed")
	require.Equal(t, int32(0), atomic.LoadInt32(&callCount), "Replay must NOT call LLM")
}

// TestReplay_LLMWithEffectStore 验证配置 Effect Store 时 Replay 从 Effect Store 注入（adapter 层防御）
func TestReplay_LLMWithEffectStore(t *testing.T) {
	ctx := context.Background()
	jobID := "job-llm-effect"

	var callCount int32
	mockLLM := &mockLLMWithCallCount{
		callCount: &callCount,
		response:  "Should not be called",
	}

	effectStore := NewEffectStoreMem()
	eventStore := jobstore.NewMemoryStore()

	taskGraph := &planner.TaskGraph{
		Nodes: []planner.TaskNode{
			{ID: "llm1", Type: planner.NodeLLM},
		},
		Edges: []planner.TaskEdge{},
	}

	// 构造已有记录的事件流（模拟第一次执行后状态）
	buildReplayableEventStream(t, eventStore, jobID, taskGraph, map[string][]byte{
		"llm1": []byte(`{"output":"Stored LLM response"}`),
	})

	// Effect Store 中已有记录（模拟第一次执行写入）
	respBytes, _ := json.Marshal("Stored LLM response")
	_ = effectStore.PutEffect(ctx, &EffectRecord{
		JobID:     jobID,
		CommandID: "llm1",
		Kind:      EffectKindLLM,
		Output:    respBytes,
	})

	// LLMNodeAdapter 配置 Effect Store
	llmAdapter := &LLMNodeAdapter{LLM: mockLLM, EffectStore: effectStore}
	compiler := NewCompiler(map[string]NodeAdapter{planner.NodeLLM: llmAdapter})
	runner := NewRunner(compiler)
	cpStore := runtime.NewCheckpointStoreMem()
	fakeJobStore := &fakeJobStoreForRunner{}
	runner.SetCheckpointStores(cpStore, fakeJobStore)
	runner.SetReplayContextBuilder(replay.NewReplayContextBuilder(eventStore))

	agent := &runtime.Agent{ID: "agent-1"}
	job := &JobForRunner{ID: jobID, Goal: "test"}

	// Replay：应从 Effect Store 注入，不调用 LLM
	err := runner.RunForJob(ctx, agent, job)
	require.NoError(t, err)
	require.Equal(t, int32(0), atomic.LoadInt32(&callCount), "Replay with EffectStore must not call LLM")
}

// TestLLMAdapter_RequiresEffectStoreInProduction 验证生产模式下 LLM 必须配置 Effect Store（可选特性，未来）
func TestLLMAdapter_RequiresEffectStoreInProduction(t *testing.T) {
	t.Skip("Feature not implemented yet: requireEffectStore flag")

	// 未来实现：LLMNodeAdapter 增加 requireEffectStore 标志
	// 生产模式下若 EffectStore == nil，应返回错误而非执行
}
