# M4 Signature Guide - 数字签名机制

## 概述

Aetheris 3.0-M4 引入 Ed25519 数字签名，为证据包提供不可否认性，确保证据包由可信组织签名且未被篡改。

---

## 核心概念

### 数字签名

使用 Ed25519 算法：
- **高性能**: 签名/验证速度快
- **安全**: 256-bit 安全级别
- **紧凑**: 签名只有 64 bytes
- **确定性**: 相同数据相同签名

### 签名格式

```
ed25519:<key_id>:<signature_base64>
```

示例：
```
ed25519:org_primary_key:SGVsbG8gV29ybGQhIFRoaXMgaXMgYSBzaWdu...
```

---

## CLI 使用

### 生成密钥对

```bash
aetheris keygen --id org_primary_key

# 输出
✓ Key pair generated
  Key ID: org_primary_key
  Public Key: ed25519:AAAAC3NzaC1lZDI1NTE5...
  Private Key: (stored securely)
```

### 签名证据包

```bash
aetheris sign evidence.zip --key org_primary_key

# 输出
✓ Evidence package signed
  Signature: ed25519:org_primary_key:SGVsbG8...
  Signed package: evidence-signed.zip
```

### 验证签名

```bash
aetheris verify-signature evidence-signed.zip --public-key <public_key>

# 输出
✓ Signature valid
  Signed by: org_primary_key
  Timestamp: 2026-02-12 10:30:00 UTC
```

---

## API 使用

### 导出时签名

```bash
POST /api/jobs/:id/export?sign=true&key_id=org_primary_key
```

响应包含签名：
```json
{
  "download_url": "/api/download/evidence.zip",
  "signature": "ed25519:org_primary_key:...",
  "signed_at": "2026-02-12T10:30:00Z"
}
```

---

## 密钥管理

### 存储方式

**开发环境**: 文件存储
```bash
~/.aetheris/keys/
├── org_primary_key.pub
└── org_primary_key.priv (加密)
```

**生产环境**: HashiCorp Vault
```bash
vault kv put secret/aetheris/keys/org_primary_key \
  private_key=<base64_encoded>
```

### 密钥轮换

```bash
# 1. 生成新密钥
aetheris keygen --id org_key_2026

# 2. 更新配置
vi configs/api.yaml
# signature.key_id: "org_key_2026"

# 3. 旧密钥标记为 retired（保留用于验证历史签名）
```

---

## 安全最佳实践

1. **私钥保护**:
   - 永不记录到日志
   - 永不包含在证据包中
   - 使用 HSM/KMS（生产环境）

2. **密钥轮换**:
   - 每年轮换一次
   - 保留旧公钥（验证历史）
   - 逐步淘汰过期密钥

3. **访问控制**:
   - 只有 Admin 角色可签名
   - 签名操作记录审计日志
   - 多重签名（关键操作）

---

## 故障排查

### 问题: 签名验证失败

**原因**: 公钥不匹配或证据包被篡改

**解决**:
1. 检查公钥是否正确
2. 验证证据包完整性
3. 查看审计日志

### 问题: 私钥不可用

**原因**: Vault 连接失败或密钥被删除

**解决**:
1. 检查 Vault 连接
2. 恢复密钥备份
3. 生成新密钥（标记为新 key_id）

---

## 下一步

- 查看 `docs/m4-distributed-ledger-guide.md` 了解跨组织验证
- 查看 `docs/aetheris-3.0-complete.md` 了解完整能力
