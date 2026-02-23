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

package evidence

import (
	"encoding/json"
	"testing"
	"time"
)

// TestEvidenceExport_FullWorkflow 端到端验证证据包导出：
// 模拟一个完整 Job 的 reasoning_snapshot 事件序列，构建依赖图并验证导出格式。
func TestEvidenceExport_FullWorkflow(t *testing.T) {
	now := time.Now()
	events := []Event{
		{
			ID:    "e1",
			JobID: "job-export-001",
			Type:  "reasoning_snapshot",
			Payload: []byte(`{
				"step_id":"step-plan",
				"node_id":"plan-node",
				"type":"plan",
				"label":"规划阶段",
				"output_keys":["task_list"],
				"evidence": {}
			}`),
			CreatedAt: now,
		},
		{
			ID:    "e2",
			JobID: "job-export-001",
			Type:  "reasoning_snapshot",
			Payload: []byte(`{
				"step_id":"step-rag",
				"node_id":"rag-node",
				"type":"node",
				"label":"检索文档",
				"input_keys":["task_list"],
				"output_keys":["retrieved_docs"],
				"evidence": {
					"rag_doc_ids": ["doc-001","doc-002"]
				}
			}`),
			CreatedAt: now.Add(time.Second),
		},
		{
			ID:    "e3",
			JobID: "job-export-001",
			Type:  "reasoning_snapshot",
			Payload: []byte(`{
				"step_id":"step-llm",
				"node_id":"llm-node",
				"type":"node",
				"label":"LLM 推理",
				"input_keys":["task_list","retrieved_docs"],
				"output_keys":["answer"],
				"evidence": {
					"llm_decision": {
						"model":"gpt-4o",
						"provider":"openai",
						"temperature":0.7,
						"token_count":512
					}
				}
			}`),
			CreatedAt: now.Add(2 * time.Second),
		},
		{
			ID:    "e4",
			JobID: "job-export-001",
			Type:  "reasoning_snapshot",
			Payload: []byte(`{
				"step_id":"step-tool",
				"node_id":"tool-node",
				"type":"tool",
				"label":"调用外部 API",
				"input_keys":["answer"],
				"output_keys":["api_result"],
				"evidence": {
					"tool_invocation_ids":["inv-abc"]
				}
			}`),
			CreatedAt: now.Add(3 * time.Second),
		},
	}

	builder := NewBuilder()
	graph, err := builder.BuildFromEvents(events)
	if err != nil {
		t.Fatalf("BuildFromEvents failed: %v", err)
	}

	// 应有 4 个节点
	if len(graph.Nodes) != 4 {
		t.Errorf("nodes: got %d want 4", len(graph.Nodes))
	}

	// 验证因果边：plan→rag, plan→llm, rag→llm, llm→tool
	expectedEdges := map[string]string{
		"step-plan:step-rag": "task_list",
		"step-plan:step-llm": "task_list",
		"step-rag:step-llm":  "retrieved_docs",
		"step-llm:step-tool": "answer",
	}

	edgeFound := make(map[string]bool)
	for _, edge := range graph.Edges {
		key := edge.From + ":" + edge.To
		edgeFound[key] = true
		if wantKey, ok := expectedEdges[key]; ok {
			if edge.DataKey != wantKey {
				t.Errorf("edge %s: data_key got %q want %q", key, edge.DataKey, wantKey)
			}
		}
	}
	for key := range expectedEdges {
		if !edgeFound[key] {
			t.Errorf("missing expected edge: %s", key)
		}
	}

	// 验证 RAG 文档证据
	var ragNode *GraphNode
	for i := range graph.Nodes {
		if graph.Nodes[i].StepID == "step-rag" {
			ragNode = &graph.Nodes[i]
			break
		}
	}
	if ragNode == nil {
		t.Fatal("step-rag node not found")
	}
	ragDocCount := 0
	for _, en := range ragNode.Evidence.Nodes {
		if en.Type == EvidenceTypeRAGDoc {
			ragDocCount++
		}
	}
	if ragDocCount != 2 {
		t.Errorf("rag doc count: got %d want 2", ragDocCount)
	}

	// 验证图可以 JSON 序列化（模拟导出）
	exportData, err := json.Marshal(graph)
	if err != nil {
		t.Fatalf("marshal evidence graph: %v", err)
	}
	if len(exportData) == 0 {
		t.Error("export data should not be empty")
	}

	// 验证反序列化后内容一致
	var imported DependencyGraph
	if err := json.Unmarshal(exportData, &imported); err != nil {
		t.Fatalf("unmarshal evidence graph: %v", err)
	}
	if len(imported.Nodes) != len(graph.Nodes) {
		t.Errorf("imported nodes: got %d want %d", len(imported.Nodes), len(graph.Nodes))
	}
	if len(imported.Edges) != len(graph.Edges) {
		t.Errorf("imported edges: got %d want %d", len(imported.Edges), len(graph.Edges))
	}

	t.Logf("evidence export: %d nodes, %d edges, %d bytes", len(graph.Nodes), len(graph.Edges), len(exportData))
}

// TestEvidenceExport_EmptyEvents 验证空事件列表返回空图（不 panic）。
func TestEvidenceExport_EmptyEvents(t *testing.T) {
	builder := NewBuilder()
	graph, err := builder.BuildFromEvents(nil)
	if err != nil {
		t.Fatalf("BuildFromEvents with nil events: %v", err)
	}
	if graph == nil {
		t.Fatal("graph should not be nil")
	}
	if len(graph.Nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(graph.Nodes))
	}
	if len(graph.Edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(graph.Edges))
	}
}

// TestEvidenceExport_UnknownEventTypes 验证非 reasoning_snapshot 类型的事件被跳过。
func TestEvidenceExport_UnknownEventTypes(t *testing.T) {
	events := []Event{
		{ID: "1", JobID: "job-1", Type: "plan_generated", Payload: []byte(`{}`), CreatedAt: time.Now()},
		{ID: "2", JobID: "job-1", Type: "node_finished", Payload: []byte(`{}`), CreatedAt: time.Now()},
		{ID: "3", JobID: "job-1", Type: "reasoning_snapshot", Payload: []byte(`{"step_id":"s1","output_keys":["k"]}`), CreatedAt: time.Now()},
	}

	builder := NewBuilder()
	graph, err := builder.BuildFromEvents(events)
	if err != nil {
		t.Fatalf("BuildFromEvents: %v", err)
	}
	if len(graph.Nodes) != 1 {
		t.Errorf("expected 1 node (only from reasoning_snapshot), got %d", len(graph.Nodes))
	}
}
