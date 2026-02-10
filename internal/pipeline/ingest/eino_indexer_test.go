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
	"testing"

	"github.com/cloudwego/eino/schema"

	"rag-platform/internal/storage/vector"
)

func TestMemoryIndexer_Store(t *testing.T) {
	ctx := context.Background()
	store := vector.NewMemoryStore()
	dim := 4
	if err := vector.EnsureIndex(ctx, store, "default", dim, "cosine"); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	idx, err := NewMemoryIndexer(&MemoryIndexerConfig{
		VectorStore:       store,
		DefaultCollection: "default",
		BatchSize:         10,
	})
	if err != nil {
		t.Fatalf("NewMemoryIndexer: %v", err)
	}

	docs := []*schema.Document{
		{ID: "doc1", Content: "hello", MetaData: map[string]any{"document_id": "parent1"}},
		{ID: "doc2", Content: "world", MetaData: map[string]any{"document_id": "parent1"}},
	}
	vec := []float64{1, 0, 0, 0}
	for _, d := range docs {
		d.WithDenseVector(vec)
	}

	ids, err := idx.Store(ctx, docs)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 ids, got %d", len(ids))
	}

	// 通过 vector store 验证写入
	for _, id := range ids {
		v, err := store.Get(ctx, "default", id)
		if err != nil {
			t.Errorf("Get %s: %v", id, err)
		}
		if v == nil || len(v.Values) != dim {
			t.Errorf("Get %s: bad vector", id)
		}
	}
}
