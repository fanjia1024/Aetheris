# Durable Memory Layer 数据模型

本文档定义 Aetheris 2.0 的 **持久化 Memory 层** 数据模型与接口约定，包括 short-term working memory、long-term memory、episodic memory（history abstraction），以及 **memory snapshot binding on resume**。与现有 [internal/agent/runtime/state.go](../internal/agent/runtime/state.go) 中的 Session/AgentState 映射，与 [agent-process-model.md](agent-process-model.md) 的 Resumption Context 对接。

---

## 1. 目标

- **认知连续性（cognitive continuity）**：Agent 跨 Job、跨会话恢复时，不仅恢复执行游标，还恢复「记忆」上下文。
- **Working Memory**：当前会话的对话、变量、工具调用、scratchpad；与现有 Session/AgentState 对应，显式命名为工作记忆。
- **Long-Term Memory**：跨会话持久化的事实/知识，可被多轮对话与多 Job 引用。
- **Episodic Memory**：按事件/会话片段的抽象（如「某次 Job 的摘要」「某段对话的结论」），供检索与上下文组装。
- **Memory Snapshot on Resume**：在 Checkpoint 或 job_waiting 的 resumption_context 中绑定 memory snapshot；恢复时 Load 并 Apply，保证等待后「同一思维」继续。

---

## 2. 逻辑分层

### 2.1 Working Memory（工作记忆）

- **语义**：当前会话内的对话历史、变量、工具调用记录、scratchpad、当前任务、LastCheckpoint；生命周期与 Session 绑定，单次运行中可读写。
- **与现有映射**：即现有 **AgentState**（agent_id, session_id, Messages, Variables, ToolCalls, Scratchpad, LastCheckpoint）；存储于 `agent_states` 表，由 AgentStateStore 持久化。
- **扩展（可选）**：在 agent_states 的 payload 或 meta 中增加 `working_memory_snapshot` 版本号或摘要，用于 Resume 时校验一致性。

**接口**：与现有 **AgentStateStore** 对齐即可：SaveAgentState、LoadAgentState。设计文档中显式约定「AgentState = Working Memory 的持久化形态」。

### 2.2 Long-Term Memory（长期记忆）

- **语义**：跨会话、持久化的事实/知识；Agent 可写入（如「用户偏好 X」）、读取（如 Recall 查询）；与 RAG 知识库区分：RAG 面向文档检索，Long-Term Memory 面向 Agent 自身积累的键值或向量化记忆。
- **存储建议**：新表 `agent_long_term_memory`：

| 字段 | 类型 | 说明 |
|------|------|------|
| agent_id | string | 归属 Agent |
| namespace | string | 可选命名空间（如 "preferences", "facts"） |
| key | string | 键；同一 agent_id + namespace 下唯一 |
| value | JSONB | 值 |
| updated_at | timestamp | 最后更新时间 |

或按 embedding 的 vector store 单独设计（memory_id, agent_id, embedding, text, metadata），由实现选择。

**接口约定**：

```go
type LongTermMemoryStore interface {
    Get(ctx context.Context, agentID, namespace, key string) (value []byte, err error)
    Set(ctx context.Context, agentID, namespace, key string, value []byte) error
    ListByAgent(ctx context.Context, agentID string, namespace string, limit int) ([]KeyValue, error)
}
```

### 2.3 Episodic Memory（情景记忆）

- **语义**：按「事件/会话片段」的抽象，如「某次 Job 完成后的摘要」「某段对话的结论」；供后续 Job 检索（如「上次我们讨论过 X」）或 Trace/审计。
- **存储建议**：新表 `agent_episodic_chunks`：

| 字段 | 类型 | 说明 |
|------|------|------|
| id | string | 唯一 ID |
| agent_id | string | 归属 Agent |
| session_id | string | 可选，关联会话 |
| job_id | string | 可选，关联 Job |
| summary | string | 摘要（用于检索或展示） |
| payload | JSONB | 详细内容或引用 |
| created_at | timestamp | 创建时间 |

或由事件流推导：在 JobCompleted 等时机生成 episode 摘要并写入此表；实现可选「仅事件流」推导，不落物理表。

**接口约定**：

```go
type EpisodicEntry struct {
    ID         string
    AgentID    string
    SessionID  string
    JobID      string
    Summary    string
    Payload    map[string]any
    CreatedAt  time.Time
}

type EpisodicMemoryStore interface {
    Append(ctx context.Context, entry *EpisodicEntry) error
    ListByAgent(ctx context.Context, agentID string, limit int) ([]*EpisodicEntry, error)
    ListBySession(ctx context.Context, agentID, sessionID string, limit int) ([]*EpisodicEntry, error)
}
```

