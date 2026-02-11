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

package http

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"

	"rag-platform/internal/agent/job"
	"rag-platform/internal/runtime/jobstore"
)

func TestHealthCheck(t *testing.T) {
	h := server.Default(server.WithHostPorts(":0"))
	handler := NewHandler(nil, nil)
	h.GET("/api/health", func(ctx context.Context, c *app.RequestContext) {
		handler.HealthCheck(ctx, c)
	})
	w := ut.PerformRequest(h.Engine, "GET", "/api/health", &ut.Body{Body: bytes.NewReader(nil), Len: 0})
	resp := w.Result()
	if resp.StatusCode() != 200 {
		t.Errorf("HealthCheck status: got %d", resp.StatusCode())
	}
	if !bytes.Contains(resp.Body(), []byte("ok")) {
		t.Errorf("HealthCheck body: %s", resp.Body())
	}
}

// setupJobSignalHandler 创建处于 StatusWaiting 的 job 及带 job_waiting 事件的事件流，返回 handler 与 jobID（design/runtime-contract.md JobSignal 契约测试用）
func setupJobSignalHandler(t *testing.T) (*Handler, string) {
	t.Helper()
	ctx := context.Background()
	jobID := "j1"
	meta := job.NewJobStoreMem()
	j := &job.Job{ID: jobID, AgentID: "a1", Goal: "g1", Status: job.StatusWaiting}
	_, err := meta.Create(ctx, j)
	if err != nil {
		t.Fatalf("Create job: %v", err)
	}
	if err := meta.UpdateStatus(ctx, jobID, job.StatusWaiting); err != nil {
		t.Fatalf("UpdateStatus Waiting: %v", err)
	}
	eventStore := jobstore.NewMemoryStore()
	payloadWait, _ := json.Marshal(jobstore.JobWaitingPayload{
		NodeID: "n1", CorrelationKey: "expected-key", WaitType: "signal",
	})
	_, _ = eventStore.Append(ctx, jobID, 0, jobstore.JobEvent{JobID: jobID, Type: jobstore.JobCreated})
	_, _ = eventStore.Append(ctx, jobID, 1, jobstore.JobEvent{JobID: jobID, Type: jobstore.JobRunning})
	_, _ = eventStore.Append(ctx, jobID, 2, jobstore.JobEvent{JobID: jobID, Type: jobstore.JobWaiting, Payload: payloadWait})
	handler := NewHandler(nil, nil)
	handler.SetJobStore(meta)
	handler.SetJobEventStore(eventStore)
	return handler, jobID
}

// TestJobSignal_MissingCorrelationKey 违反契约：请求体缺少 correlation_key 时应返回 400（design/runtime-contract.md §5）
func TestJobSignal_MissingCorrelationKey(t *testing.T) {
	handler, jobID := setupJobSignalHandler(t)
	h := server.Default(server.WithHostPorts(":0"))
	h.POST("/api/jobs/:id/signal", func(ctx context.Context, c *app.RequestContext) {
		handler.JobSignal(ctx, c)
	})
	body := []byte(`{}`)
	w := ut.PerformRequest(h.Engine, "POST", "/api/jobs/"+jobID+"/signal", &ut.Body{Body: bytes.NewReader(body), Len: len(body)})
	resp := w.Result()
	if resp.StatusCode() != 400 {
		t.Errorf("JobSignal missing correlation_key: status got %d, want 400", resp.StatusCode())
	}
	if !bytes.Contains(resp.Body(), []byte("correlation_key")) {
		t.Errorf("JobSignal missing correlation_key: body %s", resp.Body())
	}
}

// TestJobSignal_WrongCorrelationKey 违反契约：correlation_key 与当前 job_waiting 不一致时应返回 400（design/runtime-contract.md §5）
func TestJobSignal_WrongCorrelationKey(t *testing.T) {
	handler, jobID := setupJobSignalHandler(t)
	h := server.Default(server.WithHostPorts(":0"))
	h.POST("/api/jobs/:id/signal", func(ctx context.Context, c *app.RequestContext) {
		handler.JobSignal(ctx, c)
	})
	body := []byte(`{"correlation_key":"wrong-key"}`)
	w := ut.PerformRequest(h.Engine, "POST", "/api/jobs/"+jobID+"/signal", &ut.Body{Body: bytes.NewReader(body), Len: len(body)})
	resp := w.Result()
	if resp.StatusCode() != 400 {
		t.Errorf("JobSignal wrong correlation_key: status got %d, want 400", resp.StatusCode())
	}
	if !bytes.Contains(resp.Body(), []byte("不匹配")) {
		t.Errorf("JobSignal wrong correlation_key: body %s", resp.Body())
	}
}
