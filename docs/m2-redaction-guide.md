# M2 Redaction Guide - 敏感信息脱敏

## 概述

Aetheris 2.0-M2 提供敏感信息脱敏能力，在导出证据包时自动保护 PII（个人身份信息）和敏感数据，同时保持 proof chain 的完整性。

---

## 脱敏模式

### 1. Redact 模式

替换为固定字符串 `***REDACTED***`：

```json
// 原始
{"email": "user@example.com"}

// 脱敏后
{"email": "***REDACTED***"}
```

**适用场景**: 完全不需要原值的字段（如邮箱、手机号）

### 2. Hash 模式

替换为 SHA256 哈希值：

```json
// 原始
{"user_id": "12345"}

// 脱敏后
{"user_id": "hash:a1b2c3d4..."}
```

**适用场景**: 需要验证一致性但不需要原值（如用户 ID、交易 ID）

### 3. Encrypt 模式

使用 AES-256-GCM 加密：

```json
// 原始
{"credit_card": "4111-1111-1111-1111"}

// 脱敏后
{"credit_card": "enc:f4e3d2c1..."}
```

**适用场景**: 有解密权限的用户可恢复原值

### 4. Remove 模式

完全移除字段：

```json
// 原始
{"internal_id": "secret","public":"visible"}

// 脱敏后
{"public":"visible"}
```

**适用场景**: 完全不应出现在证据包中的字段

---

## 配置

### 基本配置

**configs/api.yaml**:
```yaml
redaction:
  enable: true
  policies:
    - event_type: "llm_called"
      fields:
        - path: "payload.prompt"
          mode: "hash"
          salt: "your_random_salt"
        - path: "payload.user_id"
          mode: "redact"
    - event_type: "tool_invocation_finished"
      fields:
        - path: "result.email"
          mode: "redact"
        - path: "result.phone"
          mode: "redact"
        - path: "result.credit_card"
          mode: "encrypt"
```

### 字段路径（Field Path）

使用点号分隔的 JSON 路径：

- `"email"` - 顶级字段
- `"payload.prompt"` - 嵌套字段
- `"result.user.email"` - 深层嵌套

---

## 导出脱敏证据包

### CLI 使用

```bash
# 导出时应用脱敏（需要配置 redaction.enable=true）
aetheris export job_123 --output evidence-redacted.zip

# 验证脱敏后的证据包（hash 链仍然有效）
aetheris verify evidence-redacted.zip
# 输出: ✓ Verification PASSED
```

### API 使用

```bash
# POST /api/jobs/:id/export 自动应用 tenant 的脱敏策略
curl -X POST -H "Authorization: Bearer <token>" \
     http://api/api/jobs/job_123/export \
     --output evidence.zip
```

---

## 验证脱敏证据包

### Hash 链仍然有效

脱敏不破坏 proof chain，因为：
1. 脱敏在导出时应用（不影响原始事件）
2. 导出的事件基于脱敏后的数据计算 hash
3. 验证时使用证据包中的数据（已脱敏）

```bash
$ aetheris verify evidence-redacted.zip
✓ Verification PASSED
  - Events: 100 valid
  - Hash chain: OK
  - Ledger consistency: OK
  - Manifest: OK
```

### Ledger 一致性

脱敏后的 ledger 验证只检查结构，不检查具体值：
- IdempotencyKey 匹配
- ToolName 匹配
- Status 匹配
- 字段值可能被脱敏（不影响一致性）

---

## 自定义脱敏规则

### 按事件类型

```yaml
redaction:
  policies:
    - event_type: "llm_called"
      fields:
        - path: "payload.prompt"
          mode: "hash"
    - event_type: "llm_returned"
      fields:
        - path: "response.content"
          mode: "hash"
```

### 全局规则

对所有事件应用的规则：

```yaml
redaction:
  global_rules:
    - path: "user_id"
      mode: "hash"
    - path: "ip_address"
      mode: "redact"
```

### 带 Salt 的 Hash

防止彩虹表攻击：

```yaml
fields:
  - path: "user_id"
    mode: "hash"
    salt: "your_random_secret_salt_32_chars"
```

---

## 脱敏策略示例

### 场景 1: 合规审计

需要证明执行路径，但不需要看到敏感内容：

```yaml
redaction:
  policies:
    - event_type: "llm_called"
      fields:
        - path: "payload.prompt"
          mode: "hash"  # 可验证一致性
    - event_type: "tool_invocation_finished"
      fields:
        - path: "result"
          mode: "hash"  # 可验证但不可见
```

### 场景 2: 安全取证

需要完全隐藏敏感信息：

```yaml
redaction:
  policies:
    - event_type: "tool_invocation_finished"
      fields:
        - path: "result.password"
          mode: "remove"  # 完全移除
        - path: "result.api_key"
          mode: "remove"
        - path: "result.credit_card"
          mode: "redact"  # 替换为 ***
```

### 场景 3: 可解密模式

特权用户可解密查看：

```yaml
redaction:
  policies:
    - event_type: "llm_called"
      fields:
        - path: "payload.prompt"
          mode: "encrypt"  # 加密存储
```

需要配置加密密钥（环境变量）：
```bash
export REDACTION_ENCRYPT_KEY="32_byte_encryption_key_here"
```

---

## 编程接口

### 在代码中使用脱敏

```go
import "rag-platform/pkg/redaction"

// 创建脱敏引擎
policy := &redaction.RedactionPolicy{
    EventRules: map[string][]redaction.FieldMask{
        "llm_called": {
            {FieldPath: "payload.prompt", Mode: redaction.RedactionModeHash},
        },
    },
}

engine := redaction.NewEngine(policy, nil)

// 脱敏事件数据
eventData := []byte(`{"payload":{"prompt":"sensitive text"}}`)
redactedData, _ := engine.RedactData("llm_called", eventData)
```

---

## 性能影响

- **Hash 模式**: +0.1ms per field
- **Redact 模式**: +0.05ms per field
- **Encrypt 模式**: +1ms per field
- **总体**: 导出时间增加 5-15%

---

## 最佳实践

1. **默认 Hash**: 对可能需要验证的字段使用 hash 模式
2. **Remove 密钥**: 对密码、API key 使用 remove 模式
3. **Encrypt 可恢复**: 对需要解密的字段使用 encrypt 模式
4. **使用 Salt**: Hash 模式加 salt 防止彩虹表
5. **分层策略**: 不同 tenant 可配置不同脱敏策略

---

## 常见问题

**Q: 脱敏会影响 proof chain 验证吗？**  
A: 不会。Hash 基于脱敏后的数据计算，验证时使用证据包中的数据（已脱敏）。

**Q: 可以在导出后解密吗？**  
A: Encrypt 模式支持解密，需要配置解密密钥。其他模式不可逆。

**Q: 如何验证脱敏正确应用？**  
A: 导出后打开 events.ndjson，检查敏感字段是否被替换。

**Q: 原始数据会被修改吗？**  
A: 不会。脱敏只在导出时应用，不影响数据库中的原始事件。

---

## 下一步

- 查看 `docs/m2-retention-guide.md` 了解留存策略
- 查看 `docs/m2-rbac-guide.md` 了解访问控制
