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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// ComputeEventHash 计算单个事件的哈希
// Hash = SHA256(JobID|Type|Payload|Timestamp|PrevHash)
func ComputeEventHash(e Event) string {
	h := sha256.New()
	h.Write([]byte(e.JobID))
	h.Write([]byte("|"))
	h.Write([]byte(e.Type))
	h.Write([]byte("|"))
	h.Write([]byte(e.Payload))
	h.Write([]byte("|"))
	h.Write([]byte(e.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"))) // RFC3339Nano
	h.Write([]byte("|"))
	h.Write([]byte(e.PrevHash))
	return hex.EncodeToString(h.Sum(nil))
}

// ValidateChain 验证完整哈希链
func ValidateChain(events []Event) error {
	if len(events) == 0 {
		return nil
	}

	// 第一个事件的 PrevHash 应该为空
	if events[0].PrevHash != "" {
		return fmt.Errorf("first event prev_hash should be empty, got: %s", events[0].PrevHash)
	}

	// 验证第一个事件的 hash
	expectedHash := ComputeEventHash(events[0])
	if expectedHash != events[0].Hash {
		return fmt.Errorf("event 0 hash mismatch: expected %s, got %s", expectedHash, events[0].Hash)
	}

	// 验证后续事件的哈希链
	for i := 1; i < len(events); i++ {
		// 检查 prev_hash 是否等于前一个事件的 hash
		if events[i].PrevHash != events[i-1].Hash {
			return fmt.Errorf("hash chain broken at event %d: prev_hash=%s, expected=%s",
				i, events[i].PrevHash, events[i-1].Hash)
		}

		// 重新计算 hash 验证
		expectedHash := ComputeEventHash(events[i])
		if expectedHash != events[i].Hash {
			return fmt.Errorf("event %d hash mismatch: expected %s, got %s", i, expectedHash, events[i].Hash)
		}
	}

	return nil
}

// ComputeFileHash 计算文件内容的 SHA256 哈希
func ComputeFileHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
