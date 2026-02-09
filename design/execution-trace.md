# Execution Trace 事件模型

基于 JobStore 事件流，为 Agent 执行提供**可解释的执行轨迹**：结构化树、因果链、Replay/Debug 语义。供 Trace API、前端与运维使用，并为 Tool Registry / Policy 审计提供基础。

## 一、事件类型（与 jobstore 一致）

定义见 [internal/runtime/jobstore/event.go](internal/runtime/jobstore/event.go)。

| EventType       | 含义           | 写入方           |
|-----------------|----------------|------------------|
| job_created     | Job 创建       | API（创建 Job 时） |
| plan_generated  | 规划完成，产出 TaskGraph | Runner（Plan 后）  |
| node_started    | 某 DAG 节点开始执行 | NodeEventSink     |
| node_finished   | 某 DAG 节点结束   | NodeEventSink     |
| tool_called     | 工具调用（入参）   | NodeEventSink     |
| tool_returned   | 工具返回（出参）   | NodeEventSink     |
| job_completed   | Job 成功结束    | API/Worker        |
| job_failed      | Job 失败结束    | API/Worker        |
| job_cancelled   | Job 被取消     | Worker            |

## 二、Payload 约定（含可解释因果字段）

所有事件 `payload` 为 JSONB，由各写入方填充。为支持**执行树**与**因果链**，关键事件应包含以下**可选**字段（兼容旧事件，缺失时由 Trace 层按规则推导）：

| 字段             | 类型   | 含义                     | 适用事件                          |
|------------------|--------|--------------------------|-----------------------------------|
| trace_span_id    | string | 本步唯一 span，用于树节点 ID | plan_generated, node_*, tool_*    |
| parent_span_id   | string | 父 span，根为 "root"     | 同上                              |
| step_index       | int    | 事件在流中的 1-based 序号 | 同上（可选，用于 time-travel 锚点） |

### 各事件 payload 约定

- **job_created**：可选 `goal`；无 span（根由 Trace 层虚拟为 "root"）。
- **plan_generated**：`task_graph`, `goal`；建议 `trace_span_id: "plan"`, `parent_span_id: "root"`, `step_index: <version>`。
- **node_started**：`node_id`；建议 `trace_span_id: <node_id>`, `parent_span_id: "plan"`, `step_index: <version>`。
- **node_finished**：`node_id`, `payload_results`；建议同 node_started 的 span（同一 span 的结束），`step_index`。
- **tool_called**：`node_id`, `tool_name`, `input`；建议 `trace_span_id: "<node_id>:tool:<tool_name>:<step_index>"`, `parent_span_id: <node_id>`, `step_index`。
- **tool_returned**：`node_id`, `output`；建议与对应 tool_called 同 span 或配对，`step_index`。
- **job_completed / job_failed / job_cancelled**：可选 `goal`, `error`；可选 `parent_span_id: "plan"` 表示整棵树的结束。

未带上述字段的旧事件仍可参与树推导（见下）。

## 三、执行树推导规则

从 `ListEvents(jobID)` 的有序事件流推导一棵**执行树**，用于「User → Planner → Tool → … → Answer」的可视化与可解释性。

### 树结构

- **根**：虚拟节点，表示 Job（User Message / goal）；id = `"root"`。
- **第一层子节点**：`plan_generated` 对应单节点，id = payload 中的 `trace_span_id` 或默认 `"plan"`，parent = `"root"`。
- **第二层**：每个 `node_started` 对应一个节点，id = `node_id`，parent = `"plan"`（或 payload 中的 `parent_span_id`）。
- **第三层**：同一 `node_id` 下的 `tool_called` / `tool_returned` 作为该节点的子节点；id 可用 `trace_span_id` 或由 `node_id` + `tool_name` + 顺序推导，parent = `node_id`。
- **终态**：`job_completed` / `job_failed` / `job_cancelled` 可作为根或 plan 的「结束」标记，不单独成子节点（或作为根的 result 字段）。

### 推导规则（兼容无 span 字段的旧事件）

1. 按 `version`（事件顺序）扫描；若 payload 含 `trace_span_id` / `parent_span_id`，直接用于建树。
2. 若缺失：`plan_generated` → span_id=`"plan"`, parent=`"root"`；`node_started` → span_id=`node_id`, parent=`"plan"`；`tool_called` → span_id=`"<node_id>:tool:<tool_name>:<version>"`, parent=`node_id`；`node_finished` / `tool_returned` 与同 span 的 start/called 配对，不新建节点，可填充该节点的 end_time / output。
3. 每个树节点可包含：`span_id`, `parent_id`, `type`（plan | node | tool）, `node_id`（若有）, `tool_name`（若有）, `start_time`, `end_time`, `input`, `output`（或 payload 引用）, `step_index`。

## 四、Trace API 返回格式

GET `/api/jobs/:id/trace` 现有字段：

- `job_id`, `timeline`, `node_durations`

扩展后增加（与现有兼容）：

- **execution_tree**：树形结构，根为 `"root"`，每个节点含 `span_id`, `parent_id`, `type`, `node_id`（可选）, `tool_name`（可选）, `start_time`, `end_time`, `step_index`（可选）, `payload_ref`（可选，指向 timeline 中某条或 payload 摘要）。

树结构示例（JSON）：

```json
{
  "job_id": "job-xxx",
  "timeline": [ ... ],
  "node_durations": [ ... ],
  "execution_tree": {
    "span_id": "root",
    "parent_id": null,
    "type": "job",
    "children": [
      {
        "span_id": "plan",
        "parent_id": "root",
        "type": "plan",
        "children": [
          {
            "span_id": "n1",
            "parent_id": "plan",
            "type": "node",
            "node_id": "n1",
            "children": [
              {
                "span_id": "n1:tool:search:3",
                "parent_id": "n1",
                "type": "tool",
                "tool_name": "search",
                "start_time": "...",
                "end_time": "..."
              }
            ]
          }
        ]
      }
    ]
  }
}
```

## 五、Replay / Debug 语义

- **GET /api/jobs/:id/replay**：只读，返回基于事件流的 timeline，不重放执行；已有 `read_only: true`。
- **Time-travel**（可选）：若需「从某 step_index 重放」，由 Runner 在后续迭代支持；1.0 仅保证「按事件流完整回放为树结构」即可。
- **Trace HTML 页**：GET `/api/jobs/:id/trace/page` 可基于 `execution_tree` 渲染「User → Planner → Tool → … → Answer」的可解释视图。

## 六、与 Tool Registry、Policy 的关系

- **审计**：Tool Registry 或 Policy 可基于 `execution_tree` 与 `timeline` 审计「某 Job 调用了哪些工具、顺序与结果」。
- **权限**：后续 Agent Policy 可限制 allowed_tools，Trace 中 tool 节点可标注是否越权（与 Policy 模块对接）。
