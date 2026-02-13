package http

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"

	"rag-platform/internal/agent/job"
	"rag-platform/internal/runtime/jobstore"
)

func buildForensicsTestHandler(t *testing.T) (*Handler, string) {
	t.Helper()

	jobStore := job.NewJobStoreMem()
	eventStore := jobstore.NewMemoryStore()

	h := NewHandler(nil, nil)
	h.SetJobStore(jobStore)
	h.SetJobEventStore(eventStore)

	jobID, err := jobStore.Create(context.Background(), &job.Job{
		AgentID:  "agent-1",
		TenantID: "default",
		Goal:     "forensics test",
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	_, ver, err := eventStore.ListEvents(context.Background(), jobID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}

	appendEvent := func(ev jobstore.JobEvent) {
		ev.JobID = jobID
		newVer, err := eventStore.Append(context.Background(), jobID, ver, ev)
		if err != nil {
			t.Fatalf("append %s: %v", ev.Type, err)
		}
		ver = newVer
	}

	appendEvent(jobstore.JobEvent{Type: jobstore.JobCreated})

	startedPayload, _ := json.Marshal(map[string]interface{}{
		"invocation_id":   "inv-1",
		"idempotency_key": "key-1",
		"tool_name":       "stripe.charge",
		"arguments_hash":  "hash-1",
		"started_at":      time.Now().UTC().Format(time.RFC3339),
	})
	appendEvent(jobstore.JobEvent{Type: jobstore.ToolInvocationStarted, Payload: startedPayload})

	finishedPayload, _ := json.Marshal(map[string]interface{}{
		"invocation_id":   "inv-1",
		"idempotency_key": "key-1",
		"tool_name":       "stripe.charge",
		"outcome":         "success",
		"result":          map[string]interface{}{"ok": true},
		"finished_at":     time.Now().UTC().Format(time.RFC3339),
	})
	appendEvent(jobstore.JobEvent{Type: jobstore.ToolInvocationFinished, Payload: finishedPayload})

	reasoningPayload, _ := json.Marshal(map[string]interface{}{
		"step_id":     "step-1",
		"node_id":     "node-1",
		"type":        "tool",
		"label":       "charge card",
		"input_keys":  []string{"payment_request"},
		"output_keys": []string{"payment_result"},
		"evidence": map[string]interface{}{
			"tool_invocation_ids": []string{"inv-1"},
		},
	})
	appendEvent(jobstore.JobEvent{Type: jobstore.ReasoningSnapshot, Payload: reasoningPayload})

	auditPayload, _ := json.Marshal(map[string]interface{}{
		"action":  "export",
		"user_id": "user-1",
	})
	appendEvent(jobstore.JobEvent{Type: jobstore.AccessAudited, Payload: auditPayload})

	return h, jobID
}

func TestForensicsConsistencyCheck_OK(t *testing.T) {
	h, jobID := buildForensicsTestHandler(t)
	s := server.Default(server.WithHostPorts(":0"))
	s.GET("/api/forensics/consistency/:job_id", h.ForensicsConsistencyCheck)

	w := ut.PerformRequest(s.Engine, "GET", "/api/forensics/consistency/"+jobID, nil)
	if got := w.Result().StatusCode(); got != 200 {
		t.Fatalf("status = %d, want 200", got)
	}
	body := w.Result().Body()
	if !bytes.Contains(body, []byte(`"hash_chain_valid":true`)) {
		t.Fatalf("hash chain should be valid: %s", body)
	}
	if !bytes.Contains(body, []byte(`"ledger_consistent":true`)) {
		t.Fatalf("ledger should be consistent: %s", body)
	}
}

func TestGetJobEvidenceGraph_OK(t *testing.T) {
	h, jobID := buildForensicsTestHandler(t)
	s := server.Default(server.WithHostPorts(":0"))
	s.GET("/api/jobs/:id/evidence-graph", h.GetJobEvidenceGraph)

	w := ut.PerformRequest(s.Engine, "GET", "/api/jobs/"+jobID+"/evidence-graph", nil)
	if got := w.Result().StatusCode(); got != 200 {
		t.Fatalf("status = %d, want 200", got)
	}
	body := w.Result().Body()
	if !bytes.Contains(body, []byte(`"step_id":"step-1"`)) {
		t.Fatalf("evidence graph should contain reasoning node: %s", body)
	}
}

func TestGetJobAuditLog_OK(t *testing.T) {
	h, jobID := buildForensicsTestHandler(t)
	s := server.Default(server.WithHostPorts(":0"))
	s.GET("/api/jobs/:id/audit-log", h.GetJobAuditLog)

	w := ut.PerformRequest(s.Engine, "GET", "/api/jobs/"+jobID+"/audit-log", nil)
	if got := w.Result().StatusCode(); got != 200 {
		t.Fatalf("status = %d, want 200", got)
	}
	body := w.Result().Body()
	if !bytes.Contains(body, []byte(`"count":1`)) {
		t.Fatalf("audit log count should be 1: %s", body)
	}
}

func TestForensicsBatchExport_StatusFlow(t *testing.T) {
	h, jobID := buildForensicsTestHandler(t)
	s := server.Default(server.WithHostPorts(":0"))
	s.POST("/api/forensics/batch-export", h.ForensicsBatchExport)
	s.GET("/api/forensics/export-status/:task_id", h.ForensicsExportStatus)

	body := []byte(`{"job_ids":["` + jobID + `"]}`)
	w := ut.PerformRequest(s.Engine, "POST", "/api/forensics/batch-export", &ut.Body{Body: bytes.NewReader(body), Len: len(body)})
	if got := w.Result().StatusCode(); got != 202 {
		t.Fatalf("status = %d, want 202", got)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Result().Body(), &resp); err != nil {
		t.Fatalf("unmarshal batch export response: %v", err)
	}
	taskID, _ := resp["task_id"].(string)
	if taskID == "" {
		t.Fatalf("task_id should not be empty")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		statusResp := ut.PerformRequest(s.Engine, "GET", "/api/forensics/export-status/"+taskID, nil)
		if got := statusResp.Result().StatusCode(); got != 200 {
			t.Fatalf("status query code = %d, want 200", got)
		}
		if bytes.Contains(statusResp.Result().Body(), []byte(`"status":"completed"`)) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("batch export task did not complete in time: %s", statusResp.Result().Body())
		}
		time.Sleep(20 * time.Millisecond)
	}
}
