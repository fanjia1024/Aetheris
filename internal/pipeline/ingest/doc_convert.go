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
	"strconv"

	"github.com/cloudwego/eino/schema"

	"rag-platform/internal/pipeline/common"
)

// CommonDocumentToSchema 将 common.Document 转为单个 schema.Document（用于 Loader 输出）
func CommonDocumentToSchema(doc *common.Document, sourceURI string) *schema.Document {
	if doc == nil {
		return nil
	}
	meta := make(map[string]any)
	for k, v := range doc.Metadata {
		meta[k] = v
	}
	if sourceURI != "" {
		meta["_source"] = sourceURI
	}
	return &schema.Document{
		ID:       doc.ID,
		Content:  doc.Content,
		MetaData: meta,
	}
}

// SchemaDocumentToCommonDocument 将 schema.Document 转为 common.Document（用于 Transformer 输入）
func SchemaDocumentToCommonDocument(d *schema.Document) *common.Document {
	if d == nil {
		return nil
	}
	meta := make(map[string]interface{})
	if d.MetaData != nil {
		for k, v := range d.MetaData {
			meta[k] = v
		}
	}
	return &common.Document{
		ID:       d.ID,
		Content:  d.Content,
		Metadata: meta,
	}
}

// CommonChunkToSchemaDocument 将 common.Chunk 转为 schema.Document（用于 Transformer 输出 / Indexer 输入）
func CommonChunkToSchemaDocument(chunk common.Chunk, documentID string, embedding []float64) *schema.Document {
	meta := make(map[string]any)
	if chunk.Metadata != nil {
		for k, v := range chunk.Metadata {
			meta[k] = v
		}
	}
	meta["document_id"] = documentID
	meta["content"] = chunk.Content
	meta["index"] = strconv.Itoa(chunk.Index)
	meta["token_count"] = strconv.Itoa(chunk.TokenCount)
	sd := &schema.Document{
		ID:       chunk.ID,
		Content:  chunk.Content,
		MetaData: meta,
	}
	if len(embedding) > 0 {
		sd.WithDenseVector(embedding)
	}
	return sd
}

// ChunksToSchemaDocuments 将 common.Document 的 Chunks 转为 []*schema.Document
func ChunksToSchemaDocuments(doc *common.Document) []*schema.Document {
	if doc == nil || len(doc.Chunks) == 0 {
		return nil
	}
	out := make([]*schema.Document, 0, len(doc.Chunks))
	for i := range doc.Chunks {
		emb := doc.Chunks[i].Embedding
		out = append(out, CommonChunkToSchemaDocument(doc.Chunks[i], doc.ID, emb))
	}
	return out
}
