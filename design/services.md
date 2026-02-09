# Go + eino 多 Service 架构设计

## 1. 架构概览

系统基于 Go 语言，采用 **多 Service + 单一编排核心（eino）** 的设计：

```
┌──────────────┐      ┌────────────────┐
│  api-service │ ───▶ │ agent-service  │
└──────────────┘      │   (eino)       │
                      └───────┬────────┘
                              ▼
                      ┌────────────────┐
                      │ index-service  │
                      └────────────────┘
```

---

## 2. api-service

### 职责

- 对外 API
- 文件上传 / 查询请求
- 权限、限流、校验

### 特点

- 无状态
- 不依赖 Pipeline
- 不运行 Workflow

---

## 3. agent-service（核心）

### 职责

- eino Workflow / DAG 执行
- Agent 调度
- Pipeline 编排
- 模型调用决策

### 内部结构

任务执行路径：Job + 事件流 JobStore（版本化 Append、Claim/Heartbeat）+ Scheduler 拉取 → Runner.RunForJob（Steppable + 节点级 Checkpoint）；eino 作为 DAG 执行内核由 executor 调用。

```
Agent Runtime
├── Job / 事件流 JobStore（ListEvents、Append、Claim、Heartbeat、Watch）
├── Scheduler（拉取 Pending Job）
├── Runner.RunForJob（Steppable、Checkpoint 恢复）
└── eino Runtime（Workflow / DAG，仅被 executor 调用）
    ├── Workflow Engine
    ├── Context / Memory
    └── Retry / Fallback
```

---

## 4. index-service

### 职责

- 离线索引
- 文档解析
- 切片
- Embedding
- 向量索引构建

### 特点

- 高并发
- 可异步
- 可横向扩展

---

## 5. 通信方式

| 调用方          | 被调用方      | 协议        |
| --------------- | ------------- | ----------- |
| api → agent     | agent-service | HTTP / gRPC |
| agent → index   | index-service | gRPC        |
| agent ↔ storage | DB / Cache    | 原生        |

---

## 6. 架构原则

- **Single Orchestrator**：只有 agent-service 可编排
- **Pipeline is Passive**：Pipeline 不主动运行
- **Offline / Online 隔离**
- **Agent-Native Design**

---

## 7. 演进方向

- Planner Agent
- Tool-Using Agent
- 多 Workflow 协同
- 人工干预（Human-in-the-loop）
- 多 Worker 与持久化 JobStore（如 Postgres）：接口与设计已具备，见 [jobstore_postgres.md](jobstore_postgres.md)
