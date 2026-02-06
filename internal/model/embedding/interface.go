package embedding

import (
	"context"
)

// Embedder 向量化接口（占位：后续由 adapter 实现）
type Embedder struct {
	client   interface{}
	model    string
	dimension int
}

// Model 返回模型名称
func (e *Embedder) Model() string {
	if e == nil {
		return ""
	}
	return e.model
}

// Dimension 返回向量维度
func (e *Embedder) Dimension() int {
	if e == nil {
		return 0
	}
	return e.dimension
}

// Embed 对文本做向量化，返回与 texts 一一对应的向量
func (e *Embedder) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	if e == nil || len(texts) == 0 {
		return nil, nil
	}
	// 占位：返回空向量切片，后续由 adapter 实现
	out := make([][]float64, len(texts))
	for i := range out {
		out[i] = []float64{}
	}
	return out, nil
}
