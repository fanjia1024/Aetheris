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
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Now 返回当前时间；Replay 时从事件流注入，否则经 Recorder 记录后返回。Step 内禁止直接使用 time.Now()。
func Now(ctx context.Context) time.Time {
	ec := getEffectsCtx(ctx)
	if ec == nil {
		return time.Now()
	}
	ec.mu.Lock()
	idx := ec.timeIdx
	ec.timeIdx++
	ec.mu.Unlock()
	effectID := fmt.Sprintf("%s:time:%d", ec.StepID, idx)
	if ec.Replay != nil && ec.Replay.RecordedTime != nil {
		if unixNano, ok := ec.Replay.RecordedTime[effectID]; ok {
			return time.Unix(0, unixNano)
		}
	}
	t := time.Now()
	if ec.Recorder != nil {
		_ = ec.Recorder.RecordTime(ctx, ec.JobID, effectID, t)
	}
	return t
}

// UUID 返回新的 UUID 字符串；Replay 时从事件流注入。Step 内禁止直接使用 uuid.New()。
func UUID(ctx context.Context) string {
	ec := getEffectsCtx(ctx)
	if ec == nil {
		return uuid.New().String()
	}
	ec.mu.Lock()
	idx := ec.uuidIdx
	ec.uuidIdx++
	ec.mu.Unlock()
	effectID := fmt.Sprintf("%s:uuid:%d", ec.StepID, idx)
	if ec.Replay != nil && ec.Replay.RecordedUUID != nil {
		if s, ok := ec.Replay.RecordedUUID[effectID]; ok {
			return s
		}
	}
	s := uuid.New().String()
	if ec.Recorder != nil {
		_ = ec.Recorder.RecordUUID(ctx, ec.JobID, effectID, s)
	}
	return s
}

// HTTP 执行 HTTP 请求并记录；Replay 时从事件流注入响应，不发起真实请求。
// req 与 resp 为 JSON 或任意序列化形式；调用方负责序列化/反序列化。
// 若 ctx 未注入 RecordedEffects，则返回错误（Step 内必须经 Runtime 记录）。
func HTTP(ctx context.Context, effectID string, doRequest func() (reqJSON, respJSON []byte, err error)) (reqJSON, respJSON []byte, err error) {
	ec := getEffectsCtx(ctx)
	if ec == nil {
		return nil, nil, fmt.Errorf("effects: HTTP 必须在 RecordedEffects context 下调用")
	}
	if effectID == "" {
		effectID = ec.StepID + ":http:0"
	}
	if ec.Replay != nil && ec.Replay.RecordedHTTP != nil {
		if resp, ok := ec.Replay.RecordedHTTP[effectID]; ok {
			return nil, resp, nil
		}
	}
	req, resp, err := doRequest()
	if err != nil {
		return req, nil, err
	}
	if ec.Recorder != nil {
		_ = ec.Recorder.RecordHTTP(ctx, ec.JobID, effectID, req, resp)
	}
	return req, resp, nil
}
