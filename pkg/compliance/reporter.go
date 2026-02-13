// Copyright 2026 fanjia1024
// Compliance report generator (3.0-M4)

package compliance

import (
	"context"
	"fmt"
	"math"
	"time"
)

// Reporter 合规报告生成器
type Reporter struct {
	template ComplianceTemplate
	source   MetricsSource
}

// MetricsSource 合规指标来源。
type MetricsSource interface {
	GetMetrics(ctx context.Context, tenantID string, timeRange TimeRange) (Metrics, error)
}

// Metrics 合规统计指标。
type Metrics struct {
	TotalChecks      int
	Violations       int
	MissingAuditLogs int
	UnredactedPII    int
}

// NewReporter 创建报告生成器
func NewReporter(template ComplianceTemplate) *Reporter {
	return &Reporter{template: template}
}

// WithMetricsSource 设置指标来源。
func (r *Reporter) WithMetricsSource(source MetricsSource) *Reporter {
	r.source = source
	return r
}

// GenerateReport 生成合规报告
func (r *Reporter) GenerateReport(ctx context.Context, tenantID string, timeRange TimeRange) (*ComplianceReport, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	if !timeRange.Start.IsZero() && !timeRange.End.IsZero() && timeRange.Start.After(timeRange.End) {
		return nil, fmt.Errorf("invalid time range: start is after end")
	}

	rate := 100.0
	if r.source != nil {
		metrics, err := r.source.GetMetrics(ctx, tenantID, timeRange)
		if err != nil {
			return nil, err
		}
		rate = calculateComplianceRate(metrics)
	}

	report := &ComplianceReport{
		TenantID:       tenantID,
		Standard:       r.template.Standard,
		TimeRange:      timeRange,
		ComplianceRate: rate,
	}
	return report, nil
}

// TimeRange 时间范围
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// ComplianceReport 合规报告
type ComplianceReport struct {
	TenantID       string    `json:"tenant_id"`
	Standard       string    `json:"standard"`
	TimeRange      TimeRange `json:"time_range"`
	ComplianceRate float64   `json:"compliance_rate"`
}

func calculateComplianceRate(m Metrics) float64 {
	if m.TotalChecks <= 0 {
		return 100
	}
	weightedViolations := float64(m.Violations) +
		1.0*float64(m.MissingAuditLogs) +
		1.5*float64(m.UnredactedPII)
	rate := 100 * (1 - weightedViolations/float64(m.TotalChecks))
	if rate < 0 {
		rate = 0
	}
	if rate > 100 {
		rate = 100
	}
	return math.Round(rate*10) / 10
}
