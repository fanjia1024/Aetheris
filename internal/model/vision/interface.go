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
