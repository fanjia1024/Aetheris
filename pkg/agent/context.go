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

package agent

import "time"

// RunOptions 单次 Run 的可选参数（sessionID、超时、最大步数等）
type RunOptions struct {
	SessionID string
	Timeout   time.Duration
	MaxSteps  int
}

// RunOption 可选函数
type RunOption func(*RunOptions)

// WithSessionID 指定会话 ID，多次 Run 共用同一会话历史
func WithSessionID(sessionID string) RunOption {
	return func(o *RunOptions) {
		o.SessionID = sessionID
	}
}

// WithTimeout 设置单次 Run 超时（未实现时由 context 控制）
func WithTimeout(d time.Duration) RunOption {
	return func(o *RunOptions) {
		o.Timeout = d
	}
}

// WithRunMaxSteps 设置单次 Run 最大步数（覆盖 Agent 默认）
func WithRunMaxSteps(n int) RunOption {
	return func(o *RunOptions) {
		o.MaxSteps = n
	}
}

func applyRunOptions(opts []RunOption) *RunOptions {
	o := &RunOptions{MaxSteps: 20}
	for _, f := range opts {
		f(o)
	}
	return o
}
