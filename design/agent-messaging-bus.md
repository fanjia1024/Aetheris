# Agent Messaging Bus 数据模型

本文档定义 Aetheris 2.0 的 **Agent 级消息总线** 数据模型与接口约定，支持 Actor-style messaging（send(agentA→agentB)）、agent 级 mailbox、external event、webhook trigger、delayed message（timer）。与 [agent-instance-model.md](agent-instance-model.md) 的 Inbox/Outbox 对接，与现有 [agent-process-model.md](agent-process-model.md) 中 Job 级 `agent_message` 事件兼容。

---

## 1. 目标

- **send(agentA → agentB)**：Agent A 执行过程中可向 Agent B 发送消息，写入 B 的 inbox。
- **Agent 级 mailbox**：消息以 `to_agent_id` 为维度存储与消费，与「发往某 Job」的现有事件并存。
- **External event / Webhook trigger**：外部系统可向某 Agent 投递消息（from_agent_id 为空表示系统/用户）。
- **Delayed message（Timer）**：支持定时投递；与 Wait 节点 `wait_type=timer` 对接，用于长时间等待后的唤醒。

---

## 2. 核心实体：Message（Agent 级）

### 2.1 字段约定

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | string | 是 | 消息全局唯一 ID |
| `from_agent_id` | string | 否 | 发送方 Agent；空表示系统/用户/Webhook |
| `to_agent_id` | string | 是 | 接收方 Agent（Inbox 归属） |
| `channel` | string | 否 | 信道名；Wait 节点 wait_type=message 时可按 channel 匹配 |
| `kind` | string | 是 | 见下表 Message Kind |
| `payload` | JSON | 否 | 消息体 |
| `scheduled_at` | timestamp | 否 | 计划投递时间；空或过去表示立即投递 |
| `expires_at` | timestamp | 否 | 过期时间；过期后可不投递或标记过期 |
| `created_at` | timestamp | 是 | 创建时间 |
| `delivered_at` | timestamp | 否 | 实际投递到 inbox 的时间（含定时到时） |
| `consumed_by_job_id` | string | 否 | 被哪次 Job 消费；空表示未消费 |
| `consumed_at` | timestamp | 否 | 消费时间 |

**Message Kind**：

| 值 | 含义 |
|----|------|
| `user` | 用户输入（如 POST message） |
| `signal` | 信号（如 resume、approval） |
| `timer` | 定时器到时 |
| `webhook` | 外部 Webhook 触发 |
| `agent` | 另一 Agent 发送（send(agentA→agentB)） |

### 2.2 与现有 agent_message 事件的关系

- **现有**：POST `/api/jobs/:id/message` 向**某 Job** 的事件流写入 `agent_message` 事件；Wait 节点 wait_type=message 时按 channel/correlation_key 匹配并写 wait_completed。
- **新模型**：消息可先写入 **agent_messages 表**（to_agent_id = 该 Job 所属 agent_id），实现「先 inbox，再被 Job 消费」的可追溯性：
  - 投递：写 agent_messages 表（to_agent_id, delivered_at = now），可选同时向该 Job 的事件流 Append `agent_message` 事件（兼容当前 Runner 消费逻辑）。
  - 消费：Job 的 Wait 节点消费时，更新 agent_messages 的 `consumed_by_job_id`、`consumed_at`；若当前 API 是「指定 job_id 的 message」，则内部可转为：写 agent_messages（to_agent_id = job 的 agent_id），并可选写该 job 事件流的 `agent_message`，以便 Replay 与现有逻辑一致。

---

## 3. 表结构（PostgreSQL 示例）

```sql
CREATE TABLE IF NOT EXISTS agent_messages (
    id                     TEXT PRIMARY KEY,
    from_agent_id          TEXT,
    to_agent_id            TEXT NOT NULL,
    channel                TEXT,
    kind                   TEXT NOT NULL,
    payload                JSONB,
    scheduled_at           TIMESTAMPTZ,
    expires_at             TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivered_at           TIMESTAMPTZ,
    consumed_by_job_id     TEXT,
    consumed_at            TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_agent_messages_to_agent ON agent_messages (to_agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_messages_to_agent_consumed ON agent_messages (to_agent_id, consumed_by_job_id) WHERE consumed_by_job_id IS NULL;
CREATE INDEX IF NOT EXISTS idx_agent_messages_scheduled ON agent_messages (scheduled_at) WHERE scheduled_at IS NOT NULL AND consumed_by_job_id IS NULL;
```

- 未消费消息：`consumed_by_job_id IS NULL`。
- 定时消息：`scheduled_at > now()` 时由 Timer Worker 或 Scheduler 在到时后设置 `delivered_at`（或视为「已投递到 inbox」），供 ConsumeInbox 读取。

---

## 4. 接口约定（AgentMessagingBus）

