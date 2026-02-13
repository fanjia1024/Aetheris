# Aetheris 2.0 Performance Baseline

## 1. 目标

定义 2.0 发布前必须具备的最小性能基线，避免“功能可用但不可运营”。

## 2. 测试前提

- Go: `1.25.7+`
- 存储: PostgreSQL（与生产同版本）
- 测试对象: API + Worker + JobStore
- 测试流量: 混合场景（短任务、工具调用、含 signal/wait 的任务）

## 3. 必测场景

1. 基础提交链路: `POST /api/agents/:id/message` -> Job 完成
2. 查询链路: `GET /api/jobs/:id` 与 `GET /api/jobs/:id/events`
3. 取证链路: `POST /api/jobs/:id/export` + 离线 verify
4. 可观测链路: `GET /api/jobs/:id/trace` 与 Trace 页面
5. 恢复链路: Worker 重启后 job 可继续

## 4. P0 基线阈值（建议作为发布门禁）

在标准环境（2C4G 单 Worker，Postgres 独立实例）下，建议满足:

- 吞吐:
  - 简单任务: `>= 10 jobs/min`
  - 复杂任务: `>= 2 jobs/min`
- 时延:
  - `POST /api/agents/:id/message` P95 `<= 500ms`（仅提交确认）
  - `GET /api/jobs/:id` P95 `<= 200ms`
  - `GET /api/jobs/:id/events` P95 `<= 500ms`
- 稳定性:
  - 30 分钟压测无 panic
  - job 丢失率 `= 0`
  - 证据 verify 失败率 `< 0.1%`

说明: 以上阈值用于发布准入，不代表最终容量上限。容量规划见 `docs/capacity-planning.md`。

## 5. 采集指标

至少采集:
- API QPS、P50/P95/P99
- Worker busy ratio
- Queue backlog
- PostgreSQL CPU/连接数/慢查询
- Export 与 verify 成功率

## 6. 执行建议

1. 先跑冒烟（10 分钟）
2. 再跑稳态（30 分钟）
3. 最后跑重启恢复（手动 kill/restart worker）

## 7. 报告模板

发布前输出一份基线报告，建议包含:
- 测试环境（版本、规格、配置）
- 场景与请求量
- 指标结果（P50/P95/P99、失败率）
- 异常与结论（是否满足 P0 基线）

## 8. 关联文档

- `docs/capacity-planning.md`
- `docs/release-checklist-2.0.md`
- `docs/observability.md`
