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

import "encoding/json"

// StateCheckpointOpts 可选扩展字段，供 state_checkpointed 事件与 Trace UI「本步变更」展示
type StateCheckpointOpts struct {
	ChangedKeys     []string // 本步变更的 memory key 列表
	ToolSideEffects []string // 本步工具调用的副作用摘要（如 "created issue #123"）
	ResourceRefs    []string // 本步涉及资源引用（如 issue ID、文件路径）
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
