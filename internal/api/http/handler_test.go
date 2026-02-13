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
	"sync"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"

	"rag-platform/internal/agent/job"
	"rag-platform/internal/agent/signal"
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

type fakeSignalInbox struct {
	mu          sync.Mutex
	appendCalls int
	ackCalls    int
	lastAppend  struct {
		jobID          string
		correlationKey string
		payload        []byte
	}
	lastAck struct {
		jobID string
		id    string
	}
}

func (f *fakeSignalInbox) Append(ctx context.Context, jobID, correlationKey string, payload []byte) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.appendCalls++
	f.lastAppend.jobID = jobID
	f.lastAppend.correlationKey = correlationKey
	f.lastAppend.payload = append([]byte(nil), payload...)
	return "sig-1", nil
}

func (f *fakeSignalInbox) MarkAcked(ctx context.Context, jobID, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ackCalls++
	f.lastAck.jobID = jobID
	f.lastAck.id = id
	return nil
}

func (f *fakeSignalInbox) snapshot() (appendCalls, ackCalls int, appendJobID, appendCorrelation, ackJobID, ackID string, payload []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.appendCalls, f.ackCalls, f.lastAppend.jobID, f.lastAppend.correlationKey, f.lastAck.jobID, f.lastAck.id, append([]byte(nil), f.lastAppend.payload...)
}

// TestJobSignal_SuccessAndIdempotentUnblock 验证 signal 首次送达写 wait_completed 并置 Pending，重复送达（状态仍 Waiting 场景）按 correlation_key 幂等处理且不重复写事件。
func TestJobSignal_SuccessAndIdempotentUnblock(t *testing.T) {
	ctx := context.Background()
	handler, jobID := setupJobSignalHandler(t)
	inbox := &fakeSignalInbox{}
	handler.SetSignalInbox(inbox)
	h := server.Default(server.WithHostPorts(":0"))
	h.POST("/api/jobs/:id/signal", func(ctx context.Context, c *app.RequestContext) {
		handler.JobSignal(ctx, c)
	})

	body := []byte(`{"correlation_key":"expected-key","payload":{"approved":true}}`)
	w1 := ut.PerformRequest(h.Engine, "POST", "/api/jobs/"+jobID+"/signal", &ut.Body{Body: bytes.NewReader(body), Len: len(body)})
	resp1 := w1.Result()
	if resp1.StatusCode() != 200 {
		t.Fatalf("first JobSignal status got %d, want 200", resp1.StatusCode())
	}
	if !bytes.Contains(resp1.Body(), []byte("重新入队执行")) {
		t.Fatalf("first JobSignal body: %s", resp1.Body())
	}

	j, err := handler.jobStore.Get(ctx, jobID)
	if err != nil {
		t.Fatalf("jobStore.Get: %v", err)
	}
	if j == nil || j.Status != job.StatusPending {
		t.Fatalf("job status = %v, want Pending", j)
	}
	events1, _, err := handler.jobEventStore.ListEvents(ctx, jobID)
	if err != nil {
		t.Fatalf("ListEvents after first signal: %v", err)
	}
	waitCompletedCount1 := 0
	for _, e := range events1 {
		if e.Type == jobstore.WaitCompleted {
			waitCompletedCount1++
		}
	}
	if waitCompletedCount1 != 1 {
		t.Fatalf("wait_completed count after first signal = %d, want 1", waitCompletedCount1)
	}

	// 模拟重复请求命中“状态仍 Waiting（最终一致性）”场景，验证幂等逻辑只返回已送达，不重复追加事件。
	if err := handler.jobStore.UpdateStatus(ctx, jobID, job.StatusWaiting); err != nil {
		t.Fatalf("reset status to waiting: %v", err)
	}
	w2 := ut.PerformRequest(h.Engine, "POST", "/api/jobs/"+jobID+"/signal", &ut.Body{Body: bytes.NewReader(body), Len: len(body)})
	resp2 := w2.Result()
	if resp2.StatusCode() != 200 {
		t.Fatalf("second JobSignal status got %d, want 200", resp2.StatusCode())
	}
	if !bytes.Contains(resp2.Body(), []byte("幂等")) {
		t.Fatalf("second JobSignal should be idempotent, body: %s", resp2.Body())
	}

	events2, _, err := handler.jobEventStore.ListEvents(ctx, jobID)
	if err != nil {
		t.Fatalf("ListEvents after second signal: %v", err)
	}
	waitCompletedCount2 := 0
	for _, e := range events2 {
		if e.Type == jobstore.WaitCompleted {
			waitCompletedCount2++
		}
	}
	if waitCompletedCount2 != 1 {
		t.Fatalf("wait_completed count after idempotent signal = %d, want 1", waitCompletedCount2)
	}

	appendCalls, ackCalls, appendJobID, appendCorrelation, ackJobID, ackID, payload := inbox.snapshot()
	if appendCalls != 1 || ackCalls != 1 {
		t.Fatalf("inbox calls append=%d ack=%d, want append=1 ack=1", appendCalls, ackCalls)
	}
	if appendJobID != jobID || ackJobID != jobID {
		t.Fatalf("inbox job_id append=%q ack=%q, want %q", appendJobID, ackJobID, jobID)
	}
	if appendCorrelation != "expected-key" {
		t.Fatalf("inbox correlation_key=%q, want expected-key", appendCorrelation)
	}
	if ackID != "sig-1" {
		t.Fatalf("inbox ack id=%q, want sig-1", ackID)
	}
	if !bytes.Contains(payload, []byte(`"approved":true`)) {
		t.Fatalf("inbox payload got %s, want approved=true", payload)
	}
}

var _ signal.SignalInbox = (*fakeSignalInbox)(nil)
