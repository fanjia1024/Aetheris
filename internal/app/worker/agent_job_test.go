package worker

import (
	"context"
	"testing"
	"time"

	"rag-platform/internal/agent/job"
	"rag-platform/internal/runtime/jobstore"
	"rag-platform/pkg/log"
)

func TestExecuteJob_ReturnsAfterRunJob(t *testing.T) {
	logger, err := log.NewLogger(&log.Config{Level: "error"})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	meta := job.NewJobStoreMem()
	ev := jobstore.NewMemoryStore()
	r := NewAgentJobRunner(
		"worker-test",
		ev,
		meta,
		func(ctx context.Context, j *job.Job) error {
			return nil
		},
		10*time.Millisecond,
		100*time.Millisecond,
		1,
		nil,
		logger,
	)

	jid, err := meta.Create(context.Background(), &job.Job{
		AgentID: "a1",
		Goal:    "g1",
		Status:  job.StatusPending,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	done := make(chan struct{})
	go func() {
		r.executeJob(context.Background(), jid, "attempt-test")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("executeJob blocked after runJob returned")
	}
}
