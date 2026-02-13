# Aetheris 2.0 — 功能模块与 Roadmap

本文档描述 Aetheris 2.0 的功能模块结构和高阶 Roadmap 时间线。执行语义与 1.0 运行时（Ledger、JobStore、Runner）的对应关系见 [runtime-core-diagrams.md](runtime-core-diagrams.md) 与 [1.0-runtime-semantics.md](1.0-runtime-semantics.md)。

---

## 1. 2.0 功能模块结构图

```mermaid
graph TD
    A[User/API] --> B[Agent Core]
    B --> C[Job Scheduler & Runner]
    B --> D[Planner / DAG Executor]
    B --> E[Event Store / JobStore]
    B --> F[Tool & Workflow Nodes]
    B --> G[RAG / Knowledge Nodes]
    B --> H[Monitoring & Visualization]
    B --> I[Security & Governance]

    subgraph AgentCollaboration [Agent Collaboration]
        B --> J[Multi-Agent Messaging]
        B --> K[Event-Driven Triggers]
    end

    subgraph PerformanceStability [Performance and Stability]
        C --> L[Dynamic Worker Pool]
        C --> M[Task Prioritization and Timeout]
        E --> N[Distributed JobStore]
        D --> O[Async DAG Execution]
    end

    subgraph Extensibility [Extensibility]
        F --> P[Custom Task Plugins]
        G --> Q[Custom Knowledge Sources]
    end

    subgraph Monitoring [Monitoring]
        H --> R[Job / Task Dashboard]
        H --> S[Event Stream Visualization]
        H --> T[Debug / Replay Tools]
    end

    subgraph Security [Security]
        I --> U[RBAC / Access Control]
        I --> V[Audit Logs]
        I --> W[Multi-Tenant Isolation]
    end
```

---

## 2. 2.0 高阶 Roadmap 时间线

| 阶段       | 时间     | 主要目标              | 关键模块                                                                |
| ---------- | -------- | --------------------- | ----------------------------------------------------------------------- |
| **阶段 1** | 1–2 个月 | 核心执行增强、多 Agent 支持 | Job Scheduler & Runner, Planner/DAG Executor, Multi-Agent Messaging     |
| **阶段 2** | 2–3 个月 | RAG 集成 & 扩展节点   | RAG / Knowledge Nodes, Tool & Workflow Nodes, Custom Task Plugins       |
| **阶段 3** | 2 个月   | 监控与可视化          | Monitoring & Visualization, Event Stream, Debug Tools                   |
| **阶段 4** | 1–2 个月 | 安全与治理            | Security & Governance, RBAC, Audit, Multi-Tenant                        |

---

## 3. 2.0 总览整合图（模块 + 依赖 + 阶段）

### 3.1 Roadmap 甘特图

下图展示 2.0 各阶段与时间线。

```mermaid
gantt
    title Aetheris 2.0 Roadmap
    dateFormat  YYYY-MM-DD
    axisFormat  %b
    section Phase 1: 核心增强
    Scheduler & Runner           :done,    s1, 2026-02-15, 30d
    Planner / DAG Executor       :active,  p1, 2026-02-15, 30d
    Multi-Agent Messaging        :         m1, 2026-02-15, 30d
    section Phase 2: RAG & 扩展节点
    RAG / Knowledge Nodes        :         r1, 2026-03-17, 45d
    Tool & Workflow Nodes        :         t1, 2026-03-17, 45d
    Custom Task Plugins          :         c1, 2026-03-17, 45d
    section Phase 3: 可视化与监控
    Job / Task Dashboard         :         j1, 2026-05-01, 30d
    Event Stream Visualization   :         e1, 2026-05-01, 30d
    Debug / Replay Tools         :         d1, 2026-05-01, 30d
    section Phase 4: 安全与治理
    RBAC / Access Control        :         a1, 2026-06-01, 30d
    Audit Logs                   :         a2, 2026-06-01, 30d
    Multi-Tenant Isolation       :         a3, 2026-06-01, 30d
```

### 3.2 模块依赖结构图

下图展示模块间依赖与扩展、监控、安全关系。

