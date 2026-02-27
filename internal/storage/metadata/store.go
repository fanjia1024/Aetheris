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

package metadata

import (
	"fmt"

	"rag-platform/pkg/config"
)

// NewStore 根据配置创建元数据存储（当前仅支持 memory）
func NewStore(cfg config.MetadataConfig) (Store, error) {
	switch cfg.Type {
	case "", "memory":
		return NewMemoryStore(), nil
	default:
		return nil, fmt.Errorf("unsupported input元数据存储类型: %s", cfg.Type)
	}
}
