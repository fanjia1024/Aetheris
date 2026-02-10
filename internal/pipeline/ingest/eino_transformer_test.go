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
)

func TestSplitterTransformer_Transform(t *testing.T) {
	parser := NewDocumentParser()
	splitter := NewDocumentSplitter(200, 50, 100) // small chunk to get multiple chunks
	trans := NewSplitterTransformer(parser, splitter)
	ctx := context.Background()

	// One doc with enough content to split into multiple chunks
	src := []*schema.Document{
		{
			ID:      "doc1",
			Content: "First paragraph here.\n\nSecond paragraph there.\n\nThird paragraph and more text to exceed chunk size so we get at least two chunks from the splitter.",
			MetaData: map[string]any{"_source": "test.txt"},
		},
	}

	out, err := trans.Transform(ctx, src)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if len(out) == 0 {
		t.Error("expected at least one chunk")
	}
	// Check first output has document_id
	if len(out) > 0 {
		if out[0].MetaData == nil {
			t.Error("expected MetaData on chunk")
		} else if _, ok := out[0].MetaData["document_id"]; !ok && out[0].Content != "" {
			t.Errorf("expected document_id in MetaData: %v", out[0].MetaData)
		}
	}
}

func TestSplitterTransformer_Transform_empty(t *testing.T) {
	trans := NewSplitterTransformer(nil, nil)
	ctx := context.Background()

	out, err := trans.Transform(ctx, nil)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if out != nil {
		t.Errorf("expected nil output, got len=%d", len(out))
	}

	out, err = trans.Transform(ctx, []*schema.Document{})
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if out != nil {
		t.Errorf("expected nil output for empty input, got len=%d", len(out))
	}
}
