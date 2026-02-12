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

package runtime

import "time"

// MemorySnapshot 恢复时绑定的记忆快照；design/durable-memory-layer.md
// 写入 job_waiting 的 resumption_context.memory_snapshot，恢复时 Apply 到 Session
type MemorySnapshot struct {
	WorkingMemory []byte   // AgentState JSON
	EpisodicTail  []byte   // 可选，最近 N 条 episodic 的 JSON 数组
	LongTermKeys  []string // 可选，本次引用的 long-term key
	SnapshotAt    time.Time
}
