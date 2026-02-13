// Copyright 2026 fanjia1024

package ai_forensics

import (
	"context"
	"testing"
	"time"
)

type fakeSignalSource struct {
	data map[string][]DecisionSignal
}

func (f *fakeSignalSource) ListDecisionSignals(ctx context.Context, jobID string) ([]DecisionSignal, error) {
	return append([]DecisionSignal(nil), f.data[jobID]...), nil
}

// TestAnomalyDetector 测试异常检测器
func TestAnomalyDetector(t *testing.T) {
	detector := NewAnomalyDetector(0.8).
		WithSignalSource(&fakeSignalSource{
			data: map[string][]DecisionSignal{
				"job_123": {
					{
						StepID:        "step_1",
						EvidenceCount: 0,
						Consistent:    false,
						Duration:      31 * time.Minute,
						Confidence:    0.2,
					},
				},
			},
		}).
		WithMaxStepDuration(10 * time.Minute)

	anomalies, err := detector.DetectAnomalies(context.Background(), "job_123")
	if err != nil {
		t.Fatalf("detect anomalies failed: %v", err)
	}

	if anomalies == nil {
		t.Fatal("anomalies should not be nil")
	}
	if len(anomalies) < 4 {
		t.Fatalf("expected multiple anomalies, got %d", len(anomalies))
	}
}

// TestPatternMatcher 测试模式匹配
func TestPatternMatcher(t *testing.T) {
	matcher := NewPatternMatcher().WithSignalSource(&fakeSignalSource{
		data: map[string][]DecisionSignal{
			"job_1": {
				{StepID: "s1", Consistent: false, Duration: 20 * time.Minute, Failed: true, BypassedApproval: true},
				{StepID: "s2", Consistent: true, Failed: true},
			},
			"job_2": {
				{StepID: "s1", Consistent: false, Duration: 25 * time.Minute},
			},
		},
	})

	patterns, err := matcher.FindSuspiciousPatterns(context.Background(), []string{"job_1", "job_2"})
	if err != nil {
		t.Fatalf("find patterns failed: %v", err)
	}

	if patterns == nil {
		t.Fatal("patterns should not be nil")
	}
	if len(patterns) == 0 {
		t.Fatal("expected at least one suspicious pattern")
	}
}
