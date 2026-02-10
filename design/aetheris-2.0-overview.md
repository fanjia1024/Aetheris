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

下图在同一视图中体现 **时间阶段**、**各阶段关键模块** 与 **依赖关系**：从左到右为阶段顺序，箭头表示依赖或交付顺序。

```mermaid
flowchart LR
    subgraph Phase1 ["阶段 1 · 1–2 个月"]
        P1A[Job Scheduler and Runner]
        P1B[Planner and DAG Executor]
        P1C[Multi-Agent Messaging]
    end

    subgraph Phase2 ["阶段 2 · 2–3 个月"]
        P2A[RAG and Knowledge Nodes]
        P2B[Tool and Workflow Nodes]
        P2C[Custom Task Plugins]
    end

    subgraph Phase3 ["阶段 3 · 2 个月"]
        P3A[Monitoring and Visualization]
        P3B[Event Stream and Debug Tools]
    end

    subgraph Phase4 ["阶段 4 · 1–2 个月"]
        P4A[RBAC and Access Control]
        P4B[Audit Logs and Multi-Tenant]
    end

    User[User and API] --> P1A
    User --> P1B
    P1A --> P1C
    P1B --> P2A
    P1B --> P2B
    P2B --> P2C
    P1A --> P3A
    P1B --> P3B
    P3A --> P4A
    P3B --> P4B
    P4A --> P4B
```

---

- **功能模块详图**：见上文 §1。
- **1.0 执行流与 StepOutcome**：[runtime-core-diagrams.md](runtime-core-diagrams.md)
- **1.0 运行时语义与 Ledger 状态机**：[1.0-runtime-semantics.md](1.0-runtime-semantics.md)
