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
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

// MemoryStore 内存对象存储实现
type MemoryStore struct {
	objects map[string]*object
	mu      sync.RWMutex
}

// object 内存对象实现
type object struct {
	path      string
	data      []byte
	metadata  map[string]string
	createdAt int64
}

// NewMemoryStore 创建新的内存对象存储
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		objects: make(map[string]*object),
	}
}

// Put 上传对象
func (s *MemoryStore) Put(ctx context.Context, path string, data io.Reader, size int64, metadata map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 读取数据
	buffer := &bytes.Buffer{}
	if size > 0 {
		buffer.Grow(int(size))
	}

	_, err := io.Copy(buffer, data)
	if err != nil {
		return fmt.Errorf("failed to read object data: %w", err)
	}

	// 存储对象
	s.objects[path] = &object{
		path:      path,
		data:      buffer.Bytes(),
		metadata:  metadata,
		createdAt: time.Now().Unix(),
	}

	return nil
}

// Get 下载对象
func (s *MemoryStore) Get(ctx context.Context, path string) (io.ReadCloser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj, exists := s.objects[path]
	if !exists {
		return nil, fmt.Errorf("object with path %s not found", path)
	}

	// 返回字节读取器
	return io.NopCloser(bytes.NewReader(obj.data)), nil
}

// Delete 删除对象
func (s *MemoryStore) Delete(ctx context.Context, path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.objects[path]; !exists {
		return fmt.Errorf("object with path %s not found", path)
	}

	delete(s.objects, path)
	return nil
}

// List 列出对象
func (s *MemoryStore) List(ctx context.Context, prefix string) ([]*ObjectInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*ObjectInfo

	for path, obj := range s.objects {
		if prefix == "" || len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			results = append(results, &ObjectInfo{
				Path:      path,
				Size:      int64(len(obj.data)),
				Metadata:  obj.metadata,
				CreatedAt: obj.createdAt,
			})
		}
	}

	return results, nil
}

// Exists 检查对象是否存在
func (s *MemoryStore) Exists(ctx context.Context, path string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, exists := s.objects[path]
	return exists, nil
}

// GetMetadata 获取对象元数据
func (s *MemoryStore) GetMetadata(ctx context.Context, path string) (map[string]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj, exists := s.objects[path]
	if !exists {
		return nil, fmt.Errorf("object with path %s not found", path)
	}

	return obj.metadata, nil
}

// Close 关闭存储连接
func (s *MemoryStore) Close() error {
	return nil
}
