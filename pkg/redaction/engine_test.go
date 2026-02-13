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

package redaction

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestRedaction_RedactMode 测试 redact 模式
func TestRedaction_RedactMode(t *testing.T) {
	policy := &RedactionPolicy{
		EventRules: map[string][]FieldMask{
			"test_event": {
				{FieldPath: "email", Mode: RedactionModeRedact},
			},
		},
	}

	engine := NewEngine(policy, nil)

	input := []byte(`{"email":"user@example.com","name":"John"}`)
	output, err := engine.RedactData("test_event", input)
	if err != nil {
		t.Fatalf("redaction failed: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(output, &result)

	if result["email"] != "***REDACTED***" {
		t.Errorf("email should be redacted, got: %v", result["email"])
	}
	if result["name"] != "John" {
		t.Error("name should not be redacted")
	}
}

// TestRedaction_HashMode 测试 hash 模式
func TestRedaction_HashMode(t *testing.T) {
	policy := &RedactionPolicy{
		EventRules: map[string][]FieldMask{
			"test_event": {
				{FieldPath: "secret", Mode: RedactionModeHash, Salt: "test_salt"},
			},
		},
	}

	engine := NewEngine(policy, nil)

	input := []byte(`{"secret":"sensitive_data","public":"visible"}`)
	output, err := engine.RedactData("test_event", input)
	if err != nil {
		t.Fatalf("redaction failed: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(output, &result)

	hashValue, ok := result["secret"].(string)
	if !ok || !strings.HasPrefix(hashValue, "hash:") {
		t.Errorf("secret should be hashed, got: %v", result["secret"])
	}

	if result["public"] != "visible" {
		t.Error("public field should not be redacted")
	}
}

// TestRedaction_RemoveMode 测试 remove 模式
func TestRedaction_RemoveMode(t *testing.T) {
	policy := &RedactionPolicy{
		EventRules: map[string][]FieldMask{
			"test_event": {
				{FieldPath: "internal", Mode: RedactionModeRemove},
			},
		},
	}

	engine := NewEngine(policy, nil)

	input := []byte(`{"internal":"secret","external":"visible"}`)
	output, err := engine.RedactData("test_event", input)
	if err != nil {
		t.Fatalf("redaction failed: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(output, &result)

	if _, exists := result["internal"]; exists {
		t.Error("internal field should be removed")
	}
	if result["external"] != "visible" {
		t.Error("external field should remain")
	}
}

// TestRedaction_NestedField 测试嵌套字段脱敏
func TestRedaction_NestedField(t *testing.T) {
	policy := &RedactionPolicy{
		EventRules: map[string][]FieldMask{
			"test_event": {
				{FieldPath: "payload.email", Mode: RedactionModeRedact},
			},
		},
	}

	engine := NewEngine(policy, nil)

	input := []byte(`{"payload":{"email":"user@example.com","name":"John"}}`)
	output, err := engine.RedactData("test_event", input)
	if err != nil {
		t.Fatalf("redaction failed: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(output, &result)

	payload := result["payload"].(map[string]interface{})
	if payload["email"] != "***REDACTED***" {
		t.Errorf("nested email should be redacted, got: %v", payload["email"])
	}
	if payload["name"] != "John" {
		t.Error("nested name should not be redacted")
	}
}
