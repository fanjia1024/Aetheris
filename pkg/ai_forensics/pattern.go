// Copyright 2026 fanjia1024
// Pattern matching for suspicious behavior (3.0-M4)

package ai_forensics

import (
	"context"
	"sort"
	"time"
)

// PatternMatcher 模式匹配器
type PatternMatcher struct {
	source DecisionSignalSource
}

// NewPatternMatcher 创建模式匹配器
func NewPatternMatcher() *PatternMatcher {
	return &PatternMatcher{}
}

// WithSignalSource 设置决策信号来源。
func (m *PatternMatcher) WithSignalSource(source DecisionSignalSource) *PatternMatcher {
	m.source = source
	return m
}

// FindSuspiciousPatterns 查找可疑模式
func (m *PatternMatcher) FindSuspiciousPatterns(ctx context.Context, jobs []string) ([]Pattern, error) {
	if len(jobs) == 0 || m.source == nil {
		return []Pattern{}, nil
	}

	inconsistentJobs := make([]string, 0)
	bypassJobs := make([]string, 0)
	abnormalTimingJobs := make([]string, 0)
	failureJobs := make([]string, 0)

	for _, jobID := range jobs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		signals, err := m.source.ListDecisionSignals(ctx, jobID)
		if err != nil {
			return nil, err
		}
		hasInconsistent := false
		hasBypass := false
		hasAbnormalTiming := false
		failedSteps := 0

		for _, s := range signals {
			if !s.Consistent {
				hasInconsistent = true
			}
			if s.BypassedApproval {
				hasBypass = true
			}
			if s.Duration > 15*time.Minute {
				hasAbnormalTiming = true
			}
			if s.Failed {
				failedSteps++
			}
		}

		if hasInconsistent {
			inconsistentJobs = append(inconsistentJobs, jobID)
		}
		if hasBypass {
			bypassJobs = append(bypassJobs, jobID)
		}
		if hasAbnormalTiming {
			abnormalTimingJobs = append(abnormalTimingJobs, jobID)
		}
		if failedSteps >= 2 {
			failureJobs = append(failureJobs, jobID)
		}
	}

	patterns := make([]Pattern, 0, 4)
	if len(inconsistentJobs) >= 2 {
		patterns = append(patterns, Pattern{
			Type:        PatternInconsistentDecisions,
			JobIDs:      inconsistentJobs,
			Description: "multiple jobs contain inconsistent decision signals",
			RiskScore:   82,
		})
	}
	if len(bypassJobs) > 0 {
		patterns = append(patterns, Pattern{
			Type:        PatternBypassApproval,
			JobIDs:      bypassJobs,
			Description: "approval bypass behavior detected",
			RiskScore:   95,
		})
	}
	if len(abnormalTimingJobs) >= 2 {
		patterns = append(patterns, Pattern{
			Type:        PatternAbnormalTiming,
			JobIDs:      abnormalTimingJobs,
			Description: "long-duration decision steps appear repeatedly",
			RiskScore:   70,
		})
	}
	if len(failureJobs) > 0 {
		patterns = append(patterns, Pattern{
			Type:        PatternRepeatedFailures,
			JobIDs:      failureJobs,
			Description: "repeated step failures detected in job executions",
			RiskScore:   76,
		})
	}

	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].RiskScore > patterns[j].RiskScore
	})
	return patterns, nil
}

// Pattern 可疑模式
type Pattern struct {
	Type        PatternType `json:"type"`
	JobIDs      []string    `json:"job_ids"`
	Description string      `json:"description"`
	RiskScore   float64     `json:"risk_score"`
}

// PatternType 模式类型
type PatternType string

const (
	PatternInconsistentDecisions PatternType = "inconsistent_decisions"
	PatternBypassApproval        PatternType = "bypass_approval"
	PatternAbnormalTiming        PatternType = "abnormal_timing"
	PatternRepeatedFailures      PatternType = "repeated_failures"
)
