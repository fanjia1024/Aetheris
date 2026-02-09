# CoRag 1.0 发布验证（Release Certification）

本清单为 **1.0 发布门禁**：按顺序执行，逐条打勾。**任一条失败 → 不能发 1.0。**

---

## 0. 准备环境

**最小拓扑**：1 API + 2 Worker + 1 存储。

- **Tests 3、8** 必须使用 **Postgres** 作为 job 存储（`jobstore.type=postgres`）；否则 API/进程崩溃后 job 状态不共享，无法通过。
- 存储（向量/元数据）可为内存；仅用于本认证时。

**启动**：

```bash
# 终端 1
go run ./cmd/api

# 终端 2
go run ./cmd/worker

# 终端 3
go run ./cmd/worker
```

**验证**：

```bash
curl -s http://localhost:8080/api/health
```

- **通过条件**：返回 200，响应表示 OK。

---

## 第一部分：Runtime 正确性（v0.9 Gate）

### 测试 1：Job 不丢失

1. 创建 Agent：
   ```bash
   curl -s -X POST http://localhost:8080/api/agents -H "Content-Type: application/json" -d '{}'
   ```
   记录返回的 `id` 为 `agent_id`。

2. 发送消息：
   ```bash
   curl -s -X POST http://localhost:8080/api/agents/{agent_id}/message \
     -H "Content-Type: application/json" \
     -d '{"message":"帮我写一篇 200 字的 AI 介绍，并调用工具"}'
   ```
   得到 **202 Accepted**，记录 `job_id`。

3. 轮询状态：
   ```bash
   curl -s http://localhost:8080/api/jobs/{job_id}
   ```

**通过条件**：状态依次出现 `pending` → `running` → `completed`。若一直卡在 `pending`，则 Scheduler/Worker 未真正工作 → **失败**。

---

### 测试 2：Worker Crash Recovery（1.0 最关键）

1. 在任务**运行过程中**杀死一个 worker：
   ```bash
   ps aux | grep worker
   kill -9 <pid>
   ```

2. 观察同一 job：
   ```bash
   curl -s http://localhost:8080/api/jobs/{job_id}
   ```

**1.0 必须结果**：

- 同一 `job_id` **最终**变为 `completed`（可能先保持 `running`，直到另一 Worker 重新 Claim）。
- **不重新规划**（事件流中仅一条 `plan_generated`）。
- **不重新开始**、**不丢步骤**（从 Replay/checkpoint 续跑，已完成节点不重执行）。

若变为 `RUNNING` → `FAILED`，或从头重新执行 → **不能发 1.0**。

*说明：当前无 `stalled` 状态；若后续增加「租约过期未续约」的 stalled 展示，则可能观察到 RUNNING → STALLED → RUNNING → COMPLETED。*

---

### 测试 3：API Crash

1. 任务运行中关闭 API（如 Ctrl+C）。
2. 约 10 秒后重启 API：`go run ./cmd/api`。

**1.0 必须**：同一 job 继续执行并完成（由 Worker 从 Postgres 继续 Claim/执行）。

若 job 丢失或状态回退，说明 Job 状态仅在 API 内存 → **直接判定不是 Runtime**。本测试要求 `jobstore.type=postgres`。

---

### 测试 4：多 Worker 并发一致性

同时创建 10 个任务：

```bash
for i in $(seq 1 10); do
  curl -s -X POST http://localhost:8080/api/agents/{agent_id}/message \
    -H "Content-Type: application/json" \
    -d '{"message":"查询天气并总结"}' &
done
wait
```

**检查**：日志与事件中 **每个 job 只被执行一次**。若出现同一 job 被两个 Worker 同时执行、或同一 tool 被调用两次 → Claim/Lease 失效 → **不可发布 1.0**。

---

### 测试 5：Replay 一致性

任务完成后，可重启所有 Worker 再验证：

