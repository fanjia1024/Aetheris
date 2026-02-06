package vision

import (
	"context"
)

// Client 视觉模型接口（占位：后续由 adapter 实现多模态）
type Client interface {
	// Describe 描述图像内容
	Describe(ctx context.Context, imageURLOrBase64 string) (string, error)
	// Name 返回模型名称
	Name() string
}

// StubClient 占位实现
type StubClient struct{}

// Describe 占位
func (s *StubClient) Describe(ctx context.Context, imageURLOrBase64 string) (string, error) {
	return "vision stub", nil
}

// Name 占位
func (s *StubClient) Name() string {
	return "stub"
}
