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

// Package effects 提供 Step 内可用的 Recorded Effects API：Now、UUID、HTTP。
// 所有对外非确定性操作必须经此包记录，Replay 时仅从事件注入。参见 design/effect-system.md。
package effects

import (
	"context"
	"time"
)

// RecordedEffectsRecorder 将时间/UUID/HTTP 等效应追加到事件流；Runner 在非 Replay 路径注入实现（如 NodeSink）。
type RecordedEffectsRecorder interface {
	RecordTime(ctx context.Context, jobID, effectID string, t time.Time) error
	RecordUUID(ctx context.Context, jobID, effectID, uuid string) error
	RecordHTTP(ctx context.Context, jobID, effectID string, req, resp []byte) error
}