1. `kill -9` 所有 worker，重新启动 worker。
2. 调用 **只读** 的 Replay/Trace 接口（**不触发任何执行**）：
   ```bash
   curl -s http://localhost:8080/api/jobs/{job_id}/replay
   # 或
   curl -s http://localhost:8080/api/jobs/{job_id}/trace
   curl -s http://localhost:8080/api/jobs/{job_id}/events
   ```

**1.0 必须**：

- 得到**相同执行路径**（与事件流一致）。
- **不再次调用 LLM**、**不再次调用工具**（上述接口仅读事件，不跑 Runner）。

若「replay」实际重新执行流程，则只是重新跑流程，不是 runtime replay → **失败**。

---

### 测试 6：取消任务

任务运行中执行：

```bash
curl -s -X POST http://localhost:8080/api/jobs/{job_id}/stop
```

**通过条件**：

- 状态变为 `running` → `cancelled`。
- LLM 立即终止、Worker 不再执行该 job。

若任务继续跑 → 企业环境不可用 → **失败**。

---

## 第二部分：1.0 平台能力（v1.0 Gate）

### 测试 7：Trace 可解释性

对任意一个已完成 job，应能通过 API 看到：

- 每个节点（node_id、类型）。
- 若为 tool 节点：tool 输入/输出（来自 `tool_called` / `tool_returned` 或 `node_finished` payload）。
- 耗时（由 `node_started` / `node_finished` 时间戳可推算，或 trace 中直接提供 duration）。

**1.0 定义**：用户能理解 Agent **为什么做这个决策**。若仅能看日志 `INFO executing node...`，不视为 Trace UI/可解释性 → **失败**。

接口示例：

```bash
curl -s http://localhost:8080/api/jobs/{job_id}/trace
curl -s http://localhost:8080/api/jobs/{job_id}/nodes/{node_id}
```

---

### 测试 8：真正恢复（最硬核）

1. 创建一条**长任务**（多步 tool/LLM）。
2. **同时** kill：
   ```bash
   kill -9 所有 worker
   kill -9 api
   ```
3. 等待约 10 秒。
4. **全部重启**：先起 API，再起 2 个 Worker。

**通过条件**：同一 job 继续执行并**完成** → 拥有真正的 Agent Runtime 1.0。

若失败 → 即当前与 1.0 的差距。本测试要求 Postgres job 存储。

---

## 第三部分：可运营能力

1.0 必须支持排障，应能：

| 能力           | 接口 |
|----------------|------|
| 查看某 Agent 下所有 job | `GET /api/agents/{id}/jobs` |
| 查看某个 job 详情       | `GET /api/jobs/{id}` |
| 查看某个 job 执行步骤   | `GET /api/jobs/{id}/trace`、`GET /api/jobs/{id}/events`、`GET /api/jobs/{id}/nodes/{node_id}` |
| 强制取消               | `POST /api/jobs/{id}/stop` |
| Replay/只读回放        | `GET /api/jobs/{id}/replay` 或 trace+events |

「重新执行」指新发一条 message 创建新 job，而非对同一 job 再次执行。

---

## 发布判定标准（最终）

仅当以下**四条全部满足**时，方可宣称：

> **CoRag v1.0 — Agent Runtime Platform**

1. **任意进程崩溃任务不丢**
2. **恢复不重复调用工具**
3. **可完整回放执行**（只读 trace/replay，不重跑）
4. **用户可观察执行原因**（Trace：节点、tool I/O、耗时）

少一条 → 仅能标为 v0.9。

---

## 自动化脚本

可自动化的步骤（环境已就绪：API + Worker 已启动）可由脚本执行：

```bash
./scripts/release-cert-1.0.sh
```

脚本覆盖：Step 0（health）、Test 1（Job 不丢失）、Test 6（取消）、Test 7（Trace）、可选 Test 4（多 job）、Test 5（Replay）。Tests 2、3、8 需**手动**杀进程/重启并按上述条件验证。
