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
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Engine 脱敏引擎（2.0-M2）
type Engine struct {
	policy     *RedactionPolicy
	encryptKey []byte // For encryption mode
}

// NewEngine 创建脱敏引擎
func NewEngine(policy *RedactionPolicy, encryptKey []byte) *Engine {
	return &Engine{
		policy:     policy,
		encryptKey: encryptKey,
	}
}

// RedactData 对 JSON 数据应用脱敏策略
func (e *Engine) RedactData(eventType string, data []byte) ([]byte, error) {
	if e.policy == nil || len(data) == 0 {
		return data, nil
	}

	// 解析 JSON
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return data, err
	}

	// 获取该事件类型的脱敏规则
	rules := e.policy.EventRules[eventType]
	rules = append(rules, e.policy.GlobalRules...)

	// 应用脱敏规则
	for _, rule := range rules {
		e.applyFieldMask(obj, rule)
	}

	// 重新序列化
	return json.Marshal(obj)
}

// applyFieldMask 应用字段掩码
func (e *Engine) applyFieldMask(obj map[string]interface{}, mask FieldMask) {
	// 解析 field path (e.g., "payload.email" -> ["payload", "email"])
	parts := strings.Split(mask.FieldPath, ".")

	// 定位字段
	current := obj
	for i := 0; i < len(parts)-1; i++ {
		if next, ok := current[parts[i]].(map[string]interface{}); ok {
			current = next
		} else {
			return // 字段不存在
		}
	}

	lastKey := parts[len(parts)-1]
	value, exists := current[lastKey]
	if !exists {
		return
	}

	// 应用脱敏
	switch mask.Mode {
	case RedactionModeRedact:
		current[lastKey] = "***REDACTED***"

	case RedactionModeHash:
		strValue := fmt.Sprintf("%v", value)
		hashValue := e.hashValue(strValue, mask.Salt)
		current[lastKey] = hashValue

	case RedactionModeEncrypt:
		strValue := fmt.Sprintf("%v", value)
		encrypted, err := e.encryptValue(strValue)
		if err == nil {
			current[lastKey] = encrypted
		}

	case RedactionModeRemove:
		delete(current, lastKey)
	}
}

// hashValue 计算字段的 SHA256 hash
func (e *Engine) hashValue(value string, salt string) string {
	h := sha256.New()
	h.Write([]byte(value))
	if salt != "" {
		h.Write([]byte(salt))
	}
	return "hash:" + hex.EncodeToString(h.Sum(nil))
}

// encryptValue 加密字段值（AES-256-GCM）
func (e *Engine) encryptValue(value string) (string, error) {
	if len(e.encryptKey) == 0 {
		return "", fmt.Errorf("encryption key not configured")
	}

	block, err := aes.NewCipher(e.encryptKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(value), nil)
	return "enc:" + hex.EncodeToString(ciphertext), nil
}