---

## 3. Memory Snapshot on Resume

### 3.1 目的

在 Wait 节点挂起或 Checkpoint 恢复时，除 payload_results、cursor_node、plan_decision_id 外，将「当前记忆视图」一并快照，恢复时先 Load Memory Snapshot 再 Apply 到 Working Memory（及可选注入 Long-Term/Episodic 的最近上下文），保证**思维连续性**。

### 3.2 MemorySnapshot 结构（设计约定）

```go
type MemorySnapshot struct {
    WorkingMemory   []byte          // AgentState 或等价 JSON
    EpisodicTail   []EpisodicEntry // 最近 N 条 episodic，供上下文
    LongTermKeys   []string        // 可选：本次执行引用的 long-term key 列表，恢复时预加载
    SnapshotAt     time.Time
}
```

- **WorkingMemory**：即 SessionToAgentState(session) 的序列化；Resume 时 ApplyAgentState(session, state)。
- **EpisodicTail**：最近若干条 episodic 条目，Resume 时可供 Planner 或 Recall 使用。
- **LongTermKeys**：可选；若执行中曾读取某些 long-term key，可记录以便恢复时预加载到上下文。

### 3.3 写入时机

- **job_waiting 的 resumption_context**：在现有 resumption_context（payload_results、plan_decision_id、cursor_node）中增加 **memory_snapshot** 字段，内联或引用 MemorySnapshot。
- **Checkpoint**：可选在 Checkpoint 中增加 memory_snapshot_id 或内联 working_memory 摘要；恢复时若存在则 LoadMemorySnapshot 再 Apply。

### 3.4 加载与恢复流程

1. Worker Claim Job 后，若从 Replay 或 Checkpoint 恢复，先取 resumption_context / checkpoint 中的 memory_snapshot。
2. 若存在 MemorySnapshot.WorkingMemory，反序列化为 AgentState，ApplyAgentState(session, state)。
3. 若存在 EpisodicTail，可注入到 Session 的上下文或单独传给 Planner/Recall。
4. 若存在 LongTermKeys，从 LongTermMemoryStore 预加载并注入上下文。
5. 再按现有逻辑从 cursor_node 继续执行。

---

## 4. 与现有 Session/AgentState 的映射

| 概念 | 现有实现 | 本层约定 |
|------|----------|----------|
| Working Memory | Session + AgentState + agent_states 表 | AgentState = Working Memory 持久化；Session = 运行时工作记忆载体 |
| Long-Term Memory | 无 | 新表 agent_long_term_memory 或 vector store |
| Episodic Memory | 无 | 新表 agent_episodic_chunks 或由事件流推导 |
| Resume 上下文 | resumption_context 含 payload_results、plan_decision_id、cursor_node | 扩展 resumption_context 含 memory_snapshot（Working + 可选 EpisodicTail + 可选 LongTermKeys） |

现有 AgentStateStore 无需改接口；LongTermMemoryStore、EpisodicMemoryStore 为新增；Runner 在写 job_waiting 与 Checkpoint 时构造并写入 MemorySnapshot，恢复时读取并 Apply。

---

## 5. RAG 与 Long-Term Memory 的边界

- **RAG**：面向「知识库」的检索（文档、chunk、embedding），通常由 Pipeline 或 RAG 节点调用；数据源为知识库、非 Agent 私有。
- **Long-Term Memory**：面向「Agent 私有」的持久化键值或向量；由 Agent 在运行中写入（如「用户说喜欢 X」）、在后续 Job 中读取；与 RAG 可并存，RAG 检索「公开知识」，Long-Term 存储「该 Agent 的认知积累」。
- **实现边界**：RAG 检索接口与 LongTermMemoryStore 分离；若需「混合检索」（RAG + Agent 自有记忆），由上层或 Planner 组合两者结果。

---

## 6. 参考

- [agent-process-model.md](agent-process-model.md) — Resumption Context、Continuation Semantics
- [agent-instance-model.md](agent-instance-model.md) — AgentInstance 与 Session 归属
- [internal/agent/runtime/state.go](../internal/agent/runtime/state.go) — AgentState、AgentStateStore、SessionToAgentState、ApplyAgentState
