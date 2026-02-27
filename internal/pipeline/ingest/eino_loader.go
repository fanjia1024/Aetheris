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

package ingest

import (
	"context"
	"fmt"
	"strings"

	einodoc "github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"

	"rag-platform/internal/pipeline/common"
)

// URIDocumentLoader 实现 Eino document.Loader，包装 ingest.DocumentLoader，支持 Source.URI（文件路径或 file://）
type URIDocumentLoader struct {
	loader *DocumentLoader
}

// NewURIDocumentLoader 创建基于现有 DocumentLoader 的 Eino Loader
func NewURIDocumentLoader(loader *DocumentLoader) *URIDocumentLoader {
	if loader == nil {
		loader = NewDocumentLoader()
	}
	return &URIDocumentLoader{loader: loader}
}

// Load 实现 github.com/cloudwego/eino/components/document.Loader
func (u *URIDocumentLoader) Load(ctx context.Context, src einodoc.Source, opts ...einodoc.LoaderOption) ([]*schema.Document, error) {
	pathOrURI := strings.TrimSpace(src.URI)
	if pathOrURI == "" {
		return nil, fmt.Errorf("Source.URI 为空")
	}
	// 暂unsupported HTTP(S) URL
	if strings.HasPrefix(strings.ToLower(pathOrURI), "http://") || strings.HasPrefix(strings.ToLower(pathOrURI), "https://") {
		return nil, fmt.Errorf("暂unsupported HTTP(S) URL 加载，仅支持本地文件路径或 file://")
	}
	// file:// 前缀去掉得到本地路径（保留首字符以支持 /abs/path 或 Windows C:/）
	path := pathOrURI
	if strings.HasPrefix(strings.ToLower(pathOrURI), "file://") {
		path = pathOrURI[7:]
	}

	pipeCtx := common.NewPipelineContext(ctx, "eino_loader")
	out, err := u.loader.Execute(pipeCtx, path)
	if err != nil {
		return nil, fmt.Errorf("loader Execute: %w", err)
	}
	doc, ok := out.(*common.Document)
	if !ok {
		return nil, fmt.Errorf("loader did not return *common.Document，得到 %T", out)
	}
	sd := CommonDocumentToSchema(doc, src.URI)
	if sd == nil {
		return nil, fmt.Errorf("CommonDocumentToSchema 返回 nil")
	}
	return []*schema.Document{sd}, nil
}
