package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"rag-platform/pkg/proof"
)

type testJobStore struct {
	events []proof.Event
}

func (s testJobStore) ListEvents(ctx context.Context, jobID string) ([]proof.Event, error) {
	return s.events, nil
}

type testLedger struct {
	invocations []proof.ToolInvocation
}

func (l testLedger) ListToolInvocations(ctx context.Context, jobID string) ([]proof.ToolInvocation, error) {
	return l.invocations, nil
}

func makeProofEvents(jobID string, count int) []proof.Event {
	events := make([]proof.Event, 0, count)
	prevHash := ""

	for i := 0; i < count; i++ {
		e := proof.Event{
			ID:        strconv.Itoa(i + 1),
			JobID:     jobID,
			Type:      "test_event",
			Payload:   "{\"index\":" + strconv.Itoa(i) + "}",
			CreatedAt: time.Now().UTC().Add(time.Duration(i) * time.Second),
			PrevHash:  prevHash,
		}
		e.Hash = proof.ComputeEventHash(e)
		prevHash = e.Hash
		events = append(events, e)
	}

	return events
}

func TestVerifyEvidenceZip_Success(t *testing.T) {
	jobID := "job_cli_verify_success"
	zipBytes, err := proof.ExportEvidenceZip(
		context.Background(),
		jobID,
		testJobStore{events: makeProofEvents(jobID, 5)},
		testLedger{},
		proof.ExportOptions{RuntimeVersion: "test", SchemaVersion: "2.0"},
	)
	if err != nil {
		t.Fatalf("export evidence zip: %v", err)
	}

	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "ok.zip")
	if err := os.WriteFile(zipPath, zipBytes, 0644); err != nil {
		t.Fatalf("write zip: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := verifyEvidenceZip(zipPath, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%s", code, stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("Verification PASSED")) {
		t.Fatalf("expected success output, got: %s", stdout.String())
	}
}

func TestVerifyEvidenceZip_Tampered(t *testing.T) {
	jobID := "job_cli_verify_tampered"
	zipBytes, err := proof.ExportEvidenceZip(
		context.Background(),
		jobID,
		testJobStore{events: makeProofEvents(jobID, 5)},
		testLedger{},
		proof.ExportOptions{RuntimeVersion: "test", SchemaVersion: "2.0"},
	)
	if err != nil {
		t.Fatalf("export evidence zip: %v", err)
	}

	// 篡改 ZIP 字节，触发验证失败。
	if len(zipBytes) > 0 {
		zipBytes[len(zipBytes)-1] ^= 0xFF
	}

	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "tampered.zip")
	if err := os.WriteFile(zipPath, zipBytes, 0644); err != nil {
		t.Fatalf("write zip: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := verifyEvidenceZip(zipPath, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit code, got %d", code)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("Verification FAILED")) {
		t.Fatalf("expected failure output, got: %s", stdout.String())
	}
}

func TestBackfillHashesFile(t *testing.T) {
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "events.ndjson")
	outputPath := filepath.Join(tmpDir, "events.out.ndjson")
	input := `{"id":"1","job_id":"job_1","type":"job_created","payload":{"goal":"g1"},"created_at":"2026-02-13T10:00:00Z"}
{"id":"2","job_id":"job_1","type":"job_completed","payload":{},"created_at":"2026-02-13T10:00:01Z"}
`
	if err := os.WriteFile(inputPath, []byte(input), 0644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	n, err := backfillHashesFile(inputPath, outputPath)
	if err != nil {
		t.Fatalf("backfill hashes: %v", err)
	}
	if n != 2 {
		t.Fatalf("count = %d, want 2", n)
	}

	b, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	lines := bytes.Split(bytes.TrimSpace(b), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lines))
	}
	var first map[string]interface{}
	var second map[string]interface{}
	if err := json.Unmarshal(lines[0], &first); err != nil {
		t.Fatalf("unmarshal first: %v", err)
	}
	if err := json.Unmarshal(lines[1], &second); err != nil {
		t.Fatalf("unmarshal second: %v", err)
	}
	firstHash, _ := first["hash"].(string)
	secondPrev, _ := second["prev_hash"].(string)
	if firstHash == "" {
		t.Fatal("first hash should not be empty")
	}
	if secondPrev != firstHash {
		t.Fatalf("second prev_hash = %q, want %q", secondPrev, firstHash)
	}
}
