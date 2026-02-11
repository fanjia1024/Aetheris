# P0: Effect Isolation Layer - 实现计划

## 目标

建立 **确定性边界**：
- 所有非确定性行为必须通过 `runtime.ExecuteEffect()` 进入系统
- Effect 结果自动写入事件流
- Replay 时自动注入历史结果，禁止真实调用

## 核心设计

### 1. Effect 类型定义

```go
package effects

type Kind string

const (
    KindLLM       Kind = "llm"
    KindTool      Kind = "tool"
    KindHTTP      Kind = "http"
    KindTime      Kind = "time"
    KindRandom    Kind = "random"
    KindSleep     Kind = "sleep"
)

// Effect 代表一次副作用调用
type Effect struct {
    ID             string              // 唯一标识，供幂等性校验
    Kind           Kind                // 效应类型
    Payload        any                 // 调用参数（LLMRequest, ToolCall, etc.）
    IdempotencyKey string              // 幂等性 key（Tool/HTTP 必填）
    Description    string              // 供调试/trace
}

// Result 代表 Effect 执行结果
type Result struct {
    ID        string
    Kind      Kind
    Data      any
    Error     error
    Timestamp time.Time
    Duration  time.Duration
}
```

### 2. Effect System 接口

```go
package effects

// System 定义 Effect 执行系统
type System interface {
    // Execute 执行 Effect，必须通过此入口
    Execute(ctx context.Context, effect Effect) (Result, error)

    // MustExecute 执行并记录到事件流
    MustExecute(ctx context.Context, effect Effect, jobID string, version int) (Result, error)

    // Replay 从事件流重建结果（Replay 模式用）
    Replay(ctx context.Context, effectID string) (Result, bool)

    // IsReplaying 是否处于 Replay 模式
    IsReplaying(ctx context.Context) bool
}
```

### 3. 事件流中的 Effect 记录

| EffectKind | EventType | Payload 字段 |
|------------|-----------|--------------|
| LLM | `llm_invocation_started` + `llm_response_recorded` | prompt_tokens, completion_tokens, model, result |
| Tool | `tool_invocation_started` + `tool_invocation_finished` | tool_name, input, output, idempotency_key |
| HTTP | `http_request_started` + `http_response_recorded` | method, url, body, status_code, response |
| Time | `time_recorded` | timestamp, timezone |
| Random | `random_recorded` | source, values |
| Sleep | `sleep_completed` | duration |

## 实现步骤

### Phase 1: 核心 Effect 类型定义

**文件**：
- `pkg/effects/kind.go` - EffectKind 类型定义
- `pkg/effects/effect.go` - Effect/Result 结构体
- `pkg/effects/errors.go` - 错误定义
- `pkg/effects/context.go` - context key 定义（replaying, effect_id）

**产出**：
- Effect, Result 结构体
- Kind 常量
- Context helper（WithReplaying, WithEffectID）

### Phase 2: Effect System 接口与内存实现

**文件**：
- `pkg/effects/system.go` - System 接口
- `pkg/effects/memory.go` - 内存实现（供测试用）

**产出**：
- `NewMemorySystem()` - 内存实现
- `Execute()` 默认实现：执行并记录
- `Replay()` - 从内存历史恢复

### Phase 3: 迁移 LLM 调用

**文件**：
- `pkg/effects/llm.go` - LLM Effect 实现
- `internal/model/llm/effect.go` - Adapter

**改动**：
```go
// 之前
response, err := llm.Generate(ctx, req)

// 之后
result, err := effects.System.Execute(ctx, effects.Effect{
    Kind:  effects.KindLLM,
    Payload: req,
    IdempotencyKey: req.String(), // 或使用 prompt hash
    Description: "llm.generate",
})
response = result.Data.(llm.Response)
```

**需要**：
- 为每个 LLM Provider 实现 Effects 包装
- 确保 event 记录 `llm_invocation_started` + `llm_response_recorded`

### Phase 4: 迁移 Tool 调用

**文件**：
- `pkg/effects/tool.go` - Tool Effect 实现
- `internal/tool/builtin/effect.go` - Adapter

**改动**：
```go
// 之前
output, err := tool.Execute(ctx, name, args)

// 之后
result, err := effects.System.MustExecute(ctx, effects.Effect{
    Kind:  effects.KindTool,
    Payload: toolCall,
    IdempotencyKey: buildToolKey(name, args), // 关键！
    Description: "tool." + name,
}, jobID, version)
output = result.Data
```

**关键**：
- `tool_invocation_started` **先于** 实际执行写入事件流
- 执行完成后写入 `tool_invocation_finished`
- Replay 时检测 `pending_started` 状态，禁止重复执行

### Phase 5: HTTP/Sleep/Time/Random Effect

**文件**：
- `pkg/effects/http.go`
- `pkg/effects/time.go`
- `pkg/effects/sleep.go`
- `pkg/effects/random.go`

**实现**：
- `Time.Now()` → `effects.KindTime`
- `rand.Int63()` → `effects.KindRandom`
- `time.Sleep()` → `effects.KindSleep`
- `http.Do()` → `effects.KindHTTP`

