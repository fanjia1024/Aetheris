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

package sdk

import "time"

// Option 创建 Agent 时的可选配置
type Option func(*AgentConfig)

// AgentConfig Agent 可选配置
type AgentConfig struct {
	WaitTimeout time.Duration // 等待 Job 完成的最长时间，0 表示默认 5 分钟
}

// WithWaitTimeout 设置 Run 等待 Job 完成的超时
func WithWaitTimeout(d time.Duration) Option {
	return func(c *AgentConfig) {
		c.WaitTimeout = d
	}
}
