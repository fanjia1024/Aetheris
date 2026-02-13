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
)

// Builder Evidence Graph 构建器（2.0-M3）
type Builder struct{}

// NewBuilder 创建 Evidence Graph 构建器
func NewBuilder() *Builder {
	return &Builder{}
}

// BuildFromEvents 从事件流构建 Evidence Graph
func (b *Builder) BuildFromEvents(events []Event) (*DependencyGraph, error) {
	graph := &DependencyGraph{
		Nodes: []GraphNode{},
		Edges: []GraphEdge{},
	}

	stepsByID := make(map[string]*GraphNode)
	keyToSteps := make(map[string][]string) // output_key -> step_ids that produce it

	// 解析 reasoning_snapshot 事件，构建节点
	for _, event := range events {
		if event.Type == "reasoning_snapshot_recorded" {
			node, err := b.parseReasoningSnapshot(event)
			if err != nil {
				continue
			}

			graph.Nodes = append(graph.Nodes, node)
			stepsByID[node.StepID] = &node

			// 记录该 step 产生的 output keys
			for _, outputKey := range node.OutputKeys {
				keyToSteps[outputKey] = append(keyToSteps[outputKey], node.StepID)
			}
		}
	}

	// 构建因果边
	for _, node := range graph.Nodes {
		for _, inputKey := range node.Evidence.InputKeys {
			// 找到产生该 key 的上游 steps
			if producers, ok := keyToSteps[inputKey]; ok {
				for _, producerID := range producers {
					// 避免自环
					if producerID != node.StepID {
						graph.Edges = append(graph.Edges, GraphEdge{
							From:     producerID,
							To:       node.StepID,
							Relation: "uses_output",
							DataKey:  inputKey,
						})
					}
				}
			}
		}

		// 添加 tool invocation 边
		for _, evidenceNode := range node.Evidence.Nodes {
			if evidenceNode.Type == EvidenceTypeToolInvocation {
				// Tool invocation 是一种特殊的依赖关系
				// 这里可以扩展为指向 tool 调用的边
			}
		}
	}

	return graph, nil
}

// parseReasoningSnapshot 解析 reasoning_snapshot 事件
func (b *Builder) parseReasoningSnapshot(event Event) (GraphNode, error) {
	var payload map[string]interface{}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return GraphNode{}, err
	}

	node := GraphNode{
		StepID:    getStringFromMap(payload, "step_id"),
		NodeID:    getStringFromMap(payload, "node_id"),
		Type:      getStringFromMap(payload, "type"),
		Label:     getStringFromMap(payload, "label"),
		Timestamp: event.CreatedAt,
		Evidence:  Evidence{},
	}

	// 提取 evidence
	if evidenceMap, ok := payload["evidence"].(map[string]interface{}); ok {
		node.Evidence = b.parseEvidence(evidenceMap)
	}

	// 提取 input_keys 和 output_keys（用于因果依赖）
	if inputKeys, ok := payload["input_keys"].([]interface{}); ok {
		for _, key := range inputKeys {
			if keyStr, ok := key.(string); ok {
				node.Evidence.InputKeys = append(node.Evidence.InputKeys, keyStr)
			}
		}
	}

	if outputKeys, ok := payload["output_keys"].([]interface{}); ok {
		for _, key := range outputKeys {
			if keyStr, ok := key.(string); ok {
				node.OutputKeys = append(node.OutputKeys, keyStr)
				node.Evidence.OutputKeys = append(node.Evidence.OutputKeys, keyStr)
			}
		}
	}

	return node, nil
}

// parseEvidence 解析证据字段
func (b *Builder) parseEvidence(evidenceMap map[string]interface{}) Evidence {
	evidence := Evidence{
		Nodes: []EvidenceNode{},
	}

	// 解析 rag_doc_ids
	if ragDocs, ok := evidenceMap["rag_doc_ids"].([]interface{}); ok {
		for _, docID := range ragDocs {
			if docStr, ok := docID.(string); ok {
				evidence.Nodes = append(evidence.Nodes, EvidenceNode{
					Type: EvidenceTypeRAGDoc,
					ID:   docStr,
				})
			}
		}
	}

	// 解析 tool_invocation_ids
	if toolInvs, ok := evidenceMap["tool_invocation_ids"].([]interface{}); ok {
		for _, invID := range toolInvs {
			if invStr, ok := invID.(string); ok {
				evidence.Nodes = append(evidence.Nodes, EvidenceNode{
					Type: EvidenceTypeToolInvocation,
					ID:   invStr,
				})
			}
		}
	}

	// 解析 llm_decision
	if llmDec, ok := evidenceMap["llm_decision"].(map[string]interface{}); ok {
		evidence.LLMDecision = &LLMDecisionEvidence{
			Model:       getStringFromMap(llmDec, "model"),
			Provider:    getStringFromMap(llmDec, "provider"),
			Temperature: getFloatFromMap(llmDec, "temperature"),
			PromptHash:  getStringFromMap(llmDec, "prompt_hash"),
			TokenCount:  getIntFromMap(llmDec, "token_count"),
		}

		// 添加为证据节点
		evidence.Nodes = append(evidence.Nodes, EvidenceNode{
			Type:    EvidenceTypeLLMDecision,
			ID:      evidence.LLMDecision.Model,
			Summary: evidence.LLMDecision.Provider,
		})
	}

	return evidence
}

// Helper functions
func getStringFromMap(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getFloatFromMap(m map[string]interface{}, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0
}

func getIntFromMap(m map[string]interface{}, key string) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	if v, ok := m[key].(int); ok {
		return v
	}
	return 0
}