### Phase 6: Replay 集成

**改动**：
- `Execute()` 检测 `ctx.Replaying()`
- 若为 Replay 模式，调用 `Replay(effectID)` 注入历史结果
- 禁止真实执行任何 Effect

### Phase 7: 工厂函数与默认实例

**文件**：
- `pkg/effects/default.go` - 全局默认 System

```go
var DefaultSystem System = NewMemorySystem()

// 全局入口
func Execute(ctx context.Context, effect Effect) (Result, error) {
    return DefaultSystem.Execute(ctx, effect)
}

// 注入自定义实现（测试时用 mock）
func RegisterSystem(s System) {
    DefaultSystem = s
}
```

## 事件流 Schema 扩展

### LLM Response Recorded

```json
{
  "type": "llm_response_recorded",
  "payload": {
    "effect_id": "uuid",
    "model": "gpt-4",
    "prompt_tokens": 120,
    "completion_tokens": 350,
    "result": {
      "content": "...",
      "stop_reason": "stop"
    },
    "duration_ms": 1500,
    "cached": false
  }
}
```

### Tool Invocation Finished（幂等性）

```json
{
  "type": "tool_invocation_finished",
  "payload": {
    "effect_id": "uuid",
    "idempotency_key": "sha256(tool_name+args)",
    "tool_name": "http_get",
    "input": {...},
    "output": {...},
    "cached": true
  }
}
```

## 测试策略

### T1: Effect 执行记录测试

```go
func TestEffectRecordsToEventStream(t *testing.T) {
    sys := NewMemorySystem()
    result, err := sys.Execute(ctx, Effect{
        Kind: KindTool,
        Payload: req,
        IdempotencyKey: "key",
    })
    assert.NoError(t, err)
    assert.Len(t, sys.History(), 1)
    assert.Equal(t, result.ID, sys.History()[0].ID)
}
```

### T2: Replay 不触发真实调用

```go
func TestReplayDoesNotCallExternal(t *testing.T) {
    sys := NewMemorySystem()
    // 第一次执行
    _, _ = sys.Execute(ctx, Effect{Kind: KindLLM, ...})

    // Replay 模式
    ctx := WithReplaying(ctx, true)
    called := false
    sys = NewMemorySystem(func(k Kind, p any) (Result, error) {
        called = true
        return Result{}, errors.New("should not be called")
    })

    _, _ = sys.Execute(ctx, Effect{Kind: KindLLM, ...})
    assert.False(t, called)
}
```

### T3: Idempotency 校验

```go
func TestIdempotencyDedupe(t *testing.T) {
    sys := NewMemorySystem()
    // 两次相同 key
    _, _ = sys.Execute(ctx, Effect{IdempotencyKey: "same"})
    result2, _ := sys.Execute(ctx, Effect{IdempotencyKey: "same"})
    assert.Equal(t, result1.ID, result2.ID) // 返回相同结果
}
```

## 依赖关系

```
Phase 1 ──────────────────────────────────────────────┐
Phase 2 ──────────────────────────────────────────────┤
Phase 3 ──────┐                                       │
Phase 4 ──────┼───────────────────────────────────────┤
Phase 5 ──────┤                                       │
Phase 6 ──────┴───────────────────────────────────────┤
Phase 7 ──────────────────────────────────────────────┘
```

## 产出物清单

| 文件 | 描述 |
|------|------|
| `pkg/effects/kind.go` | EffectKind 类型定义 |
| `pkg/effects/effect.go` | Effect/Result 结构体 |
| `pkg/effects/context.go` | Context helper |
| `pkg/effects/system.go` | System 接口 |
| `pkg/effects/memory.go` | 内存实现 |
| `pkg/effects/llm.go` | LLM Effect |
| `pkg/effects/tool.go` | Tool Effect |
| `pkg/effects/http.go` | HTTP Effect |
| `pkg/effects/time.go` | Time Effect |
| `pkg/effects/sleep.go` | Sleep Effect |
| `pkg/effects/random.go` | Random Effect |
| `pkg/effects/default.go` | 默认实例 |
| `internal/runtime/effects/pgstore.go` | PostgreSQL 实现（可选） |

## 风险与缓解

| 风险 | 缓解 |
|------|------|
| 改动太大，影响现有代码 | 分 Phase 实现，每个 Phase 可独立运行测试 |
| LLM 调用分散，难以统一 | 提取 `internal/model/llm/common.go`，统一入口 |
| 性能开销 | Effect 只做轻量封装，关键路径无额外开销 |
| 忘记通过 Effect 调用 | 编译时检查（使用 wrapper 类型） |

## 验收标准

- [ ] 所有 LLM 调用通过 `effects.Execute(ctx, KindLLM)`
- [ ] 所有 Tool 调用通过 `effects.Execute(ctx, KindTool)`
- [ ] Replay 模式不触发真实 LLM/Tool/HTTP 调用
- [ ] 相同 idempotency_key 返回相同结果
- [ ] 可以写 deterministic tests（使用 memory system）