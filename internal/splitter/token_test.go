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

package splitter

import (
	"testing"
)

func TestTokenSplitter_Name(t *testing.T) {
	s := NewTokenSplitter()
	if s.Name() != "token_splitter" {
		t.Errorf("Name: got %q", s.Name())
	}
}

func TestTokenSplitter_Split_ShortContent(t *testing.T) {
	s := NewTokenSplitter()
	chunks, err := s.Split("hello world", nil)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for short text, got %d", len(chunks))
	}
	if chunks[0].Content != "hello world" {
		t.Errorf("chunk content: %q", chunks[0].Content)
	}
}

func TestTokenSplitter_Split_WithOptions(t *testing.T) {
	s := NewTokenSplitter()
	// 3 words per chunk max -> multiple chunks for 10 words
	options := map[string]interface{}{
		"max_tokens":    3,
		"chunk_overlap": 1,
	}
	content := "a b c d e f g h i j"
	chunks, err := s.Split(content, options)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks for 10 words with max_tokens=3, got %d", len(chunks))
	}
}

func TestTokenSplitter_Split_EmptyContent(t *testing.T) {
	s := NewTokenSplitter()
	chunks, err := s.Split("", nil)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("empty content should yield 0 chunks, got %d", len(chunks))
	}
}
