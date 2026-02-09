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

package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// MemoryStore 内存缓存存储实现
type MemoryStore struct {
	items map[string]*cacheItem
	mu    sync.RWMutex
}

// cacheItem 缓存项
type cacheItem struct {
	value      []byte
	expiration int64
}

// NewMemoryStore 创建新的内存缓存存储
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		items: make(map[string]*cacheItem),
	}
}

// Set 设置缓存
func (s *MemoryStore) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 序列化值
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal cache value: %w", err)
	}

	// 计算过期时间
	var exp int64
	if expiration > 0 {
		exp = time.Now().Add(expiration).Unix()
	}

	// 存储缓存项
	s.items[key] = &cacheItem{
		value:      data,
		expiration: exp,
	}

	return nil
}

// Get 获取缓存
func (s *MemoryStore) Get(ctx context.Context, key string, dest interface{}) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	item, exists := s.items[key]
	if !exists {
		return fmt.Errorf("cache item with key %s not found", key)
	}

	// 检查是否过期
	if item.expiration > 0 && item.expiration < time.Now().Unix() {
		return fmt.Errorf("cache item with key %s has expired", key)
	}

	// 反序列化值
	if err := json.Unmarshal(item.value, dest); err != nil {
		return fmt.Errorf("failed to unmarshal cache value: %w", err)
	}

	return nil
}

// Delete 删除缓存
func (s *MemoryStore) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.items[key]; !exists {
		return fmt.Errorf("cache item with key %s not found", key)
	}

	delete(s.items, key)
	return nil
}

// Exists 检查缓存是否存在
func (s *MemoryStore) Exists(ctx context.Context, key string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	item, exists := s.items[key]
	if !exists {
		return false, nil
	}

	// 检查是否过期
	if item.expiration > 0 && item.expiration < time.Now().Unix() {
		return false, nil
	}

	return true, nil
}

// Clear 清除所有缓存
func (s *MemoryStore) Clear(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.items = make(map[string]*cacheItem)
	return nil
}

// Close 关闭缓存连接
func (s *MemoryStore) Close() error {
	return nil
}
