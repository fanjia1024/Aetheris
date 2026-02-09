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

package embedding

import (
	"context"
)

// OpenAIAdapter OpenAI Embedding 适配器（占位：可后续对接真实 API）
type OpenAIAdapter struct {
	Embedder
	apiKey string
	model  string
}

// NewOpenAIAdapter 创建 OpenAI Embedding 适配器
func NewOpenAIAdapter(apiKey, model string, dimension int) *OpenAIAdapter {
	if model == "" {
		model = "text-embedding-3-small"
	}
	if dimension <= 0 {
		dimension = 1536
	}
	return &OpenAIAdapter{
		Embedder: Embedder{model: model, dimension: dimension},
		apiKey:   apiKey,
		model:    model,
	}
}

// Embed 实现向量化（占位：后续调用 OpenAI embeddings API）
func (a *OpenAIAdapter) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	out := make([][]float64, len(texts))
	for i := range out {
		out[i] = make([]float64, a.dimension)
	}
	return out, nil
}
