package compliance

import (
	"context"
	"testing"
	"time"
)

type fakeMetricsSource struct {
	metrics Metrics
	err     error
}

func (f *fakeMetricsSource) GetMetrics(ctx context.Context, tenantID string, timeRange TimeRange) (Metrics, error) {
	if f.err != nil {
		return Metrics{}, f.err
	}
	return f.metrics, nil
}

func TestGenerateReport_DefaultRate(t *testing.T) {
	r := NewReporter(TemplateGDPR)
	report, err := r.GenerateReport(context.Background(), "tenant_1", TimeRange{})
	if err != nil {
		t.Fatalf("generate report failed: %v", err)
	}
	if report.ComplianceRate != 100 {
		t.Fatalf("default compliance rate = %v, want 100", report.ComplianceRate)
	}
}

func TestGenerateReport_WithMetrics(t *testing.T) {
	r := NewReporter(TemplateGDPR).WithMetricsSource(&fakeMetricsSource{
		metrics: Metrics{
			TotalChecks:      100,
			Violations:       5,
			MissingAuditLogs: 3,
			UnredactedPII:    2,
		},
	})
	report, err := r.GenerateReport(context.Background(), "tenant_1", TimeRange{})
	if err != nil {
		t.Fatalf("generate report failed: %v", err)
	}
	if report.ComplianceRate >= 95 || report.ComplianceRate <= 80 {
		t.Fatalf("unexpected compliance rate: %v", report.ComplianceRate)
	}
}

func TestGenerateReport_InvalidTimeRange(t *testing.T) {
	r := NewReporter(TemplateGDPR)
	_, err := r.GenerateReport(context.Background(), "tenant_1", TimeRange{
		Start: time.Now(),
		End:   time.Now().Add(-time.Hour),
	})
	if err == nil {
		t.Fatal("expected invalid time range error")
	}
}
