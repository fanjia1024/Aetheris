# Go + eino 全量重构架构设计（RAG / Agent / Workflow）

## 定位

Aetheris 是 **Agent Workflow Runtime**（类比 Temporal 之于工作流）：核心是任务编排、事件溯源、恢复与可观测，而非单一 AI 应用。RAG/检索/生成以 **Pipeline 或工具** 形式接入，是默认可选能力之一；通过配置或插件注册启用，并非 Runtime 唯一内置场景。

---

## 1. 设计目标

本架构基于 **Go 语言**，以 **eino** 作为唯一的核心 Workflow / Agent Runtime，重构现有离线索引与在线检索系统，目标包括：

- 统一流程编排、Agent 调度、模型调用
- 支持复杂 RAG / 多阶段 Pipeline / DAG Workflow
- 提升并发能力、可观测性与跨 Pipeline 协同能力
- 面向 2025–2026 的 Agent-native 架构演进

---

## 2. 总体架构分层

系统采用 **严格分层 + 单一调度核心** 的设计：

```
┌──────────────────────────────────────────────┐
│            API / Interface Layer              │
├──────────────────────────────────────────────┤
│        Orchestration & Agent Runtime          │
│                 (eino)                        │
├──────────────────────────────────────────────┤
│        Domain Pipelines (Go Native)           │
├──────────────────────────────────────────────┤
│        Model Abstraction Layer                │
├──────────────────────────────────────────────┤
│        Storage & Infrastructure               │
└──────────────────────────────────────────────┘
```

---

## 3. API / Interface Layer

### 职责

- 对外提供统一入口
- 不承载业务逻辑
- 不感知 Pipeline / 模型细节

### 组成

- HTTP / REST API
- gRPC（内部调用）
- CLI / Admin API（可选）

```
[ File Upload ]
[ Query / QA API ]
[ Knowledge Base Management ]
```

---

## 4. Orchestration & Agent Runtime（核心）

### 🔴 系统唯一核心：eino

eino 是整个系统的 **中枢神经系统**，负责：

- Workflow / DAG 定义与执行
- Agent 调度
- Node 生命周期管理
- 上下文（Context / State）传递
- 并发与异步执行

### Runtime 结构

```
eino Runtime
├─ Workflow / Graph Engine
├─ Agent Executor
├─ Node Scheduler
├─ Context & State Manager
├─ Retry / Fallback / Timeout
└─ Concurrency Runtime (Go)
```

> ⚠️ 所有 Pipeline **只能被 eino 调度**
> ⚠️ 不允许 Pipeline 之间直接互相调用

### 4.1 Agent Runtime 与任务执行

以 **Agent 为第一公民** 的请求路径：

1. 用户发消息 → API 创建 **Job**（双写：事件流 `jobstore.Append(JobCreated)` + 状态型 `job.JobStore.Create`）。
2. **Scheduler** 从状态型 JobStore 拉取 Pending Job → 调用 `Runner.RunForJob`。
3. **RunForJob**：若 `Job.Cursor` 存在则从 Checkpoint 恢复；否则 PlanGoal 产出 TaskGraph → Compiler 编译为 DAG → **Steppable** 逐节点执行；每节点执行后落盘 Checkpoint、更新 Session.LastCheckpoint 与 Job.Cursor；恢复时从下一节点继续。
4. Pipeline（如 RAG、Ingest）可作为 TaskGraph 中的 **workflow 节点** 被规划器选用。

**事件化 JobStore**（`internal/runtime/jobstore`）：

- 任务以**事件流**形态存储：ListEvents（带 version）、版本化 Append（乐观并发）、Claim/Heartbeat（租约）、Watch（订阅）。
- 与 eino 的关系：eino **仅作为 DAG 执行内核** 被 `internal/agent/runtime/executor` 调用，不直接面对“创建任务”；任务创建与调度由 Agent Runtime 与 JobStore 负责。

**执行保证契约**：步至少/至多执行一次、Signal 交付、Replay 确定性、崩溃后不重复副作用等正式语义见 [execution-guarantees.md](execution-guarantees.md)。

---

## 5. Domain Pipelines（Go Native）

所有 Pipeline 均为 **Go 原生实现**，仅关注“业务步骤”，不关心执行顺序。

### 5.1 Ingest Pipeline（离线 / 批量）

