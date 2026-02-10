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
	"os"
	"path/filepath"
	"testing"

	einodoc "github.com/cloudwego/eino/components/document"
)

func TestURIDocumentLoader_Load_filePath(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(f, []byte("hello world"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	loader := NewDocumentLoader()
	uriLoader := NewURIDocumentLoader(loader)
	ctx := context.Background()

	docs, err := uriLoader.Load(ctx, einodoc.Source{URI: f})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(docs) != 1 {
		t.Errorf("expected 1 doc, got %d", len(docs))
	}
	if len(docs) > 0 {
		if docs[0].Content != "hello world" {
			t.Errorf("content = %q", docs[0].Content)
		}
		if docs[0].MetaData != nil && docs[0].MetaData["_source"] != f {
			t.Errorf("_source = %v", docs[0].MetaData["_source"])
		}
	}
}

func TestURIDocumentLoader_Load_fileURI(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(f, []byte("file:// test"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	loader := NewDocumentLoader()
	uriLoader := NewURIDocumentLoader(loader)
	ctx := context.Background()

	fileURI := "file://" + f
	docs, err := uriLoader.Load(ctx, einodoc.Source{URI: fileURI})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(docs) != 1 {
		t.Errorf("expected 1 doc, got %d", len(docs))
	}
	if len(docs) > 0 && docs[0].Content != "file:// test" {
		t.Errorf("content = %q", docs[0].Content)
	}
}

func TestURIDocumentLoader_Load_httpUnsupported(t *testing.T) {
	loader := NewDocumentLoader()
	uriLoader := NewURIDocumentLoader(loader)
	ctx := context.Background()

	_, err := uriLoader.Load(ctx, einodoc.Source{URI: "https://example.com/doc.pdf"})
	if err == nil {
		t.Error("expected error for HTTP URL")
	}
}
