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

package vector

import (
	"fmt"

	"rag-platform/pkg/config"
)

// NewStore 根据配置创建向量存储。type 为空或 "memory" 时返回内存实现；
// 其他类型（如 pgvector、milvus）需在此处扩展 case 并实现对应 Store。
// cfg.Addr/cfg.DB/cfg.Collection 供扩展后端连接与默认集合名使用。
func NewStore(cfg config.VectorConfig) (Store, error) {
	switch cfg.Type {
	case "", "memory":
		return NewMemoryStore(), nil
	default:
		return nil, fmt.Errorf("unsupported input type向量存储类型: %s（当前支持: memory）", cfg.Type)
	}
}
