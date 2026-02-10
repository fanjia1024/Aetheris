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

package einoext

import (
	"strconv"

	"github.com/redis/go-redis/v9"

	"rag-platform/pkg/config"
)

// RedisOptionsFromVectorConfig 从 VectorConfig 构造 redis.Options（type=redis 时使用）
func RedisOptionsFromVectorConfig(cfg config.VectorConfig) (*redis.Options, error) {
	opts := &redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       0,
	}
	if cfg.Addr == "" {
		opts.Addr = "localhost:6379"
	}
	if cfg.DB != "" {
		db, err := strconv.Atoi(cfg.DB)
		if err == nil && db >= 0 {
			opts.DB = db
		}
	}
	// Redis Stack 向量检索需 Protocol 2、UnstableResp3 true（见 eino-ext retriever 注释）
	opts.Protocol = 2
	opts.UnstableResp3 = true
	return opts, nil
}
