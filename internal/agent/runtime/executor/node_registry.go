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

package executor

import (
	"sort"
	"sync"
)

// NodeAdapterRegistry 提供节点适配器的注册与发现能力，支持运行时扩展自定义节点类型。
type NodeAdapterRegistry struct {
	mu       sync.RWMutex
	adapters map[string]NodeAdapter
}

// NewNodeAdapterRegistry 创建节点注册表；base 非 nil 时会复制初始适配器集合。
func NewNodeAdapterRegistry(base map[string]NodeAdapter) *NodeAdapterRegistry {
	r := &NodeAdapterRegistry{adapters: make(map[string]NodeAdapter)}
	for k, v := range base {
		r.adapters[k] = v
	}
	return r
}

// Register 注册或覆盖一个节点类型适配器。
func (r *NodeAdapterRegistry) Register(nodeType string, adapter NodeAdapter) {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.adapters == nil {
		r.adapters = make(map[string]NodeAdapter)
	}
	r.adapters[nodeType] = adapter
	r.mu.Unlock()
}

// Get 获取某个节点类型的适配器。
func (r *NodeAdapterRegistry) Get(nodeType string) (NodeAdapter, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	v, ok := r.adapters[nodeType]
	r.mu.RUnlock()
	return v, ok
}

// List 返回当前已注册的节点类型（按字典序排序），用于 discovery。
func (r *NodeAdapterRegistry) List() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	out := make([]string, 0, len(r.adapters))
	for k := range r.adapters {
		out = append(out, k)
	}
	r.mu.RUnlock()
	sort.Strings(out)
	return out
}

// AsMap 返回当前适配器快照副本。
func (r *NodeAdapterRegistry) AsMap() map[string]NodeAdapter {
	if r == nil {
		return map[string]NodeAdapter{}
	}
	r.mu.RLock()
	out := make(map[string]NodeAdapter, len(r.adapters))
	for k, v := range r.adapters {
		out[k] = v
	}
	r.mu.RUnlock()
	return out
}
