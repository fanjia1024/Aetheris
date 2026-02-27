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

	einodoc "github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"

	"rag-platform/internal/pipeline/common"
)

// SplitterTransformer 实现 Eino document.Transformer，包装 Parser + Splitter，对 []*schema.Document 做解析与切片
type SplitterTransformer struct {
	parser   *DocumentParser
	splitter *DocumentSplitter
}

// NewSplitterTransformer 创建基于 Parser 与 Splitter 的 Eino Transformer
func NewSplitterTransformer(parser *DocumentParser, splitter *DocumentSplitter) *SplitterTransformer {
	if parser == nil {
		parser = NewDocumentParser()
	}
	if splitter == nil {
		splitter = NewDocumentSplitter(1000, 100, 1000)
	}
	return &SplitterTransformer{parser: parser, splitter: splitter}
}

// Transform 实现 github.com/cloudwego/eino/components/document.Transformer
func (s *SplitterTransformer) Transform(ctx context.Context, src []*schema.Document, opts ...einodoc.TransformerOption) ([]*schema.Document, error) {
	if len(src) == 0 {
		return nil, nil
	}
	pipeCtx := common.NewPipelineContext(ctx, "eino_transformer")
	var out []*schema.Document
	for _, d := range src {
		doc := SchemaDocumentToCommonDocument(d)
		if doc == nil {
			continue
		}
		// parser
		parsed, err := s.parser.Execute(pipeCtx, doc)
		if err != nil {
			return nil, fmt.Errorf("transformer parser: %w", err)
		}
		doc, ok := parsed.(*common.Document)
		if !ok {
			return nil, fmt.Errorf("parser did not return *common.Document，得到 %T", parsed)
		}
		// splitter
		split, err := s.splitter.Execute(pipeCtx, doc)
		if err != nil {
			return nil, fmt.Errorf("transformer splitter: %w", err)
		}
		doc, ok = split.(*common.Document)
		if !ok {
			return nil, fmt.Errorf("splitter did not return *common.Document，得到 %T", split)
		}
		chunks := ChunksToSchemaDocuments(doc)
		out = append(out, chunks...)
	}
	return out, nil
}
