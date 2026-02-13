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
	"testing"
	"time"
)

// TestBuildDependencyGraph_Simple 测试简单依赖图（A → B → C）
func TestBuildDependencyGraph_Simple(t *testing.T) {
	events := []Event{
		{
			ID:        "1",
			JobID:     "job_1",
			Type:      "reasoning_snapshot_recorded",
			Payload:   []byte(`{"step_id":"step_a","node_id":"node_a","type":"plan","output_keys":["order_id"]}`),
			CreatedAt: time.Now(),
		},
		{
			ID:        "2",
			JobID:     "job_1",
			Type:      "reasoning_snapshot_recorded",
			Payload:   []byte(`{"step_id":"step_b","node_id":"node_b","type":"node","input_keys":["order_id"],"output_keys":["payment_result"]}`),
			CreatedAt: time.Now().Add(time.Second),
		},
		{
			ID:        "3",
			JobID:     "job_1",
			Type:      "reasoning_snapshot_recorded",
			Payload:   []byte(`{"step_id":"step_c","node_id":"node_c","type":"tool","input_keys":["payment_result"]}`),
			CreatedAt: time.Now().Add(2 * time.Second),
		},
	}

	builder := NewBuilder()
	graph, err := builder.BuildFromEvents(events)
	if err != nil {
		t.Fatalf("build graph failed: %v", err)
	}

	if len(graph.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(graph.Nodes))
	}

	if len(graph.Edges) != 2 {
		t.Errorf("expected 2 edges (A→B, B→C), got %d", len(graph.Edges))
	}

	// 验证边的方向
	edgeFound := false
	for _, edge := range graph.Edges {
		if edge.From == "step_a" && edge.To == "step_b" && edge.DataKey == "order_id" {
			edgeFound = true
		}
	}
	if !edgeFound {
		t.Error("expected edge step_a → step_b with data_key order_id")
	}
}

// TestBuildDependencyGraph_Complex 测试复杂依赖（A、B → C）
func TestBuildDependencyGraph_Complex(t *testing.T) {
	events := []Event{
		{
			ID:        "1",
			JobID:     "job_2",
			Type:      "reasoning_snapshot_recorded",
			Payload:   []byte(`{"step_id":"step_a","output_keys":["key_1"]}`),
			CreatedAt: time.Now(),
		},
		{
			ID:        "2",
			JobID:     "job_2",
			Type:      "reasoning_snapshot_recorded",
			Payload:   []byte(`{"step_id":"step_b","output_keys":["key_2"]}`),
			CreatedAt: time.Now().Add(time.Second),
		},
		{
			ID:        "3",
			JobID:     "job_2",
			Type:      "reasoning_snapshot_recorded",
			Payload:   []byte(`{"step_id":"step_c","input_keys":["key_1","key_2"]}`),
			CreatedAt: time.Now().Add(2 * time.Second),
		},
	}

	builder := NewBuilder()
	graph, err := builder.BuildFromEvents(events)
	if err != nil {
		t.Fatalf("build graph failed: %v", err)
	}

	if len(graph.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(graph.Nodes))
	}

	// 应该有 2 条边：A→C 和 B→C
	if len(graph.Edges) != 2 {
		t.Errorf("expected 2 edges (A→C, B→C), got %d", len(graph.Edges))
	}
}

// TestBuildDependencyGraph_WithEvidence 测试包含证据节点的图
func TestBuildDependencyGraph_WithEvidence(t *testing.T) {
	events := []Event{
		{
			ID:    "1",
			JobID: "job_3",
			Type:  "reasoning_snapshot_recorded",
			Payload: []byte(`{
				"step_id":"step_a",
				"evidence": {
					"rag_doc_ids": ["doc_123"],
					"tool_invocation_ids": ["inv_456"],
					"llm_decision": {
						"model": "gpt-4o",
						"provider": "openai",
						"temperature": 0.7,
						"token_count": 1234
					}
				}
			}`),
			CreatedAt: time.Now(),
		},
	}

	builder := NewBuilder()
	graph, err := builder.BuildFromEvents(events)
	if err != nil {
		t.Fatalf("build graph failed: %v", err)
	}

	if len(graph.Nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(graph.Nodes))
	}

	node := graph.Nodes[0]
	if len(node.Evidence.Nodes) == 0 {
		t.Error("expected evidence nodes")
	}

	// 验证证据节点类型
	hasRAGDoc := false
	hasToolInv := false
	hasLLM := false

	for _, evNode := range node.Evidence.Nodes {
		switch evNode.Type {
		case EvidenceTypeRAGDoc:
			hasRAGDoc = true
			if evNode.ID != "doc_123" {
				t.Errorf("expected rag doc id doc_123, got %s", evNode.ID)
			}
		case EvidenceTypeToolInvocation:
			hasToolInv = true
		case EvidenceTypeLLMDecision:
			hasLLM = true
		}
	}

	if !hasRAGDoc {
		t.Error("expected RAG doc evidence")
	}
	if !hasToolInv {
		t.Error("expected tool invocation evidence")
	}
	if !hasLLM {
		t.Error("expected LLM decision evidence")
	}
}
