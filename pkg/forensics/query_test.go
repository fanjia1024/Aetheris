// Copyright 2026 fanjia1024
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package forensics

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

type fakeJobSource struct {
	jobs []JobSummary
	err  error
}

func (f *fakeJobSource) ListJobs(ctx context.Context, req QueryRequest) ([]JobSummary, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]JobSummary(nil), f.jobs...), nil
}

type fakeEventSource struct {
	events map[string][]Event
	err    error
}

func (f *fakeEventSource) ListEvents(ctx context.Context, jobID string) ([]Event, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]Event(nil), f.events[jobID]...), nil
}

type fakeExporter struct {
	data map[string][]byte
	err  error
}

func (f *fakeExporter) ExportEvidenceZip(ctx context.Context, jobID string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	if b, ok := f.data[jobID]; ok {
		return append([]byte(nil), b...), nil
	}
	return nil, fmt.Errorf("job not found: %s", jobID)
}

// TestQuery_TimeRange 测试时间范围过滤
func TestQuery_TimeRange(t *testing.T) {
	engine := NewQueryEngine().WithJobSource(&fakeJobSource{
		jobs: []JobSummary{
			{
				JobID:     "job_old",
				CreatedAt: time.Now().AddDate(0, 0, -10),
			},
			{
				JobID:     "job_new",
				CreatedAt: time.Now().AddDate(0, 0, -1),
			},
		},
	})

	req := QueryRequest{
		TenantID: "tenant_1",
		TimeRange: TimeRange{
			Start: time.Now().AddDate(0, 0, -7),
			End:   time.Now(),
		},
		Limit: 10,
	}

	result, err := engine.Query(context.Background(), req)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.TotalCount != 1 || len(result.Jobs) != 1 || result.Jobs[0].JobID != "job_new" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

// TestQuery_ToolFilter 测试 Tool 类型过滤
func TestQuery_ToolFilter(t *testing.T) {
	now := time.Now()
	engine := NewQueryEngine().
		WithJobSource(&fakeJobSource{
			jobs: []JobSummary{
				{JobID: "job_1", CreatedAt: now.Add(-time.Hour)},
				{JobID: "job_2", CreatedAt: now.Add(-2 * time.Hour)},
			},
		}).
		WithEventSource(&fakeEventSource{
			events: map[string][]Event{
				"job_1": {
					{Type: "tool_invocation_finished", Payload: []byte(`{"tool_name":"stripe.charge"}`)},
				},
				"job_2": {
					{Type: "tool_invocation_finished", Payload: []byte(`{"tool_name":"github.create_issue"}`)},
				},
			},
		})

	req := QueryRequest{
		TenantID: "tenant_1",
		TimeRange: TimeRange{
			Start: time.Now().AddDate(0, 0, -30),
			End:   time.Now(),
		},
		ToolFilter: []string{"stripe*", "sendgrid*"},
		Limit:      10,
	}

	result, err := engine.Query(context.Background(), req)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.TotalCount != 1 || len(result.Jobs) != 1 || result.Jobs[0].JobID != "job_1" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

// TestConsistencyCheck 测试一致性检查
func TestConsistencyCheck(t *testing.T) {
	engine := NewQueryEngine()

	report, err := engine.ConsistencyCheck(context.Background(), "job_123")
	if err != nil {
		t.Fatalf("consistency check failed: %v", err)
	}

	if report == nil {
		t.Fatal("report should not be nil")
	}

	if report.JobID != "job_123" {
		t.Errorf("expected job_id job_123, got %s", report.JobID)
	}
}

// TestBatchExport 测试批量导出
func TestBatchExport(t *testing.T) {
	engine := NewQueryEngine().WithExporter(&fakeExporter{
		data: map[string][]byte{
			"job_1": []byte("zip-1"),
			"job_2": []byte("zip-2"),
			"job_3": []byte("zip-3"),
		},
	})

	jobIDs := []string{"job_1", "job_2", "job_3"}
	result, err := engine.BatchExport(context.Background(), jobIDs)
	if err != nil {
		t.Fatalf("batch export failed: %v", err)
	}

	if result == nil {
		t.Fatal("result should not be nil")
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 exports, got %d", len(result))
	}
}

func TestQuery_JobSourceError(t *testing.T) {
	engine := NewQueryEngine().WithJobSource(&fakeJobSource{err: errors.New("boom")})
	_, err := engine.Query(context.Background(), QueryRequest{Limit: 10})
	if err == nil {
		t.Fatal("expected query error")
	}
}
