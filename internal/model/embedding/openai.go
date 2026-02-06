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
