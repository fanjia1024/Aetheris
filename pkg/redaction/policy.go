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

// RedactionPolicy 脱敏策略（2.0-M2）
type RedactionPolicy struct {
	EventRules  map[string][]FieldMask // event_type -> field masks
	GlobalRules []FieldMask            // 全局规则（应用于所有事件）
}

// FieldMask 字段掩码配置
type FieldMask struct {
	FieldPath string        // JSON path (e.g., "payload.prompt", "result.email")
	Mode      RedactionMode // 脱敏模式
	Salt      string        // Hash 模式的 salt（可选）
}

// RedactionMode 脱敏模式
type RedactionMode string

const (
	RedactionModeRedact  RedactionMode = "redact"  // 替换为 "***"
	RedactionModeHash    RedactionMode = "hash"    // 替换为 SHA256 hash
	RedactionModeEncrypt RedactionMode = "encrypt" // 加密（需要 key）
	RedactionModeRemove  RedactionMode = "remove"  // 完全移除字段
)

// PolicyConfig 脱敏策略配置（YAML）
type PolicyConfig struct {
	Enable   bool                `yaml:"enable"`
	Policies []EventPolicyConfig `yaml:"policies"`
}

// EventPolicyConfig 单个事件类型的脱敏策略
type EventPolicyConfig struct {
	EventType string            `yaml:"event_type"`
	Fields    []FieldMaskConfig `yaml:"fields"`
}

// FieldMaskConfig 字段掩码配置（YAML）
type FieldMaskConfig struct {
	Path string        `yaml:"path"`
	Mode RedactionMode `yaml:"mode"`
	Salt string        `yaml:"salt"`
}

// LoadPolicyFromConfig 从配置加载脱敏策略
func LoadPolicyFromConfig(config PolicyConfig) *RedactionPolicy {
	if !config.Enable {
		return nil
	}

	policy := &RedactionPolicy{
		EventRules:  make(map[string][]FieldMask),
		GlobalRules: []FieldMask{},
	}

	for _, eventPolicy := range config.Policies {
		masks := []FieldMask{}
		for _, fieldConfig := range eventPolicy.Fields {
			masks = append(masks, FieldMask{
				FieldPath: fieldConfig.Path,
				Mode:      fieldConfig.Mode,
				Salt:      fieldConfig.Salt,
			})
		}
		policy.EventRules[eventPolicy.EventType] = masks
	}

	return policy
}
