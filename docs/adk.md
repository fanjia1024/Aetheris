# Eino ADK 集成说明

本文说明项目中 [Eino ADK](https://www.cloudwego.io/zh/docs/eino/core_modules/eino_adk/) 的接入方式与对话/恢复/流式接口用法。

## 默认对话路径

当配置中 **agent.adk.enabled** 未设为 `false` 时，以下接口由 **ADK Runner** 执行（主 ChatModelAgent + 检索、生成、文档加载/解析/切片/向量化/建索引等工具）：

- **POST /api/agent/run** — 单次对话，请求体 `{"query":"...", "session_id":"可选"}`，返回 `answer`、`session_id`、`steps` 等。
- **POST /api/agent/resume** — 从 checkpoint 恢复，请求体 `{"checkpoint_id":"...", "session_id":"可选"}`。
- **POST /api/agent/stream** — 流式对话，请求体同 run，响应为 SSE（`text/event-stream`），事件中含 `answer`、`session_id`。

Session 历史会转换为 ADK 消息（最近 20 轮）传入 Runner，执行结果写回 Session 并保存。

## CheckPointStore 与中断/恢复

Runner 使用 **CheckPointStore**（当前为内存实现）保存中断点。当 Agent 内调用 `adk.Interrupt(ctx, info)` 时，框架会写入 checkpoint，调用方可通过 **POST /api/agent/resume** 传入返回的 `checkpoint_id` 恢复执行。配置项 **agent.adk.checkpoint_store** 目前仅支持 `memory`，后续可扩展为 postgres/redis 等持久化存储。

## 禁用 ADK

在 **configs/api.yaml** 中设置：

```yaml
agent:
  adk:
    enabled: false
```

则 **POST /api/agent/run** 将使用原 Plan→Execute Agent（Planner + Executor + Tools），**/api/agent/resume** 与 **/api/agent/stream** 会返回 503（ADK Runner 未配置）。

## 后续：Multi-Agent 与 Workflow

Phase 2 可通过配置或 API 注册命名 Agent（如 Sequential、Loop、Parallel、Supervisor、Plan-Execute），并对外暴露不同 Runner，使 run/resume/stream 按 agent 名选择执行器。参见 [Eino ADK Agent 实现](https://www.cloudwego.io/zh/docs/eino/core_modules/eino_adk/agent_implementation/)。
