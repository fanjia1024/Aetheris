# 毒任务保护（Poison Job / max_attempts）

当某 Job 因 Planner/Tool 永久失败而反复失败时，若不限制重试次数，Worker 会被该任务长期占用或反复调度同一失败任务。本设计明确「达到 max_attempts 后进入终态、不再调度」的语义。

## 语义

- **max_attempts**：单 Job 最多被执行的次数（含首次）。例如 max_attempts=3 表示最多执行 3 次。
- **终态**：Job 状态为 **Failed**（或 Cancelled）后，不再被任何 Worker 或 Scheduler 调度；Claim/ClaimNextPending 只返回 Pending 的 Job。
- **毒任务**：达到 max_attempts 仍失败的 Job 被标记为 Failed，即「毒任务」被隔离，不再占用调度资源。

## API 进程内 Scheduler

- 配置：`agent.job_scheduler.retry_max`（失败后最大重试次数，不含首次）。总尝试次数 = 1 + RetryMax。
- 行为：失败时若 `job.RetryCount < RetryMax` 则 `Requeue`（递增 RetryCount、置为 Pending）；否则 `UpdateStatus(StatusFailed)`，不再调度。
- 见 [internal/agent/job/scheduler.go](internal/agent/job/scheduler.go)。

## Worker 进程（Claim/Heartbeat 模式）

- 配置：`worker.max_attempts`（可选）。总尝试次数上限；默认 3（即最多重试 2 次）。
- 行为：执行失败时，若当前 `RetryCount+1 < max_attempts` 则 `Requeue`（不写 job_failed、不置 Failed），释放租约后该 Job 可再次被 Claim；否则写 job_failed 并 `UpdateStatus(StatusFailed)`，不再被调度。
- 见 [internal/app/worker/app.go](internal/app/worker/app.go) 中 runJob 失败分支。

## 可观测性

- 列表/查询 Job 时可按 `Status == Failed` 过滤，便于排查毒任务。
- 可选后续增强：在 Job 或事件中记录「最后一次错误原因」便于运维。
