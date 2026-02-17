# Evidence Package 使用指南（审计人员版）

## 什么是 Evidence Package？

Evidence Package（证据包）是一个 ZIP 文件，包含 AI Agent 执行过程的**完整审计记录**。

可以理解为：
- **飞机黑匣子**：记录完整执行过程
- **银行对账单**：可独立验证
- **法庭证据**：可作为法律证据使用

---

## 如何获取证据包？

### 方式 1: 通过 API（推荐）

```bash
curl -X POST http://api/jobs/job_123/export \
  -H "Authorization: Bearer <token>" \
  --output evidence.zip
```

### 方式 2: 通过 CLI 工具

```bash
aetheris export job_123 --output evidence.zip
```

### 方式 3: 通过 Web UI

1. 访问：`http://api/jobs/job_123/trace/page`
2. 点击 "Export Evidence" 按钮
3. 下载 ZIP 文件

---

## 证据包包含什么？

打开 ZIP 文件，你会看到：

### 1. manifest.json（清单）

- 文件列表和哈希值
- 导出时间和版本
- 事件数量统计

**作用**: 验证文件完整性

### 2. events.ndjson（事件流）

- AI 执行的每一步操作
- 时间戳精确到纳秒
- 每行一个 JSON 记录

**作用**: 完整时间线，回答"发生了什么"

### 3. ledger.ndjson（工具调用账本）

- 所有外部操作记录（支付、发邮件、创建工单等）
- 标记是否已提交（committed=true 表示外部世界已改变）

**作用**: 证明副作用只执行了一次

### 4. proof.json（证明摘要）

- Hash chain 根哈希
- 验证状态
- 生成信息

**作用**: 快速检查完整性

### 5. metadata.json（元信息）

- Job 基本信息
- Agent ID
- 创建和完成时间

**作用**: 上下文信息

---

## 如何验证证据包？

### 步骤 1: 下载验证工具

```bash
# 下载 aetheris CLI
curl -O https://releases.aetheris.dev/cli/aetheris
chmod +x aetheris
```

### 步骤 2: 验证证据包

```bash
./aetheris verify evidence.zip
```

### 步骤 3: 查看结果

**成功示例**：
```
Verifying evidence package: evidence.zip

=== Verification Results ===
✓ ZIP file readable
  Size: 125,678 bytes
✓ Manifest valid
✓ Hash chain: OK
✓ Ledger consistency: OK
✓ File integrity: OK

Evidence package verification PASSED
```

**失败示例**：
```
Verifying evidence package: evidence.zip

=== Verification Results ===
✗ Verification FAILED
  - Hash chain broken at event 42
  - File hash mismatch for events.ndjson

DO NOT TRUST this evidence package.
It may have been tampered with.
```

---

## 如何解读证据包？

### 查看事件流

```bash
unzip -p evidence.zip events.ndjson | head -10
```

每行是一个 JSON 对象，关键字段：
- `type`: 事件类型（plan_generated, tool_invocation_finished 等）
- `created_at`: 时间戳
- `payload`: 具体数据

### 查看工具调用

```bash
unzip -p evidence.zip ledger.ndjson
```

关注：
- `tool_name`: 做了什么操作（stripe.charge, sendgrid.send 等）
- `committed`: 是否真正执行（true=已执行）
- `result`: 操作结果

### 查看时间线

使用在线工具（或请技术团队协助）：
```bash
aetheris trace job_123 --from-zip evidence.zip
```

---

## 常见审计场景

### 场景 1: 审查支付操作

**问题**: AI 是否批准了这笔支付？依据是什么？

**操作**:
1. 在 `ledger.ndjson` 中搜索 `stripe.charge`
2. 查看 `result` 确认金额和状态
3. 在 `events.ndjson` 中找到对应的 `tool_invocation_finished` 事件
4. 查看前后事件，了解决策依据

### 场景 2: 验证是否重复执行

**问题**: 某个操作是否被执行了多次？

**操作**:
1. 在 `ledger.ndjson` 中搜索工具名称
2. 统计 `committed=true` 的记录数
3. 验证 `idempotency_key` 不重复

### 场景 3: 确认人工审批

**问题**: 是否有人类审批？

**操作**:
1. 在 `events.ndjson` 中搜索 `human_approval_given`
2. 查看 payload 中的审批人和时间
3. 验证审批后才执行了关键操作

---

## 证据包的法律效力

### 可作为证据的原因

1. **完整性**: 包含完整执行记录
2. **不可篡改**: Hash chain 保证未被修改
3. **可独立验证**: 任何人可验证
4. **时间戳**: 精确到纳秒
5. **因果关系**: 可追溯决策依据

### 提交给法庭/监管机构

提供：
1. Evidence.zip 文件
2. 验证报告（aetheris verify 输出）
3. 解读说明（本文档）
4. 技术联系人（如需专家证言）

---

## 常见问题

**Q: 证据包可以被修改吗？**  
A: 不可以。任何修改都会导致 hash chain 断裂，验证失败。

**Q: 证据包包含敏感信息吗？**  
A: 可能包含。建议在非公开场合处理，或要求技术团队导出时脱敏。

**Q: 证据包可以验证多少次？**  
A: 无限次。验证是纯计算，不依赖外部系统。

**Q: 如果验证失败怎么办？**  
A: 不要信任该证据包，联系技术团队或法务部门。

**Q: 证据包的有效期？**  
A: 永久有效。Hash chain 不会过期。

---

## 技术支持

如有疑问，请联系：
- 技术团队
- 法务部门
- Aetheris 支持：support@aetheris.dev

---

**面向**: 审计人员、合规团队、法务人员  
**技术要求**: 无（只需会用命令行工具）  
**可信程度**: 高（基于密码学证明）
