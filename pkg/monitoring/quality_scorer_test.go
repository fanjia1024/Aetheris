// Copyright 2026 fanjia1024

package monitoring

import (
	"context"
	"testing"
)

// TestQualityScorer 测试质量评分
func TestQualityScorer(t *testing.T) {
	scorer := NewQualityScorer()

	score, err := scorer.ScoreDecision(context.Background(), "step_123")
	if err != nil {
		t.Fatalf("score decision failed: %v", err)
	}

	if score.Overall < 0 || score.Overall > 100 {
		t.Errorf("overall score should be 0-100, got %f", score.Overall)
	}
}
