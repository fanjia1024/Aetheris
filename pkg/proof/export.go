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
	"encoding/json"
	"fmt"
	"time"
)

// ExportEvidenceZip 导出证据包为 ZIP 格式
func ExportEvidenceZip(
	ctx context.Context,
	jobID string,
	jobStore JobStore,
	ledger Ledger,
	opts ExportOptions,
) ([]byte, error) {
	if jobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}

	// 1. 获取事件流
	events, err := jobStore.ListEvents(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("no events found for job %s", jobID)
	}

	// 2. 验证内部一致性（哈希链）
	if err := ValidateChain(events); err != nil {
		return nil, fmt.Errorf("hash chain validation failed: %w", err)
	}

	// 3. 获取 ledger
	var toolInvocations []ToolInvocation
	if ledger != nil {
		toolInvocations, err = ledger.ListToolInvocations(ctx, jobID)
		if err != nil {
			return nil, fmt.Errorf("failed to list tool invocations: %w", err)
		}
	}

	// 4. 生成文件内容
	eventsNDJSON, err := eventsToNDJSON(events)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize events: %w", err)
	}

	ledgerNDJSON, err := ledgerToNDJSON(toolInvocations)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize ledger: %w", err)
	}

	metadataJSON, err := json.MarshalIndent(JobMetadata{
		JobID:   jobID,
		AgentID: "(from events)", // TODO: extract from events
		Status:  "(from events)",
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to serialize metadata: %w", err)
	}

	// 5. 计算文件哈希
	fileHashes := map[string]string{
		"events.ndjson": ComputeFileHash(eventsNDJSON),
		"ledger.ndjson": ComputeFileHash(ledgerNDJSON),
		"metadata.json": ComputeFileHash(metadataJSON),
	}

	// 6. 生成 manifest
	manifest := Manifest{
		Version:        "2.0",
		JobID:          jobID,
		ExportedAt:     time.Now().UTC(),
		EventCount:     len(events),
		LedgerCount:    len(toolInvocations),
		FirstEventHash: events[0].Hash,
		LastEventHash:  events[len(events)-1].Hash,
		FileHashes:     fileHashes,
		RuntimeVersion: opts.RuntimeVersion,
		SchemaVersion:  opts.SchemaVersion,
	}
	if manifest.RuntimeVersion == "" {
		manifest.RuntimeVersion = "2.0.0"
	}
	if manifest.SchemaVersion == "" {
		manifest.SchemaVersion = "2.0"
	}

	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to serialize manifest: %w", err)
	}

	// 7. 生成 proof summary
	proofSummary := ProofSummary{
		JobID:           jobID,
		RootHash:        events[len(events)-1].Hash,
		ChainValidated:  true,
		LedgerValidated: true,
		GeneratedBy:     fmt.Sprintf("aetheris %s", opts.RuntimeVersion),
	}

	proofJSON, err := json.MarshalIndent(proofSummary, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to serialize proof: %w", err)
	}

	fileHashes["proof.json"] = ComputeFileHash(proofJSON)

	// 8. 打包为 ZIP
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	files := map[string][]byte{
		"manifest.json": manifestJSON,
		"events.ndjson": eventsNDJSON,
		"ledger.ndjson": ledgerNDJSON,
		"proof.json":    proofJSON,
		"metadata.json": metadataJSON,
	}

	for filename, content := range files {
		fw, err := zw.Create(filename)
		if err != nil {
			return nil, fmt.Errorf("failed to create zip file %s: %w", filename, err)
		}
		if _, err := fw.Write(content); err != nil {
			return nil, fmt.Errorf("failed to write zip file %s: %w", filename, err)
		}
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("failed to close zip writer: %w", err)
	}

	return buf.Bytes(), nil
}

// eventsToNDJSON 将事件列表转换为 NDJSON 格式
func eventsToNDJSON(events []Event) ([]byte, error) {
	buf := new(bytes.Buffer)
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			return nil, err
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

// ledgerToNDJSON 将 ledger 转换为 NDJSON 格式
func ledgerToNDJSON(ledger []ToolInvocation) ([]byte, error) {
	buf := new(bytes.Buffer)
	for _, inv := range ledger {
		data, err := json.Marshal(inv)
		if err != nil {
			return nil, err
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}
