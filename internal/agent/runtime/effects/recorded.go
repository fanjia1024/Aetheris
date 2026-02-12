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

// Package effects 提供 Step 内可用的 Recorded Effects API：Now、UUID、HTTP 等必须经 Runtime 记录，
// Replay 时仅从事件注入，保证确定性。参见 design/effect-system.md、design/step-contract.md。
package effects

import (
	"context"
	"time"
)

// RecordedEffectRecorder 用于在非 Replay 路径下将时间/随机/UUID/HTTP 记录到事件流；
// 由 Runner 注入，实现方（如 node_sink）追加 timer_fired、uuid_recorded、random_recorded 等事件。
type RecordedEffectRecorder interface {
	RecordTime(ctx context.Context, jobID, effectID string, t time.Time) error
	RecordUUID(ctx context.Context, jobID, effectID, uuid string) error
	RecordRandom(ctx context.Context, jobID, effectID string, values []byte) error
}
