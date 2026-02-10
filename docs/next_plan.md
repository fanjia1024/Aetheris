很好，这一步其实决定 **Aetheris 能不能从“一个很强的 demo”变成“一个真正的基础设施项目”**。
下面这份是按**你一个人开发的现实速度**排的，不是公司路线图，是 **可落地的 8 周 coding 计划**（每周你应该具体写哪些包、哪些文件、完成什么验收）。

> 原则：2.0 不做“多功能”，只做一件事 —— **让 Agent 执行具备可恢复的事务语义**
> （也就是：崩溃 / 重试 / worker 换机器 / LLM 不稳定 → 都不破坏执行历史）

---

# Aetheris 2.0 八周开发路线

先给你总览：

| 周     | 主题                       | 结果                      |
| ------ | -------------------------- | ------------------------- |
| Week 1 | Replay 基础设施            | 能从事件流重建执行状态    |
| Week 2 | Sandbox + SideEffect       | LLM/Tool 可重放           |
| Week 3 | Activity（外部调用事务化） | 防重复副作用              |
| Week 4 | Checkpoint Barrier         | 真正“从中间恢复”          |
| Week 5 | Scheduler 2.0              | 分布式执行稳定            |
| Week 6 | Worker 能力调度            | 多 agent / 多模型基础     |
| Week 7 | Execution Trace UI         | 可调试（极关键）          |
| Week 8 | Agent SDK                  | 外部开发者可用 + 发布 2.0 |

下面是你每天应该写什么级别的细化。

---

## Week 1 — Execution History & Replay（地基）

目标：

> Runner 不再直接依赖 JobStore，而是依赖“历史”。

你现在的系统：
Runner = 执行器

2.0：
Runner = 状态机恢复器

### 新增包

```
internal/history/
internal/replay/
```

### 要写的核心文件

**Day 1**

```
internal/history/history.go
```

```go
type History interface {
    Next() (*Event, error)
    Peek() (*Event, error)
    IsReplay() bool
}
```

**Day 2**

```
internal/history/job_history.go
```

实现：

- 从 JobStore 拉事件
- 按顺序迭代
- 可回放

验收：

> 能把一条 Job 的 event 流完整打印出来。

---

**Day 3–4**
修改 Runner：

```
internal/runner/runner.go
```

原来：

```
Runner -> JobStore
```

改为：

```
Runner -> History
```

验收标准：

你可以：

1. 跑一个 job
2. 关掉进程
3. 重启
4. Runner 从中间继续

（这一步就已经很大突破）

---

**Day 5**
实现 replay 模式：

```
internal/replay/replayer.go
```

功能：

- 如果事件存在 → 不执行
- 只恢复状态

验收：

> 重启不会重新跑 planner。

---

## Week 2 — Sandbox & Deterministic Side Effects（核心质变）

目标：

> LLM、时间、随机数、工具 调用全部可重放。

### 新增包

```
internal/sandbox/
```

### Day 1 — 定义模式

```
sandbox.go
```

```go
type Mode int
const (
    Execute Mode = iota
    Replay
)
```

---

### Day 2 — SideEffect API（最关键）

```go
func (s *Sandbox) SideEffect(
    name string,
    fn func() ([]byte, error),
) ([]byte, error)
```

逻辑：

| 模式    | 行为         |
| ------- | ------------ |
| Execute | 执行并写事件 |
| Replay  | 读取事件     |

新增事件：

```
SideEffectRecorded
```

---

### Day 3–4 — 接管 LLM 调用

改：

```
planner/llm.go
tools/llm_tool.go
```

从：

```
直接调用模型
```

改为：

```
sandbox.SideEffect("llm.call", ...)
```

验收：

**重启后 LLM 不会再次调用。**

（这是 Aetheris 第一次具备“确定性执行”）

---

### Day 5 — 时间 & 随机数

新增：

```
sandbox.Now()
sandbox.Random()
```

全部事件化。

---

## Week 3 — Activity（外部副作用事务化）

目标：

> 防止重复调用外部 API（最重要能力之一）

新增包：

```
internal/activity/
```

### 新事件

```
ActivityScheduled
ActivityStarted
ActivityCompleted
ActivityFailed
```

---

### Day 1

定义：

```
activity.go
```

```go
type Activity interface {
    Execute(ctx context.Context, input []byte) ([]byte, error)
}
```

---

### Day 2–3

改 Tool 执行流程：

原来：

```
runner -> tool
```

2.0：

```
runner -> schedule activity -> worker 执行
```

---

### Day 4

worker 写：

```
ActivityStarted
ActivityCompleted
```

---

### Day 5 验收（关键）

你要测试：

1. tool 调用外部 API
2. 成功
3. 强制 kill worker
4. 重启

预期：

> 外部 API 不会再次被调用

如果通过，Aetheris 进入“生产可用”阶段。

---

## Week 4 — Checkpoint Barrier（真正恢复）

目标：

> DAG 从节点恢复，而不是从 Job 开头。

新增：

```
internal/checkpoint/
```

新增事件：

```
StepCommitted
```

### 实现

每个 TaskNode 完成：

```
写 StepCommitted
才允许调度下一个节点
```

验收：

- DAG 执行到第 4 个节点
- kill 所有 worker
- 重启
- 从第 5 个节点继续

这一步完成：

你真正拥有 **workflow runtime**。

---

## Week 5 — Scheduler 2.0

当前 scheduler 是 polling。
要升级为真正调度器。

新增：

```
internal/scheduler/queue/
```

实现：

- 多队列
- 优先级
- retry backoff

验收：

同时跑 20 个 job，系统不乱序、不阻塞。

---

## Week 6 — Worker 能力匹配

Worker 注册能力：

```json
{
  "capabilities": ["llm", "tool", "rag"]
}
```

Scheduler 按能力派发。

这一步是多 Agent 的前置条件。

---

## Week 7 — Execution Trace UI（极重要）

新增：

```
cmd/aetheris-web/
```

只做 3 页：

1. Job Timeline（事件流）
2. DAG Graph
3. Tool/LLM 调用详情

你已经有 EventStore —— 其实就是“时序数据库”。

这会让 Aetheris 从：

> 黑盒系统 → 可调试系统

（非常关键，甚至比多 agent 更重要）

---

## Week 8 — Agent SDK + 发布 2.0

新增：

```
pkg/agent/
```

目标：隐藏 Job/Runner/Planner。

用户使用：

```go
agent := aetheris.NewAgent("researcher")

agent.Tool(webSearch)
agent.Tool(codeExec)

agent.Run(ctx, "analyze postgres vs mysql performance")
```

最后一周只做三件事：

1. example
2. 文档
3. 发布 v2.0

---

# 当你完成这 8 周

Aetheris 的定位将变为：

**Temporal × LangChain × Durable Execution Runtime**

而不是“一个 Agent 框架”。

它会成为一个类别：

> Agent Infrastructure（目前这个领域几乎是空的）

---

如果你愿意，下一步我可以帮你写一份
**Aetheris 2.0 发布时的 Hacker News / Reddit 技术发布帖文案**（这个其实非常重要，决定是否有人开始关注你的项目）。
