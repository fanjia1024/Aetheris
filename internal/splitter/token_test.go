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
		"max_tokens":   3,
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
