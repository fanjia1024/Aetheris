# Aetheris v1.0 发布后自检清单

用于发布后快速确认核心功能、事件追踪、分布式执行、CLI/API、日志监控与文档同步。

---

## 1. 核心功能验证

- [ ] **Job 创建** — 能通过 Agent API 或 CLI 创建 Job。验证方式：`POST /api/agents/:id/message` 或 `corag chat <agent_id>` 发送消息，确认返回 `job_id`。
- [ ] **Job 执行** — Job 从开始到结束顺利完成。验证方式：执行示例 DAG 或 TaskGraph，观察 Job 状态至完成。
- [ ] **Event 流记录** — 每个动作（planning、tool call、step、retry、failure）都被记录。验证方式：`GET /api/jobs/:id/events` 或 `corag trace <job_id>` 查看 JobStore 事件流。
- [ ] **Job 重放** — Job 可重放并得到相同结果。验证方式：`GET /api/jobs/:id/replay` 或 `corag replay <job_id>` 测试。
- [ ] **Idempotency** — Job 重复执行不会导致副作用。验证方式：对同一 Job 重复触发重放或查询，确认无重复写或异常。
- [ ] **Job 中断恢复** — 模拟任务中断后，Job 能恢复。验证方式：运行中 kill Runner 或 Worker，重启后通过 `POST /api/agents/:id/resume` 或 Scheduler 继续执行。

## 2. 分布式执行与 Worker 系统

- [ ] **多 Worker 支持** — Task 可分配到不同 Worker 并执行。验证方式：部署 2+ Worker（如 `go run ./cmd/worker` 多实例），运行同一 Agent 的 Job，确认任务被不同 Worker 处理。
- [ ] **Scheduler 重试机制** — 任务失败后自动重试。验证方式：故意触发任务失败（如断网、错误 tool），观察 Scheduler 重试与退避。
- [ ] **Runner Checkpoint** — 每步执行有 checkpoint，可断点恢复。验证方式：中途停止 Runner，重新启动 Worker，确认从 checkpoint 恢复执行。

## 3. CLI / Agent API 功能

- [ ] **CLI 命令可用** — 常用命令可正常执行。验证方式：`corag agent create [name]`、`corag chat [agent_id]`、`corag jobs <agent_id>`、`corag trace <job_id>`、`corag replay <job_id>`、`corag cancel <job_id>`、`corag workers`。
- [ ] **Agent API 可用** — REST 接口可正常调用。验证方式：用 curl、Postman 或测试脚本调用 `POST /api/agents`、`POST /api/agents/:id/message`、`GET /api/agents` 等。
- [ ] **Job Cancel** — 支持 Job 取消。验证方式：`POST /api/jobs/:id/stop` 或 `corag cancel <job_id>`，确认 Job 进入取消/已停止状态。
- [ ] **Event 查询** — 支持按 Job 查询事件流。验证方式：`GET /api/jobs/:id/events` 或 `corag trace <job_id>`、`corag replay <job_id>`。

## 4. 日志与监控

- [ ] **Trace UI 可用** — 能查看任务执行状态、DAG、TaskGraph。验证方式：打开 `GET /api/jobs/:id/trace/page` 返回的 Trace 页面 URL，或查看 `GET /api/jobs/:id/trace` JSON。
- [ ] **日志完整** — 包含 Job 执行、错误、重试信息。验证方式：查看 API/Worker 系统日志，确认关键步骤与错误有记录。
- [ ] **审计可用** — 每个决策可回溯。验证方式：随机抽查历史 Job 的 events/trace，确认规划、工具调用、节点完成等可追溯。

## 5. 文档与版本

- [ ] **README 更新** — 与 v1.0 功能一致。验证方式：核对 README 中架构、运行方式、CLI 说明与当前实现一致。
- [ ] **CHANGELOG** — 包含 v1.0 版本信息。验证方式：核对 CHANGELOG.md 与 GitHub Release 说明。
- [ ] **AGENTS.md** — 反映最新 agent 使用示例与项目定位。验证方式：核对文档中的命令、目录结构、技术栈描述。
- [ ] **GitHub Release 标签** — v1.0 标签正确。验证方式：在 GitHub Releases 页确认 v1.0 标签与发布说明。

## 6. 可选高级检查

- [ ] **RAG 功能** — Agent 可选调用 RAG 能力（若已接入）。验证方式：运行相关示例或 pipeline，确认检索与回答正常。
- [ ] **DAG 边缘情况** — DAG 含循环、复杂分支时行为正确。验证方式：构建特殊 DAG 测试，确认执行顺序与 checkpoint 一致。
- [ ] **压力测试** — 多 Job 并发执行稳定。验证方式：部署压力测试脚本，观察成功率与延迟。

---

勾选完成后，可连同 [release-certification-1.0.md](release-certification-1.0.md) 一起作为 v1.0 发布验收依据。
