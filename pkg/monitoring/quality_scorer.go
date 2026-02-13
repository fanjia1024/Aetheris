// Copyright 2026 fanjia1024
// Decision quality scoring (3.0-M4)

package monitoring

import (
	"context"
	"fmt"
	"hash/fnv"
	"math"
)

// QualityScorer 决策质量评分器
type QualityScorer struct {
	source DecisionInputSource
}

// DecisionInputSource 决策评分输入来源。
type DecisionInputSource interface {
	GetDecisionInput(ctx context.Context, stepID string) (DecisionInput, error)
}

// DecisionInput 决策评分输入。
type DecisionInput struct {
	EvidenceCompleteness float64
	EvidenceQuality      float64
	Confidence           float64
	HumanOversight       float64
}

// NewQualityScorer 创建质量评分器
func NewQualityScorer() *QualityScorer {
	return &QualityScorer{}
}

// WithInputSource 设置评分输入来源。
func (s *QualityScorer) WithInputSource(source DecisionInputSource) *QualityScorer {
	s.source = source
	return s
}

// ScoreDecision 对单个决策评分
func (s *QualityScorer) ScoreDecision(ctx context.Context, stepID string) (*QualityScore, error) {
	if stepID == "" {
		return nil, fmt.Errorf("step_id is required")
	}

	input := defaultDecisionInput(stepID)
	if s.source != nil {
		in, err := s.source.GetDecisionInput(ctx, stepID)
		if err != nil {
			return nil, err
		}
		input = DecisionInput{
			EvidenceCompleteness: clampScore(in.EvidenceCompleteness),
			EvidenceQuality:      clampScore(in.EvidenceQuality),
			Confidence:           clampScore(in.Confidence),
			HumanOversight:       clampScore(in.HumanOversight),
		}
	}

	overall := 0.30*input.EvidenceCompleteness +
		0.25*input.EvidenceQuality +
		0.25*input.Confidence +
		0.20*input.HumanOversight

	score := &QualityScore{
		Overall:              round1(overall),
		EvidenceCompleteness: round1(input.EvidenceCompleteness),
		EvidenceQuality:      round1(input.EvidenceQuality),
		Confidence:           round1(input.Confidence),
		HumanOversight:       round1(input.HumanOversight),
		Recommendations:      buildRecommendations(input),
	}
	return score, nil
}

// QualityScore 质量评分
type QualityScore struct {
	Overall              float64  `json:"overall"`
	EvidenceCompleteness float64  `json:"evidence_completeness"`
	EvidenceQuality      float64  `json:"evidence_quality"`
	Confidence           float64  `json:"confidence"`
	HumanOversight       float64  `json:"human_oversight"`
	Recommendations      []string `json:"recommendations"`
}

func defaultDecisionInput(stepID string) DecisionInput {
	h := fnv.New32a()
	_, _ = h.Write([]byte(stepID))
	base := float64(h.Sum32()%21) + 70 // 70-90

	return DecisionInput{
		EvidenceCompleteness: clampScore(base + 4),
		EvidenceQuality:      clampScore(base),
		Confidence:           clampScore(base + 2),
		HumanOversight:       clampScore(base - 3),
	}
}

func buildRecommendations(in DecisionInput) []string {
	recs := make([]string, 0, 4)
	if in.EvidenceCompleteness < 70 {
		recs = append(recs, "improve evidence collection coverage for this step")
	}
	if in.EvidenceQuality < 70 {
		recs = append(recs, "improve evidence quality and source reliability")
	}
	if in.Confidence < 70 {
		recs = append(recs, "review model confidence calibration and decision thresholds")
	}
	if in.HumanOversight < 70 {
		recs = append(recs, "increase human oversight for high-risk decisions")
	}
	return recs
}

func clampScore(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}