```mermaid
graph TD
    U[User / API] --> A[Agent Core]

    A --> B[Scheduler & Runner]
    A --> C[Planner / DAG Executor]
    A --> D[JobStore / EventStore]
    A --> E[Tool & Workflow Nodes]
    A --> F[RAG / Knowledge Nodes]
    A --> G[Monitoring & Visualization]
    A --> H[Security & Governance]

    B -->|depends on| D
    C -->|depends on| D
    E -->|optional| F
    G -->|reads| D
    H -->|controls access| B
    H -->|controls access| C
    H -->|controls access| E

    subgraph agentCollab [Agent Collaboration]
        A --> I[Multi-Agent Messaging]
        A --> J[Event-Driven Triggers]
        I --> B
        J --> C
    end

    subgraph perfStability [Performance and Stability]
        B --> K[Dynamic Worker Pool]
        B --> L[Task Prioritization and Timeout]
        D --> M[Distributed JobStore]
        C --> N[Async DAG Execution]
    end

    subgraph extensibility [Extensibility]
        E --> O[Custom Task Plugins]
        F --> P[Custom Knowledge Sources]
    end

    subgraph monitoring [Monitoring]
        G --> Q[Job / Task Dashboard]
        G --> R[Event Stream Visualization]
        G --> S[Debug / Replay Tools]
    end

    subgraph security [Security]
        H --> T[RBAC / Access Control]
        H --> U1[Audit Logs]
        H --> V[Multi-Tenant Isolation]
    end
```

---

## 4. 2.0 可证明性栈与架构升级图

在 1.0 Runtime 核心（Runner–Ledger–JobStore）之上，2.0 强化**可证明性**：先落地 Verification 层，再延伸事件链密封与安全/多租户边界。

```mermaid
graph TB
    subgraph runtimeCore [Runtime Core 1.0]
        Runner[Runner]
        Ledger[InvocationLedger]
        JobStore[JobStore / EventStore]
        Runner --> Ledger
        Runner --> JobStore
    end

    subgraph verification [Verification Layer]
        VerifyAPI[GET /api/jobs/:id/verify]
        VerifyCLI[aetheris verify]
        EventChainRoot[Event Chain Root Hash]
        ReplayProof[Replay Proof]
        ExecHash[Execution Hash]
        LedgerProof[Ledger Proof]
        VerifyAPI --> EventChainRoot
        VerifyAPI --> ReplayProof
        VerifyAPI --> ExecHash
        VerifyAPI --> LedgerProof
        VerifyCLI --> VerifyAPI
    end

    subgraph sealing [Optional Sealing 2.0]
        SignRoot[Sign Event Chain Root]
        VerifySig[Verify Signature]
    end

    subgraph boundary [Security and Multi-Tenant]
        RBAC[RBAC / Access Control]
        Namespace[Namespace / Tenant ID]
        Quota[Resource Quota]
    end

    JobStore --> VerifyAPI
    runtimeCore --> verification
    verification --> sealing
    sealing --> boundary
```

- **Runtime Core**：Runner、Ledger、JobStore 构成 1.0 执行与 at-most-once 保证。
- **Verification Layer**：只读校验与摘要；Verify API/CLI 输出 execution_hash、event_chain_root_hash、ledger proof、replay proof（见 [verification-mode.md](verification-mode.md)）。
- **Optional Sealing**：事件链根 hash 可选私钥签名、公钥校验（2.0 延伸，见 [event-chain-sealing.md](event-chain-sealing.md)）。
- **Security and Multi-Tenant**：Phase 4 能力边界（RBAC、Namespace、Quota）。

---

- **功能模块详图**：见上文 §1。
- **1.0 执行流与 StepOutcome**：[runtime-core-diagrams.md](runtime-core-diagrams.md)
- **1.0 运行时语义与 Ledger 状态机**：[1.0-runtime-semantics.md](1.0-runtime-semantics.md)
- **Verification Mode**：[verification-mode.md](verification-mode.md)
- **2.0 能力矩阵**：[docs/2.0-capability-matrix.md](../docs/2.0-capability-matrix.md)
