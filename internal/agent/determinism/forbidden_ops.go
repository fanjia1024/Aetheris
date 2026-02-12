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

// Package determinism 定义 Replay 时的确定性边界：禁止在 step 内使用的操作列表，
// 以及 Replay 模式下对非确定性行为的检测与阻断（replay_guard）。参见 design/replay-sandbox.md。
package determinism

// ForbiddenOp 表示在 step 内禁止直接使用的操作类型；Replay 时若检测到应 panic。
type ForbiddenOp string

const (
	// OpWallClock 读取系统时间（如 time.Now()）；应使用 runtime.Now(ctx)。
	OpWallClock ForbiddenOp = "wall_clock"
	// OpRandom 使用随机数（如 rand.Intn、uuid.New()）；应使用 runtime.UUID(ctx) 或 runtime.Random(ctx)。
	OpRandom ForbiddenOp = "random"
	// OpUnrecordedIO 未声明的外部 IO（如 http.Get、db.Query）；应通过 Tool 或 runtime.HTTP(ctx)。
	OpUnrecordedIO ForbiddenOp = "unrecorded_io"
	// OpGoroutine 在 step 内启动 goroutine；会破坏确定性调度。
	OpGoroutine ForbiddenOp = "goroutine"
	// OpChannel 在 step 内使用 channel 通信；应使用纯计算或 Tool。
	OpChannel ForbiddenOp = "channel"
	// OpSleep 在 step 内 time.Sleep；会引入非确定性延迟。
	OpSleep ForbiddenOp = "sleep"
)

// ForbiddenOps 返回 Replay 时禁止在 step 内使用的操作列表（用于文档与 replay_guard 检查）。
func ForbiddenOps() []ForbiddenOp {
	return []ForbiddenOp{
		OpWallClock,
		OpRandom,
		OpUnrecordedIO,
		OpGoroutine,
		OpChannel,
		OpSleep,
	}
}

// Description 返回禁止操作的说明，供 panic 消息与文档使用。
func (o ForbiddenOp) Description() string {
	switch o {
	case OpWallClock:
		return "读取系统时间（time.Now）在 Replay 时禁止；请使用 runtime.Now(ctx)"
	case OpRandom:
		return "使用随机数/uuid 在 Replay 时禁止；请使用 runtime.UUID(ctx) 或 runtime.Random(ctx)"
	case OpUnrecordedIO:
		return "未记录的外部 IO（http、db）在 Replay 时禁止；请通过 Tool 或 runtime.HTTP(ctx)"
	case OpGoroutine:
		return "在 step 内启动 goroutine 禁止；会破坏确定性"
	case OpChannel:
		return "在 step 内使用 channel 禁止；请使用纯计算或 Tool"
	case OpSleep:
		return "time.Sleep 在 step 内禁止；会引入非确定性"
	default:
		return "未记录的非确定性操作"
	}
}
