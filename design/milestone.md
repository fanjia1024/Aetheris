好，那 2.0 我们就把重心收敛到 **“审计合规 / 取证（forensics）”**：目标不是“有 trace”，而是 **事后能回答清楚、能证明、能交付证据包、还能在合规约束下存储/访问/留存**。

下面是我建议你在 2.0 需要继续提高的能力（按落地优先级）——每一项我都给了“要补什么 + 验收标准（能测/能证明）”。

---

## 0) 2.0 合规取证的北极星

你现在 1.0 已经有“Execution Proof Chain / confirmation replay / tool ledger / reasoning snapshot”等叙事。2.0 的北极星是把它变成：

* **可归档的证据包（Evidence Package）**
* **可验证的证明链（Verifiable Proof Chain）**
* **可控的访问与留存（AuthZ + Retention + Redaction）**
* **可查询的取证 API（Forensics API）**
* **可在 UI 上复盘与定位（Trace UI -> Forensics UI）**

---

## 1) 证明链“可验证”而不是“可叙述”

### 你需要补的能力

* **链式哈希 / Merkle 化**：对 event stream（或 step 级别）做链式 hash（prev_hash + event_hash），让“篡改可检测”
* **签名与信任根**：把 proof chain 绑定到运行时身份（worker instance / scheduler / org key）
* **不可变存储策略**：至少做到“逻辑不可变”（append-only + 防更新/防删除），更进一步支持 WORM/对象存储锁定

### 验收标准

* 给定一个 job 的证据包，离线工具可以验证：

  1. event stream 未被插入/删除/篡改
  2. 每个 tool invocation 的结果确实来自 ledger（而不是重跑伪造）
  3. proof chain 签名可追溯到可信 key/证书

---

## 2) Evidence Graph：把“发生了什么”变成“为什么这么做”

你 README 里已经提到 Evidence Graph（RAG doc IDs、tool invocations、LLM model/version），2.0 要把它产品化。

### 你需要补的能力

* **证据节点标准化**：对“输入证据/中间证据/外部证据/人类审批证据”定义统一 schema
* **因果关联（Causality）固化**：每个 step 的输入来自哪些事件/工具/文档，输出影响了哪些后续 step
* **关键决策点标注**：把“业务关键节点”（比如付款/退款/审批/发信）作为强类型的 evidence marker

### 验收标准

* UI 或 API 能回答并导出：

  * “这封邮件是谁让 AI 发的？在哪一步？用的哪个模型？依据了哪些文档/工具结果？”
  * “某个关键决策（approve/deny）对应的证据集合是什么？是否完整？”

---

## 3) 取证包（Evidence Package）一键导出 + 可离线复核

### 你需要补的能力

* **证据包格式**：建议做成一个目录/zip（manifest.json + events.ndjson + ledger.ndjson + snapshots + attachments）
* **清单与校验**：manifest 里列出所有文件 hash、proof chain、版本信息、时间范围、导出策略
* **离线验证器**：CLI 提供 `aetheris verify <evidence.zip>`（或者 go test/工具）验证一致性

### 验收标准

* 任何人拿到证据包，在没有你的数据库的情况下也能复核：

  * proof chain ok
  * event/ledger 对齐
  * “重放不重跑副作用”的证据可确认（ledger 命中 + replay记录）

---

## 4) 合规三件套：访问控制、脱敏、留存

这块是很多项目从 1.0 到 2.0 最大的“工程鸿沟”。

### 4.1 多租户与 RBAC（AuthZ）

* 能按 tenant / namespace / job / tool / evidence 类型授权
* 审计“谁看过/导出过”证据（访问本身也应进入审计链）

**验收**：同一套集群中不同 tenant 无法互查；导出行为有审计事件可追溯。

### 4.2 PII/敏感信息脱敏（Redaction）

* 对事件字段、LLM request/response、tool 输入输出做 **字段级策略**
* 支持“原文不可见但可验证”的模式（例如存 hash/摘要，或加密后按权限解密）

**验收**：开启脱敏策略后，证据包中不会出现指定敏感字段，但仍能验证 proof chain 不被破坏。

### 4.3 留存与删除（Retention / Deletion）

* **合规留存**：按 job 类型/tenant 配置 retention
* **可证明删除**：在允许删除的场景下，删除也要有“删除事件”记录（或 tombstone）并解释其合规性

**验收**：不同策略下数据能自动归档/清理；删除不会让系统“假装没发生过”，而是可审计地消失或被封存。

---

## 5) “人类参与”的取证：HITL 证据必须可验证

你 1.0 强调 HITL（StatusParked/Signal）。2.0 要确保人类动作也进入证据链。

### 你需要补的能力

* 对 **approval/override/comment** 等人类输入做强类型事件
* 绑定身份（who/when/why），支持签名（至少服务端签名）
* 对外部系统审批（如工单/邮件）支持“外部证据引用”（URL/ID/hash）

### 验收标准

* 任一恢复/继续执行都能展示：

  * 谁发了 signal
  * signal payload 是否被篡改
  * 它如何影响后续 step

---

## 6) Forensics API：支持“案件式查询”

光有 trace UI 不够，2.0 需要为合规/风控/审计团队提供检索能力。

### 你需要补的能力

* 查询维度：tenant、时间窗、job type、tool、模型版本、关键事件（资金动作/发信/审批）
* 支持“证据链一致性检查”接口（job级 / step级）
* 支持导出作业异步化（大 job 的证据包）

### 验收标准

* 给出一条查询：`所有在 2026-02-01~02-12 内调用过 Stripe tool 且有 approve 事件的 job` 能返回完整列表，并可批量导出证据包。

---

## 7) 观测升级为“审计指标”

你现在 observability 在增强，2.0 要把它变成审计可用的指标与告警：

* proof chain 验证失败率
* replay verify 失败率
* 证据缺失（缺 snapshot / 缺 ledger / 缺 model meta）
* 导出频率与异常（疑似数据外流）
* 脱敏策略命中率（异常下降要告警）

---

# 我建议你把 2.0 拆成 3 个里程碑（最稳）

**M1：可验证证明链 + 证据包导出/离线验证**
（先把“能证明”做出来，最硬核、最对外）

**M2：RBAC + 脱敏 + 留存策略**
（把“能合规地存/看/导”做出来）

**M3：Evidence Graph + Forensics API + UI 取证视图**
（把“能快速办案定位”做出来）

---

如果你愿意，我可以下一步直接把它变成你 repo 可落地的 task list（按目录/模块拆分到具体改动点），比如：

* JobStore / Ledger 要增加哪些字段与索引
* event schema 要加哪些 event type（audit_access、export_created、human_approval、tombstone…）
* CLI 增加哪些命令（export/verify）
* Trace UI 要新增哪些视图（Evidence Graph / Decision Timeline / Access Log）

你现在更想先啃 **“证明链可验证（hash+sign）”**，还是先啃 **“证据包导出与离线验证”**？我建议从后者开始更快出成果。
