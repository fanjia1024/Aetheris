# v0.9 发布验收（Runtime 正确性）

在 Postgres + Worker 部署下，逐条执行以下 9 项校验；缺一不可。

## 前置条件

- 已部署：1 API + 至少 1 Worker + Postgres（或使用 `deployments/compose`）
- `CORAG_API_URL` 指向 API（默认 http://localhost:8080）
- CLI：`go run ./cmd/cli` 或安装后的 `corag`

---

## 1. Worker Crash Recovery

**目标**：杀 Worker 后任务不丢、从 checkpoint/replay 续跑、不重复执行已完成节点。

1. 启动 1 API + 1 Worker + PG；创建 Agent；发一条会触发多步（tool + LLM）的消息，记下 `job_id`。
2. 执行到中途（如第一个 NodeFinished 之后）对 Worker 进程 `kill -9`。
3. 再启动新 Worker（或同机再起一个）。
4. **验证**：同一 `job_id` 最终 Completed；事件流中无重复 PlanGenerated；Tool 调用次数符合预期（无重复执行已完成节点）。

```bash
# 示例（需根据实际 agent_id 与消息调整）
corag agent create test
corag chat <agent_id>   # 发一条消息，记下 job_id；在另一终端 kill -9 worker
# 重启 worker 后轮询 job 状态直至 completed
corag trace <job_id>   # 检查事件序列
```

---

## 2. API 重启不影响任务

1. 运行中任务存在时重启 API（Worker 不重启）。
2. **验证**：job 不丢、状态不回退、Worker 继续执行至完成。

```bash
# 发消息得到 job_id 后，重启 API 进程；观察 job 是否仍被 worker 执行完成
```

---

## 3. 多 Worker 不重复执行

1. 启动 3 个 Worker，连续创建约 10 个 job。
2. **验证**：每个 job 只完成一次；日志/事件中无重复的 tool 调用或重复的 plan。

```bash
# 启动 3 个 worker 后，用脚本或循环 POST 10 条 message；检查各 job 的 trace 与完成次数
```

---

## 4. Planner Determinism

1. 对同一 job：在恢复前后各导出一份「执行图/节点序列」（例如从 trace 或事件流推导）。
2. **验证**：恢复前 DAG 与恢复后 DAG 一致（节点集合与顺序一致）。

```bash
corag trace <job_id>   # 恢复前保存一份
# 触发恢复（如 kill worker 再起）后
corag trace <job_id>   # 恢复后再取一份；对比 plan_generated 与节点序列
```

---

## 5. Session Persistence

1. 多轮对话后重启 Worker，再发一条依赖上文的消息。
2. **验证**：LLM 回答仍引用之前轮次内容。

```bash
corag chat <agent_id>  # 多轮对话
# 重启 worker
# 再发一条依赖上文的消息，检查回复是否延续上下文
```

---

## 6. Tool Idempotency

1. 在恢复场景下，对已完成的 tool 节点：确认日志/事件中该 tool 只被调用一次；恢复后仅后续节点执行。
2. **验证**：恢复后仅后续节点执行；已完成 tool 不重复调用。

（与 1、4 结合：恢复后 trace 中 tool_called/tool_returned 每节点仅一次。）

---

## 7. Event Log Replay

1. 用 `GET /api/jobs/:id/trace` 或 `GET /api/jobs/:id/events` 取事件序列。
2. **验证**：可完整重放执行路径；与一次真实恢复执行对比，结果一致。

```bash
corag replay <job_id>
# 或
curl -s "$CORAG_API_URL/api/jobs/<job_id>/events"
```

---

## 8. Backpressure

1. 连续创建 100+ job（或配置的并发上限以上）。
2. **验证**：系统不 OOM、goroutine 不爆炸、LLM 不同时请求过多；Worker 使用 `worker.concurrency` 限制并发。

```bash
# 循环创建 100 个 job；观察内存与 goroutine 数；确认 worker 配置 max_concurrency 生效
```

---

## 9. Cancellation

1. 执行中调用 `POST /api/jobs/:id/stop`。
2. **验证**：LLM 终止、tool 中断、job 进入 CANCELLED。

```bash
# 发一条长任务，记下 job_id；立即执行：
corag cancel <job_id>
# 或
curl -X POST "$CORAG_API_URL/api/jobs/<job_id>/stop"
# 检查 job 状态为 cancelled，事件流含 job_cancelled
corag trace <job_id>
```

---

## 判定

当且仅当上述 9 项全部通过，可打 **v0.9** 标签。

脚本骨架见 `scripts/release-acceptance-v0.9.sh`（可逐步替换为可执行命令）。
