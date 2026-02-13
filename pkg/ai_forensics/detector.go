// Copyright 2026 fanjia1024
// AI-powered anomaly detection (3.0-M4)

package ai_forensics

import (
	"context"
	"fmt"
	"time"
)

// AnomalyDetector 异常决策检测器
type AnomalyDetector struct {
	threshold       float64
	maxStepDuration time.Duration
	source          DecisionSignalSource
}

// DecisionSignalSource 决策信号来源。
type DecisionSignalSource interface {
	ListDecisionSignals(ctx context.Context, jobID string) ([]DecisionSignal, error)
}

// DecisionSignal 单步决策信号。
type DecisionSignal struct {
	StepID            string
	EvidenceCount     int
	Consistent        bool
	Duration          time.Duration
	Confidence        float64 // 0-1
	BypassedApproval  bool
	Failed            bool
	AdditionalDetails []string
}

// NewAnomalyDetector 创建异常检测器
func NewAnomalyDetector(threshold float64) *AnomalyDetector {
	if threshold <= 0 || threshold > 1 {
		threshold = 0.8
	}
	return &AnomalyDetector{
		threshold:       threshold,
		maxStepDuration: 15 * time.Minute,
	}
}

// WithSignalSource 设置决策信号来源。
func (d *AnomalyDetector) WithSignalSource(source DecisionSignalSource) *AnomalyDetector {
	d.source = source
	return d
}

// WithMaxStepDuration 设置判定 Timing 异常的阈值。
func (d *AnomalyDetector) WithMaxStepDuration(duration time.Duration) *AnomalyDetector {
	if duration > 0 {
		d.maxStepDuration = duration
	}
	return d
}

// DetectAnomalies 检测异常决策
func (d *AnomalyDetector) DetectAnomalies(ctx context.Context, jobID string) ([]Anomaly, error) {
	if d.source == nil || jobID == "" {
		return []Anomaly{}, nil
	}
	signals, err := d.source.ListDecisionSignals(ctx, jobID)
	if err != nil {
		return nil, err
	}

	anomalies := make([]Anomaly, 0)
	for _, s := range signals {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		stepID := s.StepID
		if stepID == "" {
			stepID = "unknown"
		}

		if s.EvidenceCount == 0 {
			anomalies = append(anomalies, Anomaly{
				JobID:       jobID,
				StepID:      stepID,
				Type:        AnomalyMissingEvidence,
				Severity:    "high",
				Description: "step has no supporting evidence",
				Evidence:    append([]string{"evidence_count=0"}, s.AdditionalDetails...),
			})
		}
		if !s.Consistent {
			anomalies = append(anomalies, Anomaly{
				JobID:       jobID,
				StepID:      stepID,
				Type:        AnomalyInconsistent,
				Severity:    "medium",
				Description: "step result is inconsistent with expected policy/flow",
				Evidence:    append([]string{"consistent=false"}, s.AdditionalDetails...),
			})
		}
		if d.maxStepDuration > 0 && s.Duration > d.maxStepDuration {
			sev := "medium"
			if s.Duration > 2*d.maxStepDuration {
				sev = "high"
			}
			anomalies = append(anomalies, Anomaly{
				JobID:       jobID,
				StepID:      stepID,
				Type:        AnomalyTiming,
				Severity:    sev,
				Description: "step took unusually long to finish",
				Evidence: []string{
					fmt.Sprintf("duration=%s", s.Duration),
					fmt.Sprintf("threshold=%s", d.maxStepDuration),
				},
			})
		}
		if s.Confidence >= 0 && s.Confidence < d.threshold {
			anomalies = append(anomalies, Anomaly{
				JobID:       jobID,
				StepID:      stepID,
				Type:        AnomalyLowConfidence,
				Severity:    lowConfidenceSeverity(s.Confidence, d.threshold),
				Description: "step confidence is below threshold",
				Evidence: []string{
					fmt.Sprintf("confidence=%.4f", s.Confidence),
					fmt.Sprintf("threshold=%.4f", d.threshold),
				},
			})
		}
	}

	return anomalies, nil
}

func lowConfidenceSeverity(confidence float64, threshold float64) string {
	gap := threshold - confidence
	if gap >= 0.3 {
		return "high"
	}
	if gap >= 0.15 {
		return "medium"
	}
	return "low"
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
