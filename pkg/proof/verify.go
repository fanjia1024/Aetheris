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
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// VerifyEvidenceZip 验证证据包 ZIP
func VerifyEvidenceZip(zipBytes []byte) VerifyResult {
	result := VerifyResult{
		OK:     true,
		Errors: []string{},
	}

	// 1. 解压 ZIP
	zipReader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		result.OK = false
		result.Errors = append(result.Errors, fmt.Sprintf("failed to read zip: %v", err))
		return result
	}

	// 2. 读取所有文件
	files := make(map[string][]byte)
	for _, f := range zipReader.File {
		rc, err := f.Open()
		if err != nil {
			result.OK = false
			result.Errors = append(result.Errors, fmt.Sprintf("failed to open %s: %v", f.Name, err))
			continue
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			result.OK = false
			result.Errors = append(result.Errors, fmt.Sprintf("failed to read %s: %v", f.Name, err))
			continue
		}
		files[f.Name] = data
	}

	// 3. 读取并验证 manifest
	manifestData, ok := files["manifest.json"]
	if !ok {
		result.OK = false
		result.Errors = append(result.Errors, "manifest.json not found")
		return result
	}

	var manifest Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		result.OK = false
		result.Errors = append(result.Errors, fmt.Sprintf("failed to parse manifest: %v", err))
		return result
	}
	result.ManifestValid = true

	// 4. 验证文件哈希
	for filename, expectedHash := range manifest.FileHashes {
		if fileData, ok := files[filename]; ok {
			actualHash := ComputeFileHash(fileData)
			if actualHash != expectedHash {
				result.OK = false
				result.Errors = append(result.Errors, fmt.Sprintf("file hash mismatch for %s: expected %s, got %s", filename, expectedHash, actualHash))
			}
		} else {
			result.OK = false
			result.Errors = append(result.Errors, fmt.Sprintf("file %s declared in manifest but not found in zip", filename))
		}
	}

	// 5. 解析并验证事件流
	eventsData, ok := files["events.ndjson"]
	if !ok {
		result.OK = false
		result.Errors = append(result.Errors, "events.ndjson not found")
		return result
	}

	events, err := parseEventsNDJSON(eventsData)
	if err != nil {
		result.OK = false
		result.Errors = append(result.Errors, fmt.Sprintf("failed to parse events: %v", err))
		return result
	}
	result.Events = events

	// 6. 验证哈希链
	if err := ValidateChain(events); err != nil {
		result.OK = false
		result.HashChainValid = false
		result.Errors = append(result.Errors, fmt.Sprintf("hash chain invalid: %v", err))
	} else {
		result.HashChainValid = true
		result.EventsValid = true
	}

	// 7. 解析并验证 ledger
	ledgerData, ok := files["ledger.ndjson"]
	if ok {
		ledger, err := parseLedgerNDJSON(ledgerData)
		if err != nil {
			result.OK = false
			result.Errors = append(result.Errors, fmt.Sprintf("failed to parse ledger: %v", err))
		} else {
			// 验证 ledger 与 events 一致性
			if err := ValidateLedgerConsistency(events, ledger); err != nil {
				result.OK = false
				result.LedgerValid = false
				result.Errors = append(result.Errors, fmt.Sprintf("ledger consistency check failed: %v", err))
			} else {
				result.LedgerValid = true
			}
		}
	}

	// 8. 验证 proof summary
	proofData, ok := files["proof.json"]
	if ok {
		var proofSummary ProofSummary
		if err := json.Unmarshal(proofData, &proofSummary); err != nil {
			result.OK = false
			result.Errors = append(result.Errors, fmt.Sprintf("failed to parse proof: %v", err))
		} else {
			// 验证 root_hash == 最后一个事件的 hash
			if len(events) > 0 && proofSummary.RootHash != events[len(events)-1].Hash {
				result.OK = false
				result.Errors = append(result.Errors, fmt.Sprintf("proof root_hash mismatch: expected %s, got %s", events[len(events)-1].Hash, proofSummary.RootHash))
			}
		}
	}

	return result
}

// ValidateLedgerConsistency 验证 ledger 与 events 对齐
func ValidateLedgerConsistency(events []Event, ledger []ToolInvocation) error {
	// 从 events 中提取所有 tool_invocation_started 和 tool_invocation_finished
	startedMap := make(map[string]bool)                    // idempotency_key -> started
	finishedMap := make(map[string]map[string]interface{}) // idempotency_key -> event payload

	for _, event := range events {
		if event.Type == "tool_invocation_started" {
			var payload map[string]interface{}
			if err := json.Unmarshal([]byte(event.Payload), &payload); err == nil {
				if key, ok := payload["idempotency_key"].(string); ok && key != "" {
					startedMap[key] = true
				}
			}
		} else if event.Type == "tool_invocation_finished" {
			var payload map[string]interface{}
			if err := json.Unmarshal([]byte(event.Payload), &payload); err == nil {
				if key, ok := payload["idempotency_key"].(string); ok && key != "" {
					finishedMap[key] = payload
				}
			}
		}
	}

	// 构建 ledger map
	ledgerMap := make(map[string]ToolInvocation)
	for _, inv := range ledger {
		ledgerMap[inv.IdempotencyKey] = inv
	}

	// 验证：每个 finished 事件必须在 ledger 中有对应记录
	for idempotencyKey, eventPayload := range finishedMap {
		ledgerInv, ok := ledgerMap[idempotencyKey]
		if !ok {
			return fmt.Errorf("tool invocation %s found in events but missing in ledger", idempotencyKey)
		}

		// 验证 tool_name 一致
		if toolName, ok := eventPayload["tool_name"].(string); ok {
			if toolName != "" && toolName != ledgerInv.ToolName {
				return fmt.Errorf("tool_name mismatch for %s: event=%s, ledger=%s", idempotencyKey, toolName, ledgerInv.ToolName)
			}
		}

		// 验证 outcome/status 一致
		if outcome, ok := eventPayload["outcome"].(string); ok {
			if outcome == "success" && !ledgerInv.Committed {
				return fmt.Errorf("event shows success but ledger not committed for %s", idempotencyKey)
			}
		}
	}

	// 验证：ledger 中的每个 committed 记录必须在 events 中有 finished 事件
	for _, inv := range ledger {
		if inv.Committed {
			if _, ok := finishedMap[inv.IdempotencyKey]; !ok {
				return fmt.Errorf("ledger shows committed but no finished event for %s", inv.IdempotencyKey)
			}
		}
	}

	return nil
}

// parseEventsNDJSON 解析 NDJSON 格式的事件流
func parseEventsNDJSON(data []byte) ([]Event, error) {
	var events []Event
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, fmt.Errorf("failed to parse event line %d: %w", i+1, err)
		}
		events = append(events, event)
	}
	return events, nil
}

// parseLedgerNDJSON 解析 NDJSON 格式的 ledger
func parseLedgerNDJSON(data []byte) ([]ToolInvocation, error) {
	var ledger []ToolInvocation
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var inv ToolInvocation
		if err := json.Unmarshal([]byte(line), &inv); err != nil {
			return nil, fmt.Errorf("failed to parse ledger line %d: %w", i+1, err)
		}
		ledger = append(ledger, inv)
	}
	return ledger, nil
}
