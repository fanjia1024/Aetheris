// Copyright 2026 fanjia1024

package ai_forensics

import (
	"context"
	"testing"
)

// TestAnomalyDetector 测试异常检测器
func TestAnomalyDetector(t *testing.T) {
	detector := NewAnomalyDetector(0.8)

	anomalies, err := detector.DetectAnomalies(context.Background(), "job_123")
	if err != nil {
		t.Fatalf("detect anomalies failed: %v", err)
	}

	if anomalies == nil {
		t.Fatal("anomalies should not be nil")
	}
}

// TestPatternMatcher 测试模式匹配
func TestPatternMatcher(t *testing.T) {
	matcher := NewPatternMatcher()

	patterns, err := matcher.FindSuspiciousPatterns(context.Background(), []string{"job_1", "job_2"})
	if err != nil {
		t.Fatalf("find patterns failed: %v", err)
	}

	if patterns == nil {
		t.Fatal("patterns should not be nil")
	}
}
