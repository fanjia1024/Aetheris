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
)

// StateChanged 单条外部资源变更，供审计、Trace UI 与 Confirmation Replay 校验
type StateChanged struct {
	ResourceType string `json:"resource_type"` // e.g. github_issue, file, release
	ResourceID   string `json:"resource_id"`   // e.g. "51", path, tag
	Operation    string `json:"operation"`     // e.g. created, updated, deleted
	StepID       string `json:"step_id,omitempty"`
	ToolName     string `json:"tool_name,omitempty"`
	Version      string `json:"version,omitempty"`      // 可选，资源版本
	Etag         string `json:"etag,omitempty"`         // 可选，校验用
	ExternalRef  string `json:"external_ref,omitempty"` // 可选，外部 URL 或 ID，供验证与审计
}

// StateCheckpointOpts 可选扩展字段，供 state_checkpointed 事件与 Trace UI「本步变更」展示
type StateCheckpointOpts struct {
	ChangedKeys     []string       // 本步变更的 memory key 列表
	ToolSideEffects []string       // 本步工具调用的副作用摘要（如 "created issue #123"）
	ResourceRefs    []string       // 本步涉及资源引用（如 issue ID、文件路径）
	StateChanges    []StateChanged // 结构化外部资源变更，供审计
}

// StateChangeSink 写入 state_changed 事件；Adapter 在 tool 提交副作用后可调用
type StateChangeSink interface {
	AppendStateChanged(ctx context.Context, jobID string, nodeID string, changes []StateChanged) error
}

// ResourceVerifier Confirmation Replay 时校验外部资源是否仍存在/一致；ok==false 或 err!=nil 表示不可信，不得注入结果
type ResourceVerifier interface {
	Verify(ctx context.Context, jobID, stepID, resourceType, resourceID, operation, externalRef string) (ok bool, err error)
}

// extractStateKeys 提取 state_before 和 state_after 的 keys，用于 Causal Chain Phase 1（design/execution-forensics.md § Causal Dependency）
// inputKeys: state_before 中有的 keys（本步读取）；outputKeys: state_after 中新增/变化的 keys（本步写入）
func extractStateKeys(before, after []byte) (inputKeys, outputKeys []string) {
	var beforeMap, afterMap map[string]interface{}
	_ = json.Unmarshal(before, &beforeMap)
	_ = json.Unmarshal(after, &afterMap)

	// inputKeys: before 中的 keys
	for k := range beforeMap {
		inputKeys = append(inputKeys, k)
	}

	// outputKeys: after 中新增或变化的 keys
	for k, vAfter := range afterMap {
		vBefore, existsBefore := beforeMap[k]
		if !existsBefore {
			outputKeys = append(outputKeys, k) // 新增
		} else {
			// 值变化：简单比较 JSON 序列化后的字符串
			bBefore, _ := json.Marshal(vBefore)
			bAfter, _ := json.Marshal(vAfter)
			if string(bBefore) != string(bAfter) {
				outputKeys = append(outputKeys, k)
			}
		}
	}
	return inputKeys, outputKeys
}

// ChangedKeysFromState 比较 state_before 与 state_after JSON，返回发生变化的 key 列表
func ChangedKeysFromState(stateBefore, stateAfter []byte) []string {
	var mBefore, mAfter map[string]interface{}
	_ = json.Unmarshal(stateBefore, &mBefore)
	_ = json.Unmarshal(stateAfter, &mAfter)
	if mAfter == nil {
		return nil
	}
	var changed []string
	for k := range mAfter {
		var vBefore, vAfter interface{}
		if mBefore != nil {
			vBefore = mBefore[k]
		}
		vAfter = mAfter[k]
		if !jsonEqual(vBefore, vAfter) {
			changed = append(changed, k)
		}
	}
	return changed
}

func jsonEqual(a, b interface{}) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}
