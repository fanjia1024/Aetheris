# Capability Permission Model — 能力与审批策略

Runtime 作为**安全边界**：按能力（读/写/发邮件/付款/自定义）配置策略，在执行 Tool 前校验；不满足则阻塞或触发审批流。与 Wait/Signal/Mailbox 配合可实现「需人工确认后继续」。

## 目标

- **能力边界**：工具或操作绑定 Capability（如 `write_db`、`send_email`、`payment`）；Policy 按能力或按工具配置 allow / require_approval / require_human / deny。
- **执行前校验**：Runner/Adapter 在调用 `Tools.Execute` 前解析所需 capability，调用 PolicyChecker；deny 则直接失败该步；require_approval/require_human 则写入 job_waiting，待人工 Signal 后继续。
- **与 Mailbox 配合**：审批通过后通过 POST `/api/jobs/:id/signal`（或 message）写入 wait_completed，Job 重新入队，该步再次执行时 PolicyChecker 根据「已批准 correlation_key」返回 allowed。

## 能力与策略

| 策略 | 含义 |
|------|------|
| **allow** | 直接允许执行 |
| **require_approval** | 需先进入 job_waiting，由人工/系统 Signal 后再次执行时视为已批准 |
| **require_human** | 同 require_approval，语义上强调必须人工确认 |
| **deny** | 拒绝执行，该步永久失败 |

- **Capability**：与工具绑定，可由工具 metadata 标注（如 `capability: ["write_db"]`）或按工具名映射。
- **执行前校验**：Adapter 在 `Tools.Execute` 前根据 job/agent/step 解析所需 capability，调用 `CapabilityPolicyChecker.Check`；根据返回值决定执行、等待审批或失败。

## 接口

- **CapabilityPolicyChecker**：`Check(ctx, jobID, toolName, capability, idempotencyKey string, approvedKeys map[string]struct{}) (allowed bool, requiredApproval bool, err error)`  
  - `approvedKeys`：来自事件流中 wait_completed 的 correlation_key 集合，表示已批准的等待。  
  - 若该步的审批 key（如 `cap-approval-`+idempotencyKey）∈ approvedKeys，则返回 (true, false, nil)；否则若策略为 require_approval 则返回 (false, true, nil)，Adapter 返回 `CapabilityRequiresApproval`，Runner 写 job_waiting 并返回 ErrJobWaiting。
- **配置**：tool → capabilities，capability → policy；可来自配置文件或 DB。

## 与 Wait/Signal 的配合

1. 某步 Tool 需审批：Adapter 调用 Check，得到 requiredApproval，返回 `CapabilityRequiresApproval{CorrelationKey: "cap-approval-"+idempotencyKey}`。
2. Runner 捕获该错误，写入 job_waiting（wait_type=signal，correlation_key=该 key），UpdateStatus Waiting，返回 ErrJobWaiting。
3. 人工或系统调用 POST `/api/jobs/:id/signal`，body 含相同 correlation_key 与可选 payload。
4. 写入 wait_completed，Job 置为 Pending，Worker 认领后 Replay；ReplayContext 包含该 correlation_key 的 wait_completed，approvedKeys 含该 key。
5. 再次执行该步时，Check(ctx, ..., approvedKeys) 发现 key 已批准，返回 (true, false, nil)，Adapter 正常执行 Tool。

## 实现位置

- **接口与默认实现**：[internal/agent/runtime/executor/capability_policy.go](../internal/agent/runtime/executor/capability_policy.go)
- **执行前校验**：[internal/agent/runtime/executor/node_adapter.go](../internal/agent/runtime/executor/node_adapter.go) Tool 执行路径前插入 Check；Runner 对 CapabilityRequiresApproval 写 job_waiting 并返回 ErrJobWaiting。
- **配置**：可选；默认 PolicyChecker 为 allow-all，不配置时行为与现有一致。

## 安全表述

Aetheris 作为 **安全边界**，支持可配置的工具能力与审批策略，满足金融/审批/合同等场景的「读数据可、写库需审批、付款必须人工确认」等需求。
