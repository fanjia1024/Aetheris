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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// DeterministicStepID 根据 job、决策记录、图中序号、节点类型生成确定性步身份（design/step-identity.md）。
// 同一 job、同一 decision、同一 index、同一 nodeType 始终得到同一 ID，保证 Ledger 与 Replay 稳定。
func DeterministicStepID(jobID, decisionID string, index int, nodeType string) string {
	h := sha256.New()
	h.Write([]byte(jobID))
	h.Write([]byte("\x00"))
	h.Write([]byte(decisionID))
	h.Write([]byte("\x00"))
	h.Write([]byte(fmt.Sprint(index)))
	h.Write([]byte("\x00"))
	h.Write([]byte(nodeType))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// PlanDecisionID 从 PlanGenerated 的 task_graph 字节生成决策记录 ID（用于 DeterministicStepID）。
func PlanDecisionID(taskGraphState []byte) string {
	if len(taskGraphState) == 0 {
		return ""
	}
	h := sha256.Sum256(taskGraphState)
	return hex.EncodeToString(h[:])[:16]
}
