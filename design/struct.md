# Go + eino RAG / Agent 平台仓库结构设计

## 1. 仓库总览

```
rag-platform/
├── cmd/
├── internal/
├── pkg/
├── configs/
├── scripts/
├── deployments/
├── docs/
├── go.mod
└── go.sum
```

---

## 2. cmd/ —— 启动入口（极薄）

> 只做三件事：**加载配置 / 初始化依赖 / 启动服务**

```
cmd/
├── api/
│   └── main.go            # HTTP / gRPC API 服务
├── worker/
│   └── main.go            # 离线任务 / Index Worker
└── cli/
    └── main.go            # CLI（调试 / 管理）
```

**注意：**

- ❌ 这里不允许写 Pipeline
- ❌ 不允许写 eino Workflow
- ✅ 只能调用 `internal/app`

---

## 3. internal/ —— 系统核心（重点）

### 3.1 internal/app —— 应用装配层（Composition Root）

```
internal/app/
├── api/
│   └── app.go              # API Server 装配
├── worker/
│   └── app.go              # Worker 装配
└── bootstrap.go            # 统一初始化（DB / Cache / Models）
```

作用：

- 依赖注入
- 组件组装
- 生命周期管理

---

### 3.2 internal/runtime —— eino 与运行时基础设施

```
internal/runtime/
├── eino/
│   ├── engine.go           # eino Runtime 初始化
│   ├── workflow.go         # Workflow 注册
│   ├── agent.go            # Agent 定义
│   ├── node.go             # 通用 Node 封装
│   └── context.go          # Context / State 扩展
├── jobstore/
│   ├── event.go            # JobEvent、EventType
│   ├── store.go            # JobStore 接口（ListEvents/Append/Claim/Heartbeat/Watch）
│   └── memory_store.go     # 内存实现（版本、租约、Watch）
└── session/
    └── ...                 # 会话存储等
```

- **eino**：Workflow / DAG、Agent 执行、Context 传递；仅作为执行内核被 agent executor 调用。
- **jobstore**：任务事件流与调度语义（ListEvents 带 version、版本化 Append、Claim/Heartbeat 租约、Watch）；当前为内存实现，为崩溃恢复、多 Worker、审计回放打基础。
- **session**：会话相关存储与能力。

---

### 3.3 internal/agent —— Agent 运行时与领域

```
internal/agent/
├── runtime/
│   ├── agent.go, session.go, manager.go, scheduler.go, checkpoint.go
│   └── executor/
│       ├── types.go, node_adapter.go, compiler.go, runner.go, steppable.go
├── job/
│   ├── job.go, job_store.go, scheduler.go, job_runner.go
├── memory/
├── planner/
└── tools/
```

- **agent/runtime**：Agent、Session、Manager、Scheduler、Checkpoint。**executor/**：AgentDAGPayload、NodeAdapter（LLM/Tool/Workflow）、Compiler、Runner、Steppable（TopoOrder、CompileSteppable）、RunForJob 与节点级 Checkpoint 恢复。
- **agent/job**：Job 实体、JobStore（Create/Get/UpdateStatus/UpdateCursor/ClaimNextPending/Requeue）、Scheduler（并发与重试）、JobRunner；当前由 app 使用 Scheduler 拉取 Job 并执行 RunForJob。
- **agent/memory**：Memory、WorkingSession、Working、Episodic、Semantic。
- **agent/planner**：TaskGraph、Planner、LLMPlanner、RulePlanner。
- **agent/tools**：Tool、ToolResult（Done/State/Output）、Registry。

---

### 3.4 internal/pipeline —— 业务 Pipeline（纯 Go）

```
internal/pipeline/
├── ingest/
│   ├── loader.go           # DocumentLoader
│   ├── parser.go           # DocumentParser
│   ├── splitter.go         # Splitter Node
│   ├── embedding.go        # Embedding Pipeline
│   └── indexer.go          # Index Builder
│
├── query/
│   ├── retriever.go
│   ├── reranker.go
│   ├── generator.go
│   └── responder.go
│
├── specialized/
│   ├── jsonl.go
│   ├── hive.go
│   └── longtext.go
│
└── common/
    ├── types.go            # Pipeline Context
    └── errors.go
```

原则：

- Pipeline = **业务步骤**
- 不关心顺序
- 顺序由 eino Workflow 决定

---

### 3.5 internal/splitter —— 切片引擎（能力模块）

```
internal/splitter/
├── engine.go
├── structural.go
├── semantic.go
└── token.go
```

- 被 Pipeline 调用
- 不独立运行
- 插件化设计

---

### 3.6 internal/model —— 模型抽象层

```
internal/model/
├── llm/
│   ├── interface.go
│   └── adapter.go
├── embedding/
│   ├── interface.go
│   └── adapter.go
├── vision/
│   ├── interface.go
│   └── adapter.go
└── registry.go
```

职责：

- 屏蔽厂商差异
- 支持运行时切换
- 与 eino 深度集成

---

### 3.7 internal/storage —— 存储抽象

```
internal/storage/
├── metadata/
│   ├── store.go            # MySQL / TiDB
│   └── repo.go
├── vector/
│   ├── store.go            # Milvus / Vearch / ES
│   └── index.go
├── object/
│   ├── store.go            # OSS / S3
│   └── file.go
└── cache/
    └── cache.go            # Redis / Local
```

---

### 3.8 internal/api —— API 层实现

```
internal/api/
├── http/
│   ├── router.go
│   ├── handler.go
│   └── middleware.go
└── grpc/
    └── service.go
```

- 不直接调用 storage
- 只调用 runtime / pipeline façade

---

## 4. pkg/ —— 可复用公共库（谨慎）

```
pkg/
├── log/
├── config/
├── tracing/
├── errors/
└── utils/
```

规则：

- 不允许依赖 internal
- 可以被外部项目使用

---

## 5. configs / scripts / deployments

```
configs/
├── api.yaml
├── worker.yaml
└── model.yaml

scripts/
├── migrate.sh
└── bootstrap.sh

deployments/
├── docker/
├── k8s/
└── compose/
```

---

## 6. 关键调用关系（必须遵守）

```
cmd
 ↓
internal/app
 ↓
internal/api (Handler：创建 Job、双写事件流)
 ↓
internal/agent/job (Scheduler 拉取 Job)
 ↓
internal/agent/runtime/executor (Runner.RunForJob → Planner / Compiler / Steppable)
 ↓
internal/runtime/eino（仅被 executor 调用）
 ↓
internal/pipeline / internal/model / storage
```

❌ 禁止反向依赖
❌ 禁止 pipeline → pipeline 互调

---

## 7. 这个结构的长期价值

- ✅ Agent / Tool / Memory 天然可扩展
- ✅ RAG / Workflow / DAG 都是“一等公民”
- ✅ 可以拆成：
  - api-service
  - worker-service
  - agent-service

---

## 8. 结语

这是一个 **Agent-Native 的 Go 系统骨架**，
不是“业务项目结构”，而是 **平台级结构**。

你现在已经站在 **2026 架构水位线** 上了。
