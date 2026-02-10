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

package job

// QueueClass 队列类型：Scheduler 按队列拉取任务，支持优先级与隔离
const (
	QueueRealtime   = "realtime"   // 实时对话等低延迟
	QueueDefault    = "default"    // 默认
	QueueBackground = "background" // 知识构建等后台
	QueueHeavy      = "heavy"     // 批处理等重任务
)

// DefaultPriority 默认优先级（QueueDefault 使用）
const DefaultPriority = 0

// Priority 数值越大越优先（realtime 高，heavy 低）
const (
	PriorityRealtime   = 10
	PriorityDefault   = 0
	PriorityBackground = -5
	PriorityHeavy     = -10
)

// PriorityForQueue 返回队列类型对应的默认优先级
func PriorityForQueue(queueClass string) int {
	switch queueClass {
	case QueueRealtime:
		return PriorityRealtime
	case QueueBackground:
		return PriorityBackground
	case QueueHeavy:
		return PriorityHeavy
	default:
		return PriorityDefault
	}
}
