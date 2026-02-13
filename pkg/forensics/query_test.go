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
	"testing"
	"time"
)

// TestQuery_TimeRange 测试时间范围过滤
func TestQuery_TimeRange(t *testing.T) {
	engine := NewQueryEngine()

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
}

// TestQuery_ToolFilter 测试 Tool 类型过滤
func TestQuery_ToolFilter(t *testing.T) {
	engine := NewQueryEngine()

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
	engine := NewQueryEngine()

	jobIDs := []string{"job_1", "job_2", "job_3"}
	result, err := engine.BatchExport(context.Background(), jobIDs)
	if err != nil {
		t.Fatalf("batch export failed: %v", err)
	}

	if result == nil {
		t.Fatal("result should not be nil")
	}
}
