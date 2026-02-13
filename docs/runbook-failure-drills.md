# Aetheris 2.0 Failure Drill Runbook

## 1. 目标

在发布前验证系统面对常见故障时的可恢复性与可观测性，确保值班团队有标准处置路径。

## 2. 演练频率

- 发布前: 必做一次全量演练
- 生产期: 每月至少一次抽样演练

## 3. 演练准备

- 演练环境: staging（与生产拓扑尽量一致）
- 观察面板: 错误率、队列堆积、Worker 状态、Postgres 指标
- 回滚预案: 已准备上一版本镜像/二进制

## 4. P0 演练项

### Drill A: Worker 进程崩溃恢复

步骤:
1. 提交一个长任务或等待 signal 的任务
2. 手动停止 worker 进程
3. 重启 worker
4. 观察 job 是否继续执行并落到终态

通过标准:
- job 不丢失
- replay/trace 可追溯

### Drill B: API 重启

步骤:
1. 持续发送 message 请求
2. 重启 API
3. 验证 API 恢复后请求成功率

通过标准:
- API 在预期时间内恢复
- 已提交 job 状态仍可查询

### Drill C: Postgres 短时不可用

步骤:
1. 短时阻断 API/Worker 到 Postgres 的连接
2. 恢复连接
3. 观察系统是否恢复处理

通过标准:
- 服务不出现不可恢复错误
- 连接恢复后系统可继续处理新旧任务

### Drill D: Signal/Replay 正确性

步骤:
1. 创建 wait/signal 任务
2. 发送 signal 推进任务
3. 调用 replay 与 trace 接口确认事件序列

通过标准:
- signal 生效且幂等
- replay 输出与事件流一致

### Drill E: Forensics 可用性

步骤:
1. 导出证据包
2. 本地 verify
3. 调一致性接口

通过标准:
- 导出成功
- verify 通过
- 一致性检查通过

## 5. 失败分级

- Sev-1: 数据丢失、任务不可恢复
- Sev-2: 核心链路失败率高，需人工干预
- Sev-3: 功能可恢复但指标退化

任一 Sev-1 阻断发布。

## 6. 演练记录模板

每次演练记录:
- 演练时间/负责人
- 环境与版本
- 演练项与步骤
- 结果（通过/失败）
- 发现问题与修复 owner
- 复验时间

## 7. 关联文档

- `docs/release-checklist-2.0.md`
- `docs/observability.md`
- `docs/runtime-guarantees.md`
