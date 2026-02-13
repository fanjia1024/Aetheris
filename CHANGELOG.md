# Changelog

本文档记录 Aetheris 项目的版本与重要变更。格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)。

## [Unreleased]

### Added

- 暂无（2.0.0 已发布，后续增量请记录在此）

### Changed

- 暂无

### Deprecated

- 暂无

### Documentation

- 暂无

---

## [2.0.0] - 2026-02-13

### Added

- **M1 可验证证明链**：
  - 事件哈希链（proof chain）
  - 证据包导出：`POST /api/jobs/:id/export`
  - 离线验证：`aetheris verify <evidence.zip>`
  - Replay/Trace 能力用于确定性复盘
- **M2 合规能力**：
  - 多租户 RBAC（角色与权限控制）
  - 脱敏策略（Redaction）
  - 留存与 Tombstone
  - 审计日志能力
- **M3 取证能力**：
  - Forensics Query（按时间/tool/event 查询）
  - 批量导出与状态轮询
  - Evidence Graph、Audit Log、Consistency Check

### Fixed

- 修复部分历史事件链在导出取证包时的 500 错误：
  - `internal/api/http/forensics.go`
  - 在原链校验失败时执行链归一化重建，确保导出包可验证
- 补充对应单测：
  - `internal/api/http/forensics_test.go`

### Changed

- 发布门禁脚本稳定性增强（`set -u` 场景下 trap 清理安全）：
  - `scripts/release-p0-perf.sh`
  - `scripts/release-p0-drill.sh`
- 故障演练脚本增强：
  - API 重启后 agent 丢失场景支持重建与重试
  - Drill 结果附带更明确的 HTTP code
- 本地 compose 默认 Planner 调整（便于本地 release/perf gate 稳定）：
  - `deployments/compose/docker-compose.yml` 新增 `PLANNER_TYPE=${PLANNER_TYPE:-rule}`

### Verification

- `RUN_P0_PERF=1 RUN_P0_DRILLS=1 PERF_SAMPLES=3 PERF_POLL_MAX=45 ./scripts/release-2.0.sh` 通过
- 性能基线：`artifacts/release/perf-baseline-2.0-20260213-172044.md`
- 故障演练：`artifacts/release/failure-drill-2.0-20260213-172051.md`（passed=4 failed=0 skipped=1）

---

## 历史版本（摘要）

以下为早期提交对应的功能摘要，未按语义化版本打 tag 时可按提交顺序参考。

- **refactor: update planner integration for v1 Agent API** — planGoaler 接口、RulePlanner、PLANNER_TYPE 环境变量
- **feat: implement v1 Agent API and enhance session management** — v1 Agent 端点、Manager/Scheduler/Creator、Session 管理
- **feat: refactor agent execution to support session management and enhance planning** — Session 感知执行、Planner 单步决策、SchemaProvider
- **feat: add agent execution endpoint and integrate agent runner** — `/api/agent/run`、AgentRunner、Session 管理
- **feat: implement gRPC support and JWT authentication** — gRPC 服务、JWT 中间件、文档/查询 gRPC 方法
- **feat: integrate OpenTelemetry for tracing** — 链路追踪与文档处理增强
- **feat: enhance API configuration and workflow execution** — API 配置与工作流执行
- **refactor: migrate from Gin to Hertz** — HTTP 框架由 Gin 迁移至 Hertz
- **feat: 初始化RAG/Agent平台核心组件和架构** — 初始 RAG/Agent 平台骨架
