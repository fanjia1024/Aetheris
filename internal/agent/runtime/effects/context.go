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

package effects

import (
	"context"
	"sync"

	"rag-platform/internal/agent/replay"
)

type contextKey struct{}

type effectsCtx struct {
	JobID    string
	StepID   string
	Replay   *replay.ReplayContext
	Recorder RecordedEffectsRecorder
	timeIdx  int
	uuidIdx  int
	mu       sync.Mutex
}

// WithRecordedEffects 在调用 step 前注入；Replay 时 Replay 非空，Recorder 可为 nil；非 Replay 时 Recorder 非空。
func WithRecordedEffects(ctx context.Context, jobID, stepID string, replayCtx *replay.ReplayContext, rec RecordedEffectsRecorder) context.Context {
	return context.WithValue(ctx, contextKey{}, &effectsCtx{
		JobID: jobID, StepID: stepID, Replay: replayCtx, Recorder: rec,
	})
}

func getEffectsCtx(ctx context.Context) *effectsCtx {
	if ctx == nil {
		return nil
	}
	v := ctx.Value(contextKey{})
	if v == nil {
		return nil
	}
	ec, _ := v.(*effectsCtx)
	return ec
}
