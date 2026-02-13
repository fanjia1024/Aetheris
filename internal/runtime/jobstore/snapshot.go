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

package jobstore

import (
	"encoding/json"
	"time"
)

// JobSnapshot 事件流快照，用于优化长跑 job 的 replay 性能
// 快照内容为 ReplayContext 的完整状态，包含所有已完成节点、工具调用、状态变更等
type JobSnapshot struct {
	JobID     string    `json:"job_id"`
	Version   int       `json:"version"`  // 快照覆盖的最后事件版本号
	Snapshot  []byte    `json:"snapshot"` // ReplayContext 的 JSON 序列化
	CreatedAt time.Time `json:"created_at"`
}

// SnapshotPayload 快照的完整内容（对应 ReplayContext 的可序列化形式）
type SnapshotPayload struct {
	TaskGraphState           json.RawMessage            `json:"task_graph_state,omitempty"`
	CursorNode               string                     `json:"cursor_node,omitempty"`
	PayloadResults           json.RawMessage            `json:"payload_results,omitempty"`
	CompletedNodeIDs         []string                   `json:"completed_node_ids,omitempty"`
	PayloadResultsByNode     map[string]json.RawMessage `json:"payload_results_by_node,omitempty"`
	CompletedCommandIDs      []string                   `json:"completed_command_ids,omitempty"`
	CommandResults           map[string]json.RawMessage `json:"command_results,omitempty"`
	CompletedToolInvocations map[string]json.RawMessage `json:"completed_tool_invocations,omitempty"`
	PendingToolInvocations   []string                   `json:"pending_tool_invocations,omitempty"`
	StateChangesByStep       map[string]json.RawMessage `json:"state_changes_by_step,omitempty"`
	ApprovedCorrelationKeys  []string                   `json:"approved_correlation_keys,omitempty"`
	WorkingMemorySnapshot    json.RawMessage            `json:"working_memory_snapshot,omitempty"`
	Phase                    int                        `json:"phase,omitempty"`
	RecordedTime             map[string]int64           `json:"recorded_time,omitempty"`
	RecordedRandom           map[string]json.RawMessage `json:"recorded_random,omitempty"`
	RecordedUUID             map[string]string          `json:"recorded_uuid,omitempty"`
	RecordedHTTP             map[string]json.RawMessage `json:"recorded_http,omitempty"`
}

// SnapshotStore 快照存储接口，扩展 JobStore
type SnapshotStore interface {
	// CreateSnapshot 创建快照，覆盖 job 从版本 0 到 upToVersion 的所有状态
	// snapshot 参数为 SnapshotPayload 的 JSON 序列化
	CreateSnapshot(jobID string, upToVersion int, snapshot []byte) error

	// GetLatestSnapshot 获取最新的快照；若无快照返回 nil
	GetLatestSnapshot(jobID string) (*JobSnapshot, error)

	// DeleteSnapshotsBefore 删除指定版本之前的所有快照（用于 compaction）
	DeleteSnapshotsBefore(jobID string, beforeVersion int) error
}

// CompactionConfig 快照压缩配置
type CompactionConfig struct {
	// EnableAutoCompaction 是否启用自动压缩
	EnableAutoCompaction bool `yaml:"enable_auto_compaction"`

	// EventCountThreshold 事件数超过此阈值时触发快照
	EventCountThreshold int `yaml:"event_count_threshold"`

	// TimeIntervalHours 每隔多少小时创建快照（0 表示禁用时间触发）
	TimeIntervalHours int `yaml:"time_interval_hours"`

	// KeepSnapshotCount 保留最近几个快照（旧快照会被删除）
	KeepSnapshotCount int `yaml:"keep_snapshot_count"`
}

// DefaultCompactionConfig 默认压缩配置
func DefaultCompactionConfig() CompactionConfig {
	return CompactionConfig{
		EnableAutoCompaction: false, // 默认不启用，需显式配置
		EventCountThreshold:  1000,  // 超过 1000 个事件触发
		TimeIntervalHours:    24,    // 每 24 小时触发
		KeepSnapshotCount:    3,     // 保留最近 3 个快照
	}
}
