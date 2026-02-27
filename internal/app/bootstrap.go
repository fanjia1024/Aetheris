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

package app

import (
	"fmt"

	"rag-platform/internal/storage/metadata"
	"rag-platform/internal/storage/vector"
	"rag-platform/pkg/config"
	"rag-platform/pkg/log"
)

// Bootstrap 统一初始化：供 api 与 worker 复用，避免在 cmd 内写业务与 pipeline
type Bootstrap struct {
	Config        *config.Config
	Logger        *log.Logger
	MetadataStore metadata.Store
	VectorStore   vector.Store
}

// NewBootstrap 根据配置创建 Bootstrap（DB/Cache/Models/Storage）
func NewBootstrap(cfg *config.Config) (*Bootstrap, error) {
	logCfg := &log.Config{}
	if cfg != nil {
		logCfg.Level = cfg.Log.Level
		logCfg.Format = cfg.Log.Format
		logCfg.File = cfg.Log.File
	}
	logger, err := log.NewLogger(logCfg)
	if err != nil {
		return nil, fmt.Errorf("初始化日志failed: %w", err)
	}

	var metaStore metadata.Store
	if cfg != nil {
		metaStore, err = metadata.NewStore(cfg.Storage.Metadata)
		if err != nil {
			return nil, fmt.Errorf("初始化元数据存储failed: %w", err)
		}
	}

	// type=memory 或空时创建 vector.Store；type=redis 等由 einoext 工厂创建，不创建 Store
	var vecStore vector.Store
	if cfg != nil {
		t := cfg.Storage.Vector.Type
		if t == "" || t == "memory" {
			vecStore, err = vector.NewStore(cfg.Storage.Vector)
			if err != nil {
				return nil, fmt.Errorf("初始化向量存储failed: %w", err)
			}
		}
	}

	return &Bootstrap{
		Config:        cfg,
		Logger:        logger,
		MetadataStore: metaStore,
		VectorStore:   vecStore,
	}, nil
}
