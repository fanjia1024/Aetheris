# Changelog

本文档记录 CoRag 项目的版本与重要变更。格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)。

## [Unreleased]

### Added

- **v1 Agent API**：以 Agent 为中心的 HTTP API
  - `POST /api/agents` — 创建 Agent（返回 id、name）
  - `GET /api/agents` — 列出所有 Agent
  - `POST /api/agents/:id/message` — 向 Agent 发送消息（写入 Session 并唤醒执行）
  - `GET /api/agents/:id/state` — 查看 Agent 状态（status、current_task、last_checkpoint、updated_at）
  - `POST /api/agents/:id/resume` — 恢复执行
  - `POST /api/agents/:id/stop` — 停止执行
- **Agent Runtime**：第一公民 Agent 与生命周期
  - `internal/agent/runtime`：RunContext、Agent、Session（v1）、Manager、Scheduler、Checkpoint
  - Session：Messages、Variables、CurrentTask、LastCheckpoint；并发安全读写
  - Manager：Create/Get/List/Delete（内存存储）
  - Scheduler：WakeAgent、Suspend、Resume、Stop；RunFunc 由应用注入以驱动执行
  - Checkpoint：结构体与 CheckpointStore（Save/Load/ListByAgent），内存实现
- **Memory 分层**：统一 Memory 接口与三层实现
  - `internal/agent/memory`：Memory（Recall/Store）、MemoryItem、CompositeMemory
  - Working（WorkingSession 基于 runtime.Session、Working 步骤结果）、Episodic、Semantic（包装检索）
- **TaskGraph 与 Planner 扩展**
  - TaskGraph：Nodes（id/type/config/tool_name/workflow）、Edges（from/to）；Marshal/Unmarshal 供 Checkpoint
  - Planner.PlanGoal(ctx, goal, mem) (*TaskGraph, error)；LLMPlanner 实现（含 JSON 解析与回退）
  - **RulePlanner**：无 LLM 的规则规划器，返回固定单节点 llm TaskGraph，便于稳定调试 Executor
- **Planner 选择**：环境变量 `PLANNER_TYPE=rule` 时 v1 Agent 使用 RulePlanner，否则使用 LLMPlanner；启动日志提示当前规划器
- **TaskGraph → eino DAG 执行适配层**
  - `internal/agent/runtime/executor`：AgentDAGPayload、NodeAdapter（LLM/Tool/Workflow）、Compiler、Runner
  - Compiler：TaskGraph + Agent → compose.Graph[*AgentDAGPayload,*AgentDAGPayload]，连接 START/END
  - Runner：单轮 PlanGoal → Compile → Invoke；eino 仅被 executor 调用，不再直接服务用户请求
  - 应用层注入 LLM/Tool/Workflow 适配器，Scheduler.RunFunc 调用 Runner.Run 完成「发消息 → 唤醒 → 执行 DAG」闭环

### Changed

- **Planner 集成**：v1 Agent 创建与执行统一通过 `planGoaler` 接口（PlanGoal），支持 LLMPlanner 与 RulePlanner 切换
- **Agent 执行路径**：主流程为 Agent API → Scheduler → Runner.Run → PlanGoal → TaskGraph → Compiler → eino DAG → Tools/RAG/LLM；Pipeline 作为 workflow 节点可被 TaskGraph 调用

### Deprecated

- `POST /api/query`、`POST /api/query/batch`：推荐使用 `POST /api/agents/{id}/message` 以 Agent 为中心与系统交互；路由与 Handler 注释已标 Deprecated

---

## 历史版本（摘要）

以下为早期提交对应的功能摘要，未按语义化版本打 tag 时可按提交顺序参考。

- **refactor: update planner integration for v1 Agent API** — planGoaler 接口、RulePlanner、PLANNER_TYPE 环境变量
- **feat: implement v1 Agent API and enhance session management** — v1 Agent 端点、Manager/Scheduler/Creator、Session 管理
- **feat: refactor agent execution to support session management and enhance planning** — Session 感知执行、Planner 单步决策、SchemaProvider
- **feat: add agent execution endpoint and integrate agent runner** — `/api/agent/run`、AgentRunner、Session 管理
- **feat: implement gRPC support and JWT authentication** — gRPC 服务、JWT 中间件、文档/查询 gRPC 方法
- **feat: integrate OpenTelemetry for tracing** — 链路追踪与文档处理增强
- **feat: enhance API configuration and workflow execution** — API 配置与工作流执行
- **refactor: migrate from Gin to Hertz** — HTTP 框架由 Gin 迁移至 Hertz
- **feat: 初始化RAG/Agent平台核心组件和架构** — 初始 RAG/Agent 平台骨架
