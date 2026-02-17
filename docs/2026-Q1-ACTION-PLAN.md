# Aetheris Q1 2026 Action Plan

## 当前状态（2026-02-12）

### ✅ 已完成并可用

**Runtime Correctness (1.0+)**:
- At-most-once execution
- Tool ledger + Confirmation replay
- Distributed worker + Lease fencing
- Event sourcing + PostgreSQL
- Trace UI + CLI tools

**Operational Foundations (2.0)**:
- Rate Limiter（已集成到 tool 执行路径）
- Event Snapshot/Compaction（接口完整，待 worker 应用）
- Tenant isolation（schema 完整，待 API 应用）
- Basic RBAC（权限模型完整）
- Evidence export（proof 包完整，待集成）
- Metrics + Alerting（规则完整）

### 🔬 原型/Future Work（3.0）

已有设计框架，暂不作为当前 focus：
- Evidence Graph UI 渲染
- 复杂 Forensics Query
- 完整脱敏策略引擎
- 数字签名
- 分布式 Ledger
- AI 异常检测
- 合规模板自动应用

---

## Q1 具体行动（未来 8 周）

### Week 1-2: Rate Limiting 验证

**目标**: 确保 rate limiting 在生产环境正确工作

**任务**:
1. 补充 rate limiter 的配置加载（从 yaml）
2. 在 Worker bootstrap 时初始化 rate limiter
3. 传递给 ToolNodeAdapter
4. 集成测试：并发 tool 调用，验证限流生效
5. Metrics 验证：`rate_limit_wait_seconds` 正确上报

**验收**: 配置 qps=10 的 tool，并发调用 100 次，实际 QPS ≈ 10

### Week 3-4: Tenant Isolation 完整

**目标**: 所有 API 按 tenant 隔离

**任务**:
1. Job 创建时自动绑定 tenant_id（从 JWT 提取）
2. 所有查询 API 添加 tenant filter
3. 跨 tenant 访问返回 403
4. 集成测试：创建 2 个 tenant，验证数据隔离
5. 审计日志记录 tenant 访问

**验收**: Tenant A 无法访问 Tenant B 的任何资源

### Week 5-6: Snapshot Automation

**目标**: 长跑 job 不会导致性能退化

**任务**:
1. Worker 启动定时任务（每小时扫描）
2. 检测超过 1000 事件的 jobs
3. 自动创建 snapshot
4. ReplayContextBuilder 优先使用 snapshot
5. 性能测试：10000 events 的 job replay < 1s

**验收**: 创建 5000 events 的 job，replay 时间不超过 1 秒

### Week 7-8: Storage GC + Evidence Export

**目标**: 存储可持续增长 + 基础审计能力

**任务**:
1. 实现 tool_invocations 的 TTL 清理（90 天）
2. Worker GC 定时任务
3. Evidence export 连接真实 ToolInvocationStore
4. CLI export/verify 端到端测试
5. 文档更新

**验收**: 
- 90 天前的 tool invocations 自动清理
- `aetheris export <job_id>` 成功导出并可验证

---

## Q1 结束时的目标状态

### 功能

- [ ] Rate limiting 在生产环境正确工作
- [ ] 10+ tenants 数据完全隔离
- [ ] 长跑 jobs（10000+ events）性能稳定
- [ ] 存储增长可控（自动 GC）
- [ ] 任意 job 可导出 evidence zip 并离线验证

### 质量

- [ ] 核心路径测试覆盖 > 80%
- [ ] 生产环境运行 1 周无崩溃
- [ ] 关键 metrics 和 alerts 验证有效

### 文档

- [ ] Deployment guide完整
- [ ] Operations manual完整
- [ ] Troubleshooting guide完整

---

## 不做清单（Q1）

明确**不在 Q1 做**的事情：

❌ Evidence Graph UI 渲染（API 保留）
❌ 复杂 Forensics Query（基础过滤即可）
❌ 完整脱敏策略（redact 模式足够）
❌ 数字签名集成
❌ 分布式 Ledger 同步
❌ AI 异常检测模型
❌ 实时质量评分计算
❌ 合规模板自动应用

这些是 Q2-Q4 或 3.0 的事情。

---

## Success Metrics (Q1)

| 指标 | 目标 | 当前 |
|------|------|------|
| Concurrent jobs | 100+ | TBD |
| Job throughput | 10-20/min | TBD |
| P95 latency | < 5s | TBD |
| Storage growth | < 10GB/month | TBD |
| Uptime | > 99% | TBD |
| Test coverage | > 80% | ~70% |

---

## Q2 Preview

**Focus**: 分布式执行成熟

- Job sharding
- Worker capability routing
- Graceful shutdown
- Backpressure
- OpenTelemetry 完整
- Performance optimization

---

**Current Phase**: Q1 - Operational Runtime  
**Status**: Integration in progress  
**Next Milestone**: Q1 End (2026-03-31)