```
DocumentLoader
 → DocumentParser
 → Splitter Engine
 → Embedding Pipeline
 → Index Builder
```

用途：

- 文档入库
- 索引构建
- 向量化

---

### 5.2 Query Pipeline（在线）

```
Query Input
 → Retriever
 → Reranker
 → Generator
 → Response
```

用途：

- RAG 检索
- 实时问答
- 多轮上下文

---

### 5.3 Specialized Pipelines

- JSONL Pipeline
- HIVE Pipeline
- 长文本 Pipeline
- 流式数据 Pipeline

---

## 6. Splitter Engine（统一抽象）

所有切片逻辑统一收敛为 **Splitter Engine**：

```
Splitter Engine
├─ Structural Splitter   (文档 / 段落)
├─ Semantic Splitter     (语义)
└─ Token-based Splitter  (长度 / Token)
```

- 作为 Pipeline 的可插拔节点
- 不独立运行，不感知 Workflow

---

## 7. Model Abstraction Layer

### 目标

- 模型无关
- 支持多厂商、多模态
- 支持运行时切换

### 抽象接口

```
Model Abstraction
├─ LLM Interface
├─ Embedding Interface
└─ Vision Interface
```

### 实现方式

- eino Model Adapter
- Provider Plugins（OpenAI / Claude / 本地模型）

---

## 8. Storage & Infrastructure

存储按 **职责而非技术名** 划分：

```
Storage Layer
├─ Metadata Store        (MySQL / TiDB)
├─ Vector Store          (Milvus / Vearch / ES)
├─ Object Store          (S3 / OSS)
└─ Cache                 (Redis / Local Cache)
```

---

## 9. 典型执行路径

### 9.1 Agent 发消息（推荐）

```
Message
 → API 创建 Job（双写事件流 + 状态型 Job）
 → Scheduler 拉取 Pending Job
 → Runner.RunForJob（Steppable + 节点级 Checkpoint）
 → PlanGoal → TaskGraph → Compiler → 逐节点执行
 → Tools / RAG / LLM（Pipeline 可作为 workflow 节点被规划器选用）
```

### 9.2 离线索引流程

```
Upload
 → eino Workflow
 → Ingest Pipeline
 → Splitter
 → Embedding
 → Vector Store
```

### 9.3 在线查询流程

```
Query
 → eino Workflow
 → Retriever
 → Generator
 → Response
```

---

## 10. 架构原则总结

- **Single Orchestrator**：只有 eino 能调度
- **Pipeline = Node Graph**：Pipeline 是节点集合
- **Model = Capability**：模型是能力，不是流程
- **Go First**：所有核心逻辑 Go 原生实现
- **Agent Ready**：天然支持 Agent / Tool / Memory

---

## 11. 演进方向（2025–2026）

- Agent 自主规划（Planner Agent）
- Tool-Using Agent
- 多 Workflow 协同
- Human-in-the-loop
- 长期记忆 / Memory Graph

---

## 12. 结论

本架构以 **Go + eino** 为核心，构建了一个 **Agent-Native、Workflow-Driven、RAG-Ready** 的系统基础，可支撑复杂知识系统与智能体平台的长期演进。

---

## 13. 使用场景（Use Cases）

Aetheris 适用于需要**可信执行保证**的 Agent 场景。以下场景推荐使用 Aetheris：

### 13.1 审批流（Human-in-the-Loop）

**场景**：法务审批合同、财务审批付款、HR 审批招聘

**需求**：
- Agent 生成文档后等待人工审批（可能 3 天）
- 等待期间系统重启、Worker 崩溃不能丢失状态
- 审批通过后从中断点继续执行

**Aetheris 能力**：
- Wait 节点 + StatusParked：长时间等待不占 Scheduler 资源
- Signal/Message：审批结果通过 POST `/api/jobs/:id/signal` 唤醒
- Event Sourcing：等待期间崩溃可从事件流恢复

**典型流程**：
```
Plan → 生成合同 → Wait(correlation_key="approval-123") → (人工审批 3 天) → Signal(approval-123, approved=true) → 发送合同
```

---

### 13.2 长任务（Long-Running Tasks）

**场景**：数据处理（1 小时）、报告生成（30 分钟）、批量导入（2 小时）

