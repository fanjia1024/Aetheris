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

package eino

import (
	"context"
)

// WorkflowExecutor 工作流执行器：按名称执行工作流，参数与结果为通用 map，便于 API 与 eino 解耦。
type WorkflowExecutor interface {
	Execute(ctx context.Context, params map[string]interface{}) (interface{}, error)
}