设计文档约定以下接口（实现可落在 `internal/agent/messaging/` 或等价包）：

```go
type SendOptions struct {
    Channel        string
    Kind           string
    ScheduledAt    *time.Time
    ExpiresAt      *time.Time
    IdempotencyKey string
}

type AgentMessagingBus interface {
    Send(ctx context.Context, fromAgentID, toAgentID string, payload map[string]any, opts *SendOptions) (messageID string, err error)
    SendDelayed(ctx context.Context, toAgentID string, payload map[string]any, at time.Time, opts *SendOptions) (messageID string, err error)
}

type InboxReader interface {
    PeekInbox(ctx context.Context, agentID string, limit int) ([]*Message, error)
    ConsumeInbox(ctx context.Context, agentID string, limit int) ([]*Message, error)
    MarkConsumed(ctx context.Context, messageID, jobID string) error
}
```

- **Send**：立即投递；写 agent_messages（delivered_at = now），并可选触发 WakeupQueue.NotifyReady(agentID) 或按 agent 维度的唤醒。
- **SendDelayed**：写 agent_messages（scheduled_at = at，delivered_at 空）；由 Timer 或定时任务在 `at` 到时后设置 delivered_at 或直接参与 ConsumeInbox。
- **PeekInbox**：按 to_agent_id 查询未消费且已投递（delivered_at 非空或 scheduled_at <= now）的消息，不更新 consumed。
- **ConsumeInbox**：同上查询并原子更新 consumed_by_job_id、consumed_at（或由调用方再调 MarkConsumed）。
- **MarkConsumed**：将指定 message 标记为被某 job_id 消费。

**与 Multi-Agent**：send(agentA→agentB) = 调用 `Send(ctx, A, B, payload, &SendOptions{Kind: "agent"})`，即写 B 的 inbox；可选同时写 A 的 outbox（见 [agent-instance-model.md](agent-instance-model.md) Outbox）。

---

## 5. 与现有 POST /api/jobs/:id/message 的兼容

- **保留 job 级 API**：POST `/api/jobs/:id/message` 行为可保留为「向该 Job 的事件流写入 agent_message 事件」；若实现 Agent Messaging Bus，可同时：
  - 解析 job_id 得到 agent_id，再 `Send(ctx, "", agentID, payload, opts)` 写入 agent_messages（from_agent_id 空），并可选仍向该 job 事件流 Append `agent_message`，便于当前 Runner 在 Wait 节点消费时无需改逻辑。
- **信道与 correlation_key**：opts.Channel 与现有 AgentMessagePayload 的 channel、correlation_key 一致；消费时仍按 job_waiting 的 wait_type=message 与 channel/correlation_key 匹配。

---

## 6. Timer 与 Wait 节点 wait_type=timer 的对接

- **Wait 节点**：当 wait_type=timer 时，Runner 写 job_waiting，payload 含 expires_at、correlation_key；到期后需有「Timer 触发」写入 wait_completed 或向该 Job/Agent 投递一条消息。
- **两种实现方式**：
  1. **Timer 直接写 Job 事件流**：独立 Timer Worker 或 Scheduler 扫描 job_waiting（wait_type=timer）且 expires_at <= now，向该 job_id 的事件流 Append wait_completed（payload 含 correlation_key），并 UpdateJobStatus(Pending) + WakeupQueue.NotifyReady(jobID)。
  2. **Timer 通过 Messaging Bus**：Timer 到时后 `SendDelayed` 或等价「投递一条 kind=timer 的消息」到该 Job 所属 agent_id 的 inbox，payload 含 job_id、correlation_key；某处（API 或 Worker）消费该消息时，向对应 job 事件流写 wait_completed 并唤醒 Job。
- **统一表**：定时消息可仅用 agent_messages（scheduled_at），或单独 `timer_events` 表（job_id, agent_id, fire_at, correlation_key, status）；设计文档约定「Timer 到时后要么写 job 事件流 wait_completed，要么写 agent_messages 再经消费写 wait_completed」，与 [agent-process-model.md](agent-process-model.md) 的 Continuation 语义一致。

---

## 7. Webhook 与 External Event

- **Webhook**：外部系统 POST 到 Aetheris 的 Webhook URL（如 `/api/webhooks/agents/:id`），后端解析 body 后 `Send(ctx, "", agentID, payload, &SendOptions{Kind: "webhook"})`，即 from_agent_id 为空。
- **External event**：与 Webhook 同属「来自系统外」的消息；kind 可为 `webhook` 或 `signal`，由实现与产品约定。

---

## 8. 参考

- [agent-instance-model.md](agent-instance-model.md) — AgentInstance、Inbox/Outbox
- [agent-process-model.md](agent-process-model.md) — Mailbox、Signal、agent_message 事件
- [job-state-machine.md](job-state-machine.md) — Job 状态与 wait_completed
