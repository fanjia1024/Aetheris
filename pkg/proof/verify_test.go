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

package proof

import (
	"archive/zip"
	"bytes"
	"context"
	"testing"
	"time"
)

// 测试 helper: 创建测试事件
func makeTestEvents(jobID string, count int) []Event {
	events := make([]Event, count)
	prevHash := ""
	for i := 0; i < count; i++ {
		event := Event{
			ID:        string(rune(i + 1)),
			JobID:     jobID,
			Type:      "test_event",
			Payload:   `{"index":` + string(rune(i)) + `}`,
			CreatedAt: time.Now().UTC().Add(time.Duration(i) * time.Second),
			PrevHash:  prevHash,
		}
		event.Hash = ComputeEventHash(event)
		prevHash = event.Hash
		events[i] = event
	}
	return events
}

// 内存 JobStore 实现（用于测试）
type memJobStore struct {
	events []Event
}

func (m memJobStore) ListEvents(ctx context.Context, jobID string) ([]Event, error) {
	return m.events, nil
}

// 内存 Ledger 实现（用于测试）
type memLedger struct {
	invocations []ToolInvocation
}

func (m memLedger) ListToolInvocations(ctx context.Context, jobID string) ([]ToolInvocation, error) {
	return m.invocations, nil
}

// TestEvidence_Valid 正常证据包验证通过
func TestEvidence_Valid(t *testing.T) {
	jobID := "job_test_1"
	events := makeTestEvents(jobID, 10)

	zipBytes, err := ExportEvidenceZip(context.Background(), jobID,
		memJobStore{events: events},
		memLedger{invocations: nil},
		ExportOptions{RuntimeVersion: "test", SchemaVersion: "2.0"},
	)
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	result := VerifyEvidenceZip(zipBytes)
	if !result.OK {
		t.Errorf("verification should pass, but got errors: %v", result.Errors)
	}
	if !result.HashChainValid {
		t.Error("hash chain should be valid")
	}
	if !result.ManifestValid {
		t.Error("manifest should be valid")
	}
}

// TestEvidence_TamperEvent 篡改事件内容，验证失败
func TestEvidence_TamperEvent(t *testing.T) {
	jobID := "job_test_2"
	events := makeTestEvents(jobID, 10)

	zipBytes, err := ExportEvidenceZip(context.Background(), jobID,
		memJobStore{events: events},
		memLedger{invocations: nil},
		ExportOptions{RuntimeVersion: "test"},
	)
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	// 篡改 ZIP 中的 events.ndjson
	tamperedZip := tamperZipFile(zipBytes, "events.ndjson", func(b []byte) []byte {
		// 修改第 10 个字节
		if len(b) > 10 {
			b[10] ^= 0xFF
		}
		return b
	})

	result := VerifyEvidenceZip(tamperedZip)
	if result.OK {
		t.Error("verification should fail after tampering")
	}
}

// TestEvidence_DeleteMiddleEvent 删除中间事件，哈希链断裂
func TestEvidence_DeleteMiddleEvent(t *testing.T) {
	jobID := "job_test_3"
	events := makeTestEvents(jobID, 10)

	zipBytes, err := ExportEvidenceZip(context.Background(), jobID,
		memJobStore{events: events},
		memLedger{invocations: nil},
		ExportOptions{RuntimeVersion: "test"},
	)
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	// 删除 events.ndjson 中的一行
	tamperedZip := tamperZipFile(zipBytes, "events.ndjson", func(b []byte) []byte {
		return deleteNDJSONLine(b, 5)
	})

	result := VerifyEvidenceZip(tamperedZip)
	if result.OK {
		t.Error("verification should fail after deleting event")
	}
}

// TestEvidence_LedgerMismatch Ledger 与 events 不一致
func TestEvidence_LedgerMismatch(t *testing.T) {
	jobID := "job_test_4"
	events := makeTestEvents(jobID, 10)

	// 添加一个不在 ledger 中的 tool_invocation_finished 事件
	finishedEvent := Event{
		ID:        "11",
		JobID:     jobID,
		Type:      "tool_invocation_finished",
		Payload:   `{"idempotency_key":"key_123","outcome":"success","tool_name":"test_tool"}`,
		CreatedAt: time.Now().UTC(),
		PrevHash:  events[len(events)-1].Hash,
	}
	finishedEvent.Hash = ComputeEventHash(finishedEvent)
	events = append(events, finishedEvent)

	// Ledger 为空（不包含 key_123）
	zipBytes, err := ExportEvidenceZip(context.Background(), jobID,
		memJobStore{events: events},
		memLedger{invocations: []ToolInvocation{}},
		ExportOptions{RuntimeVersion: "test"},
	)
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	result := VerifyEvidenceZip(zipBytes)
	// 验证应该失败（ledger 不一致）
	if result.LedgerValid {
		t.Error("ledger validation should fail when event finished but ledger missing")
	}
}

// TestHashChain_Valid 验证正常哈希链
func TestHashChain_Valid(t *testing.T) {
	events := makeTestEvents("job_x", 5)
	if err := ValidateChain(events); err != nil {
		t.Errorf("hash chain should be valid: %v", err)
	}
}

// TestHashChain_Broken 验证断裂的哈希链
func TestHashChain_Broken(t *testing.T) {
	events := makeTestEvents("job_y", 5)
	// 破坏第 3 个事件的 prev_hash
	events[2].PrevHash = "invalid_hash"

	if err := ValidateChain(events); err == nil {
		t.Error("expected hash chain validation to fail")
	}
}

// === Helper functions ===

// tamperZipFile 篡改 ZIP 中的指定文件
func tamperZipFile(zipBytes []byte, filename string, mutate func([]byte) []byte) []byte {
	zipReader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return zipBytes
	}

	// 读取所有文件
	files := make(map[string][]byte)
	for _, f := range zipReader.File {
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data := bytes.NewBuffer(nil)
		_, _ = data.ReadFrom(rc)
		rc.Close()

		if f.Name == filename {
			// 应用篡改
			files[f.Name] = mutate(data.Bytes())
		} else {
			files[f.Name] = data.Bytes()
		}
	}

	// 重新打包
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	for name, content := range files {
		fw, err := zw.Create(name)
		if err != nil {
			continue
		}
		fw.Write(content)
	}
	zw.Close()

	return buf.Bytes()
}

// deleteNDJSONLine 删除 NDJSON 文件中的指定行
func deleteNDJSONLine(b []byte, lineIdx int) []byte {
	lines := bytes.Split(b, []byte("\n"))
	if lineIdx < 0 || lineIdx >= len(lines) {
		return b
	}

	// 删除指定行
	newLines := append(lines[:lineIdx], lines[lineIdx+1:]...)
	return bytes.Join(newLines, []byte("\n"))
}
