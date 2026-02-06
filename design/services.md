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

```
eino Runtime
├── Workflow Engine
├── Agent Executor
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
