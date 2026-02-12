// Copyright 2026 fanjia1024
// AI-powered anomaly detection (3.0-M4)

package ai_forensics

import (
	"context"
)

// AnomalyDetector 异常决策检测器
type AnomalyDetector struct {
	threshold float64
}

// NewAnomalyDetector 创建异常检测器
func NewAnomalyDetector(threshold float64) *AnomalyDetector {
	return &AnomalyDetector{threshold: threshold}
}

// DetectAnomalies 检测异常决策
func (d *AnomalyDetector) DetectAnomalies(ctx context.Context, jobID string) ([]Anomaly, error) {
	// TODO: 实现异常检测逻辑
	return []Anomaly{}, nil
}

// Anomaly 异常记录
type Anomaly struct {
	JobID       string      `json:"job_id"`
	StepID      string      `json:"step_id"`
	Type        AnomalyType `json:"type"`
	Severity    string      `json:"severity"`
	Description string      `json:"description"`
	Evidence    []string    `json:"evidence"`
}

// AnomalyType 异常类型
type AnomalyType string

const (
	AnomalyMissingEvidence AnomalyType = "missing_evidence"
	AnomalyInconsistent    AnomalyType = "inconsistent"
	AnomalyTiming          AnomalyType = "timing"
	AnomalyLowConfidence   AnomalyType = "low_confidence"
)