**需求**：
- 任务运行时间 > 1 分钟，Worker 可能崩溃
- 崩溃后从中断点恢复，不重新执行已完成步骤
- 进度可追踪、可审计

**Aetheris 能力**：
- Checkpoint + Replay：每步完成后写入 Checkpoint，崩溃后从最近 Checkpoint 恢复
- Event Stream：完整执行历史，可展示进度（已完成 3/5 步）
- At-most-once：已完成步骤 replay 时不重新执行

**典型流程**：
```
Plan → 拉取数据(10 min) → Checkpoint → 清洗数据(20 min) → Checkpoint → 生成报告(30 min) → Checkpoint → 发送报告
```

---

### 13.3 外部集成（External Side Effects）

**场景**：调用 Stripe 扣款、Twilio 发短信、Salesforce 创建 Lead、发送邮件

**需求**：
- 外部 API 调用必须 **at-most-once**（不能重复扣款/发短信）
- 崩溃重试时不能重复执行副作用
- 审计需要知道"何时调用了哪个 API、参数是什么、结果是什么"

**Aetheris 能力**：
- Tool Invocation + Ledger：同一 idempotency_key 最多执行一次
- Effect Store + Two-Phase Commit：先记录 effect，崩溃后 catch-up 不重执行
- Step Idempotency Key：`StepIdempotencyKeyForExternal(ctx, jobID, stepID)` 传给下游 API

**典型流程**：
```
Plan → 计算金额 → Tool(stripe.charge, idempotency_key="job:step:attempt") → Tool(email.send, idempotency_key=...) → 完成
```

---

### 13.4 合规审计（Audit & Compliance）

**场景**：金融系统（交易审计）、医疗系统（处方记录）、政府系统（决策追溯）

**需求**：
- 必须记录"谁、何时、为什么做了什么决策"
- 决策依据可追溯（使用了哪些数据、调用了哪些 API、LLM 输出是什么）
- Execution Proof Chain：不可篡改的执行证明

**Aetheris 能力**：
- Evidence Graph：记录 RAG doc IDs、tool invocation IDs、LLM model/temperature
- Decision Snapshot：Planner 级决策（为什么生成这个 Plan）
- Reasoning Snapshot：Step 级决策（goal、state_before、state_after、evidence）
- Event Stream + Trace：完整时间线，可回答"为什么 AI 发送了这封邮件"

**典型流程**：
```
PlanGenerated(evidence: goal, memory_keys) → Step1(evidence: rag_doc_ids, llm_decision) → Step2(evidence: tool_invocation_id) → Trace 展示完整证据链
```

---

### 13.5 多步推理（Multi-Step Reasoning）

**场景**：研究助手（搜索 → 阅读 → 总结 → 撰写）、销售流程（线索 → 跟进 → 报价 → 成交）

**需求**：
- 计划包含多个步骤，每步依赖上一步结果
- 中间步骤失败需重试，已完成步骤不重新执行
- 可暂停、恢复、修改计划

**Aetheris 能力**：
- TaskGraph：DAG 定义步骤依赖
- State Checkpoint：每步 state_before → state_after，下一步读取 state_after
- Replay：失败步骤重试时，前置步骤从事件流注入不重执行

**典型流程**：
```
Plan(TaskGraph: A → B → C) → A 成功 → B 失败 → Retry B（A 从 Checkpoint 注入）→ B 成功 → C 成功
```

---

### 13.6 反例：不适合 Aetheris 的场景

| 场景 | 为什么不适合 | 推荐方案 |
|------|-------------|----------|
| 无状态聊天机器人 | 单次请求/响应，无需持久化 | LangChain + stateless API |
| 原型/Demo Agent | 崩溃可接受，无审计需求 | LangGraph + 内存存储 |
| 纯内存任务（<1 分钟） | 执行时间短，崩溃风险低 | 直接调用 LLM API |
| 无外部副作用 | 不调用 API/数据库，replay 无意义 | 纯函数 + 缓存 |

---

## 参考

- [execution-guarantees.md](execution-guarantees.md) — 运行时保证
- [effect-system.md](effect-system.md) — Effect 类型与 Replay 协议
- [scheduler-correctness.md](scheduler-correctness.md) — 调度器正确性
- [runtime-contract.md](runtime-contract.md) — 运行时契约
- [step-contract.md](step-contract.md) — Step 编写契约
