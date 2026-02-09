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

package tools

import (
	"context"
	"encoding/json"

	"rag-platform/internal/runtime/session"
	"rag-platform/internal/tool"
)

// Wrap 将无 Session 的 tool.Tool 包装为 Session 感知的 tools.Tool（Execute 时忽略 session）
func Wrap(t tool.Tool) Tool {
	return &wrappedTool{t: t}
}

type wrappedTool struct {
	t tool.Tool
}

func (w *wrappedTool) Name() string        { return w.t.Name() }
func (w *wrappedTool) Description() string { return w.t.Description() }

func (w *wrappedTool) Schema() map[string]any {
	s := w.t.Schema()
	b, _ := json.Marshal(s)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

func (w *wrappedTool) Execute(ctx context.Context, _ *session.Session, input map[string]any, state interface{}) (any, error) {
	res, err := w.t.Execute(ctx, input)
	if err != nil {
		return nil, err
	}
	return res, nil
}
