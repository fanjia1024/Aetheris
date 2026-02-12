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

// Package sdk 提供 Step 编程模型与安全 API。参见 design/step-contract.md。
//
// Step Programming Model（强约束）
//
// 允许：
//   - 纯计算（无副作用）
//   - 调用 Tool（经 Runtime 执行并记录）
//   - 读 runtime state（通过 RuntimeContext：Now、UUID、HTTP、JobID、StepID）
//
// 禁止：
//   - goroutine、channel、time.Sleep
//   - 直接 time.Now()、uuid.New()、http.Get/Post 等未记录 IO
//   - 修改全局状态、裸外部 IO
//
// 违反契约会导致 replay 不确定、副作用重复执行，Runtime 保证失效。
package sdk

import (
	"context"
)

// StepFunc 单步执行函数签名；输入输出为 map，便于 DAG 传递。须遵守 Step Programming Model。
type StepFunc func(ctx context.Context, input map[string]any) (output map[string]any, err error)
