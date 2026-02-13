// Copyright 2026 fanjia1024

package monitoring

import (
	"context"
	"testing"
)

type fakeDecisionInputSource struct {
	input DecisionInput
	err   error
}

func (f *fakeDecisionInputSource) GetDecisionInput(ctx context.Context, stepID string) (DecisionInput, error) {
	if f.err != nil {
		return DecisionInput{}, f.err
	}
	return f.input, nil
}

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

func TestQualityScorer_WithInputSource(t *testing.T) {
	scorer := NewQualityScorer().WithInputSource(&fakeDecisionInputSource{
		input: DecisionInput{
			EvidenceCompleteness: 55,
			EvidenceQuality:      60,
			Confidence:           58,
			HumanOversight:       40,
		},
	})
	score, err := scorer.ScoreDecision(context.Background(), "step_abc")
	if err != nil {
		t.Fatalf("score decision failed: %v", err)
	}
	if score.Overall >= 70 {
		t.Fatalf("expected low overall score, got %f", score.Overall)
	}
	if len(score.Recommendations) == 0 {
		t.Fatal("expected recommendations for low-quality decision")
	}
}
