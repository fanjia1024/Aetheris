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

package query

import (
	"context"
	"testing"

	einoembed "github.com/cloudwego/eino/components/embedding"
	einoretriever "github.com/cloudwego/eino/components/retriever"

	"rag-platform/internal/storage/vector"
)

// mockEinoEmbedder 测试用：固定返回 4 维向量
type mockEinoEmbedder struct{}

func (m *mockEinoEmbedder) EmbedStrings(ctx context.Context, texts []string, _ ...einoembed.Option) ([][]float64, error) {
	vec := []float64{1, 0, 0, 0}
	out := make([][]float64, len(texts))
	for i := range out {
		out[i] = vec
	}
	return out, nil
}

func TestMemoryRetriever_Retrieve(t *testing.T) {
	ctx := context.Background()
	store := vector.NewMemoryStore()
	dim := 4
	if err := vector.EnsureIndex(ctx, store, "default", dim, "cosine"); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}
	// 预先写入一条向量
	vec := []float64{1, 0, 0, 0}
	err := store.Add(ctx, "default", []*vector.Vector{
		{ID: "chunk1", Values: vec, Metadata: map[string]string{"content": "hello", "document_id": "doc1"}},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	ret, err := NewMemoryRetriever(&MemoryRetrieverConfig{
		VectorStore: store, DefaultIndex: "default", DefaultTopK: 5, DefaultThreshold: 0.1,
	})
	if err != nil {
		t.Fatalf("NewMemoryRetriever: %v", err)
	}

	docs, err := ret.Retrieve(ctx, "hello", einoretriever.WithEmbedding(&mockEinoEmbedder{}))
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(docs) == 0 {
		t.Error("expected at least one doc")
	}
	if len(docs) > 0 {
		if docs[0].ID != "chunk1" || docs[0].Content != "hello" {
			t.Errorf("unexpected doc: id=%s content=%s", docs[0].ID, docs[0].Content)
		}
	}
}
