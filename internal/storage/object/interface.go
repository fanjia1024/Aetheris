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

package object

import (
	"context"
	"io"
)

// Store 对象存储接口
type Store interface {
	// Put 上传对象
	Put(ctx context.Context, path string, data io.Reader, size int64, metadata map[string]string) error
	// Get 下载对象
	Get(ctx context.Context, path string) (io.ReadCloser, error)
	// Delete 删除对象
	Delete(ctx context.Context, path string) error
	// List 列出对象
	List(ctx context.Context, prefix string) ([]*ObjectInfo, error)
	// Exists 检查对象是否存在
	Exists(ctx context.Context, path string) (bool, error)
	// GetMetadata 获取对象元数据
	GetMetadata(ctx context.Context, path string) (map[string]string, error)
	// Close 关闭存储连接
	Close() error
}

// ObjectInfo 对象信息
type ObjectInfo struct {
	Path      string            `json:"path"`       // 对象路径
	Size      int64             `json:"size"`       // 对象大小
	Metadata  map[string]string `json:"metadata"`   // 对象元数据
	CreatedAt int64             `json:"created_at"` // 创建时间
}
