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

package api

import (
	"context"

	einoembed "github.com/cloudwego/eino/components/embedding"
)

// EinoEmbedderAdapter 将 api.Embedder 适配为 eino/components/embedding.Embedder（EmbedStrings）
type EinoEmbedderAdapter struct {
	embedder Embedder
}

// NewEinoEmbedderAdapter 创建 Eino Embedder 适配器
func NewEinoEmbedderAdapter(embedder Embedder) *EinoEmbedderAdapter {
	return &EinoEmbedderAdapter{embedder: embedder}
}

// EmbedStrings 实现 eino/components/embedding.Embedder，内部调用 Embed，忽略 opts
func (a *EinoEmbedderAdapter) EmbedStrings(ctx context.Context, texts []string, _ ...einoembed.Option) ([][]float64, error) {
	if a.embedder == nil || len(texts) == 0 {
		return nil, nil
	}
	return a.embedder.Embed(ctx, texts)
}

// Ensure *EinoEmbedderAdapter 实现 einoembed.Embedder
var _ einoembed.Embedder = (*EinoEmbedderAdapter)(nil)
