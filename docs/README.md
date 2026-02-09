# 文档中心

本目录为架构说明、使用指南与 API 文档的入口。

## 版本与变更

- [CHANGELOG.md](../CHANGELOG.md) — 版本历史与重要变更（v0.8 持久化运行时、事件化 JobStore、Job/Scheduler/Checkpoint/Steppable、v1 Agent API、TaskGraph 执行适配层、RulePlanner、Planner 选择等）

## 设计文档

- [design/core.md](../design/core.md) — 总体架构、分层、Agent Runtime 与任务执行、Pipeline 与 eino 编排核心
- [design/struct.md](../design/struct.md) — 仓库结构与模块职责（含 internal/agent、internal/runtime/jobstore）
- [design/services.md](../design/services.md) — 多 Service 架构（api / agent / index）
- [design/jobstore_postgres.md](../design/jobstore_postgres.md) — JobStore 事件模型与 Postgres 实现设计

## 使用与 API

- [使用说明（usage.md）](usage.md) — 启动方式、环境变量、典型流程、API 端点汇总、常见问题
- [端到端测试（test-e2e.md）](test-e2e.md) — 上传 → 解析 → 切分 → 索引 → 检索的完整测试步骤（PDF / AGENTS.md）
- [链路追踪（tracing.md）](tracing.md) — OpenTelemetry 配置、OTEL_EXPORTER_OTLP_ENDPOINT、本地 Jaeger 查看 trace

## 示例与部署

- [examples/](../examples/) — Agent、流式、工具、工作流示例代码
- [deployments/](../deployments/) — Docker、Compose、K8s 部署说明
