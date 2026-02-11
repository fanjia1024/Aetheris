# Migrating Custom Agents to Aetheris

This guide shows how to migrate your existing agent (non-framework, custom code) to run on Aetheris and gain durability, crash recovery, and audit capabilities.

**What you get**:
- At-most-once execution (no duplicate side effects)
- Crash recovery (resume from checkpoint)
- Human-in-the-loop (wait for signals)
- Full audit trail (who, when, why)
- Replay debugging (deterministic)

---

## Before: Your Current Agent

**Typical custom agent** (Python/Go/JS):

```go
func MyRefundAgent(ctx context.Context, orderID string) error {
    // 1. Query order
    order := queryOrderAPI(orderID)
    
    // 2. LLM decides
    decision := llm.Generate("Should refund? Order: " + order.Status)
    
    // 3. Wait for approval
    approved := waitForHumanApproval() // ← Blocks thread, no crash recovery
    
    // 4. Execute refund
    if approved {
        refundAPI.Send(orderID, order.Amount) // ← May execute twice if crash
    }
    
    return nil
}
```

**Problems**:
- `waitForHumanApproval()` blocks thread (no scalability)
- Crash during `refundAPI.Send()` → may send twice (no idempotency)
- No audit trail (can't answer "why refunded?")
- No replay (can't debug determinism)

---

## After: Aetheris-Powered Agent

**Same logic, but decomposed into Tools + TaskGraph**:

### Step 1: Extract Tools

Every external call becomes a Tool:

```go
// query_order tool
type QueryOrderTool struct{}

func (t *QueryOrderTool) Execute(ctx context.Context, toolName string, input map[string]any, state interface{}) (executor.ToolResult, error) {
    orderID, _ := input["order_id"].(string)
    order := queryOrderAPI(orderID) // ← External API call
    
    output, _ := json.Marshal(order)
    return executor.ToolResult{Done: true, Output: string(output)}, nil
}

// send_refund tool
type SendRefundTool struct{}

func (t *SendRefundTool) Execute(ctx context.Context, toolName string, input map[string]any, state interface{}) (executor.ToolResult, error) {
    // ⚠️ Key: Get idempotency key and pass to external API
    jobID := executor.JobIDFromContext(ctx)
    stepID := executor.ExecutionStepIDFromContext(ctx)
    key := executor.StepIdempotencyKeyForExternal(ctx, jobID, stepID)
    
    orderID, _ := input["order_id"].(string)
    amount, _ := input["amount"].(float64)
    
    // Pass key to ensure at-most-once
    err := refundAPI.Send(key, orderID, amount)
    
    return executor.ToolResult{Done: true, Output: "refunded"}, err
}
```

**Why this works**:
- Runtime tracks each Tool execution (InvocationLedger)
- Crash before `command_committed` → Effect Store catch-up (Tool NOT re-executed)
- Replay → inject result from Ledger (Tool NOT re-called)

---

### Step 2: Define TaskGraph (Plan)

Convert your agent logic to a graph:

```go
func PlanMyAgent(ctx context.Context, goal string, mem memory.Memory) (*planner.TaskGraph, error) {
    return &planner.TaskGraph{
        Nodes: []planner.TaskNode{
            // Original: order := queryOrderAPI(orderID)
            {ID: "query", Type: "tool", ToolName: "query_order", Config: map[string]any{"order_id": "order-123"}},
            
            // Original: decision := llm.Generate(...)
            {ID: "decide", Type: "llm", Config: map[string]any{"goal": "Should refund?"}},
            
            // Original: approved := waitForHumanApproval()
            {ID: "wait", Type: "wait", Config: map[string]any{
                "wait_kind": "human",
                "correlation_key": "approval-" + uuid.New().String(),
                "park": true,  // Long wait, don't block Worker
            }},
            
            // Original: refundAPI.Send(...)
            {ID: "refund", Type: "tool", ToolName: "send_refund"},
        },
        Edges: []planner.TaskEdge{
            {From: "query", To: "decide"},
            {From: "decide", To: "wait"},
            {From: "wait", To: "refund"},
        },
    }, nil
}
```

**Mapping rules**:
- External API call → Tool node
- LLM call → LLM node
- Blocking wait → Wait node (`park: true` for >1 min waits)
- Pure computation → Can stay in Tool (or separate node)

---

### Step 3: Register & Run

```go
func main() {
    ctx := context.Background()
    
    // Register tools
    registry := tools.NewRegistry()
    registry.Register("query_order", &QueryOrderTool{})
    registry.Register("send_refund", &SendRefundTool{})
    
    // Create planner
    planner := &MyAgentPlanner{} // Implements PlanGoal() returning TaskGraph
    
    // Create agent
    agent := runtime.NewAgent("my-agent", "My Agent", session, mem, planner, registry)
    
    // Create job (via API or direct)
    jobID := createJob(agent, "退款 order-123")
    
    // Agent executes asynchronously
    // - Query tool → LLM → Wait (StatusParked)
    // - Human sends signal → Agent resumes → Refund tool
    // - Complete with full trace
}
```

**What changed**:
- Before: Synchronous function (blocks on wait)
- After: Asynchronous job (Worker executes, releases on wait, resumes on signal)

**What you gained**:
- Crash recovery (Worker crash → resume from Checkpoint)
- At-most-once (Tool idempotency)
- Audit trail (Evidence Graph, Trace API)
- Replay debugging (verify determinism)

---

## Migration Checklist

### 1. Identify External Calls

List all external API calls, database writes, file I/O in your agent:

```
- queryOrderAPI()        → query_order tool
- refundAPI.Send()       → send_refund tool
- emailAPI.Send()        → send_email tool
- database.Insert()      → db_insert tool
- slack.PostMessage()    → slack_notify tool
```

**Rule**: Every external call must become a Tool (see [design/step-contract.md](../../design/step-contract.md)).

---

### 2. Identify LLM Calls

List all LLM calls:

```
- llm.Generate("Should refund?")           → LLM node
- llm.Generate("Summarize order")          → LLM node
- llm.ChatCompletion(messages)             → LLM node
```

**Rule**: LLM calls must go through LLM nodes (not direct API calls) for replay determinism.

---

### 3. Identify Waits

List all blocking waits:

```
- waitForHumanApproval()        → Wait node (wait_kind=human, park=true)
- waitForWebhook()              → Wait node (wait_kind=webhook)
- time.Sleep(3 * time.Hour)     → Wait node (wait_kind=timer)
- waitForAPIRateLimit()         → Wait node (wait_kind=condition)
```

**Rule**: Long waits (>1 min) should use `park: true` to avoid blocking Scheduler.

---

### 4. Define TaskGraph

Convert your agent logic flow to a DAG:

```
Original flow:
  A → B → C → D

TaskGraph:
  Nodes: [A, B, C, D]
  Edges: [A→B, B→C, C→D]
```

**Example**:

```go
// Original (imperative)
func MyAgent() {
    data := fetchData()      // A
    result := process(data)  // B
    wait()                   // C
    send(result)             // D
}

// TaskGraph (declarative)
&planner.TaskGraph{
    Nodes: []planner.TaskNode{
        {ID: "fetch", Type: "tool", ToolName: "fetch_data"},
        {ID: "process", Type: "tool", ToolName: "process_data"},
        {ID: "wait", Type: "wait"},
        {ID: "send", Type: "tool", ToolName: "send_result"},
    },
    Edges: []planner.TaskEdge{
        {From: "fetch", To: "process"},
        {From: "process", To: "wait"},
        {From: "wait", To: "send"},
    },
}
```

---

### 5. Handle State

**Before**: Variables in function scope

```go
func MyAgent() {
    order := fetchOrder()    // Local variable
    decision := llm.Generate() // Local variable
    refund(order.ID, order.Amount) // Uses local variables
}
```

**After**: State in `payload.Results` (agent memory)

```go
// Tool A writes to state
func (t *FetchOrderTool) Execute(ctx, toolName, input, state) (executor.ToolResult, error) {
    order := fetchOrder()
    output, _ := json.Marshal(map[string]interface{}{
        "order_id": order.ID,
        "amount": order.Amount,
    })
    return executor.ToolResult{Done: true, Output: string(output)}, nil
}

// Tool B reads from state (via TaskNode config or from payload.Results)
func (t *RefundTool) Execute(ctx, toolName, input, state) (executor.ToolResult, error) {
    // input comes from previous step's output or TaskNode.Config
    orderID, _ := input["order_id"].(string)
    amount, _ := input["amount"].(float64)
    // ...
}
```

**State flow**:
- Each step writes to `payload.Results[node_id]`
- Next step reads from `payload.Results` (via config or direct access)
- Runtime saves `state_before` and `state_after` for each step (state_checkpointed event)

---

## Common Patterns

### Pattern 1: Sequential Steps

**Before**:
```go
func Agent() {
    a := stepA()
    b := stepB(a)
    c := stepC(b)
}
```

**After**:
```go
Nodes: [
    {ID: "a", Type: "tool", ToolName: "step_a"},
    {ID: "b", Type: "tool", ToolName: "step_b"},
    {ID: "c", Type: "tool", ToolName: "step_c"},
]
Edges: [{From: "a", To: "b"}, {From: "b", To: "c"}]
```

---

### Pattern 2: Conditional Logic

**Before**:
```go
func Agent() {
    order := fetchOrder()
    if order.Amount > 500 {
        waitApproval()
    }
    refund()
}
```

**After**:
```go
Nodes: [
    {ID: "fetch", Type: "tool", ToolName: "fetch_order"},
    {ID: "decide", Type: "llm", Config: {"goal": "amount>500? need approval"}},
    {ID: "wait", Type: "wait"},      // Conditional: only if decide returns "yes"
    {ID: "refund", Type: "tool"},
]
// Conditional edges: decide → wait (if yes) or decide → refund (if no)
// Phase 1: Use LLM node to decide; LLM output determines next step
// Phase 2: Native conditional_edges support
```

---

### Pattern 3: Error Handling

**Before**:
```go
func Agent() {
    result, err := callAPI()
    if err != nil {
        retry() // Custom retry logic
    }
}
```

**After**:
```go
// Tool returns error → Runtime classifies as retryable_failure
func (t *CallAPITool) Execute(...) (executor.ToolResult, error) {
    result, err := callAPI()
    if err != nil {
        return executor.ToolResult{Done: false, Err: err.Error()}, err
    }
    return executor.ToolResult{Done: true, Output: result}, nil
}

// Runtime handles retry
// - Step fails → StepResultRetryableFailure
// - Scheduler Requeue (with backoff)
// - Replay: already-succeeded steps injected (not re-executed)
```

**Configuration** (configs/worker.yaml):
```yaml
scheduler:
  retry_max: 3
  retry_backoff: "exponential"  # 1s, 2s, 4s
```

---

## Example: Migrating a Sales Outreach Agent

### Original Agent (100 lines)

```go
func SalesAgent(ctx context.Context, leadID string) error {
    // 1. Enrich lead from CRM
    lead := salesforce.GetLead(leadID)
    
    // 2. LLM drafts email
    email := llm.Generate("Draft outreach email for: " + lead.Company)
    
    // 3. Wait for human review
    approved := waitForReview(email) // Blocks thread
    
    // 4. Send email
    if approved {
        sendgrid.Send(lead.Email, email) // May send twice if crash
    }
    
    return nil
}
```

**Problems**:
- Blocks thread during review (can't scale to 1000 leads)
- Crash during sendgrid.Send() → may send twice
- No audit (can't prove "who approved which email")

---

### Migrated to Aetheris (150 lines, but production-grade)

**Tools**:

```go
// 1. Enrich lead (external API)
type EnrichLeadTool struct{}
func (t *EnrichLeadTool) Execute(ctx, toolName, input, state) (executor.ToolResult, error) {
    leadID, _ := input["lead_id"].(string)
    lead := salesforce.GetLead(leadID)
    output, _ := json.Marshal(lead)
    return executor.ToolResult{Done: true, Output: string(output)}, nil
}

// 2. Send email (external side effect, must be at-most-once)
type SendEmailTool struct{}
func (t *SendEmailTool) Execute(ctx, toolName, input, state) (executor.ToolResult, error) {
    // ⚠️ Idempotency key for at-most-once
    key := executor.StepIdempotencyKeyForExternal(ctx, jobID, stepID)
    
    to, _ := input["to"].(string)
    content, _ := input["content"].(string)
    
    // Pass key to sendgrid for deduplication
    err := sendgrid.Send(key, to, content)
    
    return executor.ToolResult{Done: true, Output: "sent"}, err
}
```

**Planner**:

```go
func PlanSalesAgent(ctx, goal, mem) (*planner.TaskGraph, error) {
    return &planner.TaskGraph{
        Nodes: []planner.TaskNode{
            {ID: "enrich", Type: "tool", ToolName: "enrich_lead"},
            {ID: "draft", Type: "llm", Config: {"goal": "Draft outreach email"}},
            {ID: "wait_review", Type: "wait", Config: {
                "wait_kind": "human",
                "correlation_key": "review-" + uuid.New().String(),
                "park": true,  // May wait hours
            }},
            {ID: "send", Type: "tool", ToolName: "send_email"},
        },
        Edges: []planner.TaskEdge{
            {From: "enrich", To: "draft"},
            {From: "draft", To: "wait_review"},
            {From: "wait_review", To: "send"},
        },
    }, nil
}
```

**What you gained**:
- ✅ **Scalability**: 1000 leads waiting for review → StatusParked (no thread blocked)
- ✅ **At-most-once**: Crash during send → resume without re-sending
- ✅ **Audit**: Can answer "who reviewed email X? when sent? why?"
- ✅ **Replay**: Verify determinism (LLM NOT re-called, email NOT re-sent)

**Added lines**: ~50 (Tool wrappers + TaskGraph)  
**Production readiness**: ∞ (crash recovery, audit, at-most-once)

---

## Migration Effort Estimation

| Agent Complexity | Tools Count | Migration Time | Lines Added |
|------------------|-------------|----------------|-------------|
| **Simple** (1-2 external calls) | 2-3 tools | 30 min | ~50 lines |
| **Medium** (3-5 calls + wait) | 4-6 tools | 1-2 hours | ~100 lines |
| **Complex** (10+ calls + conditional logic) | 10+ tools | 4-8 hours | ~300 lines |

**Payoff**: One-time migration cost → permanent production-grade runtime.

---

## Migration Strategy

### Incremental Migration (Recommended)

Don't rewrite your entire agent at once. Migrate high-risk parts first:

**Phase 1**: Migrate side-effect tools
```
Priority: Refund, Payment, Email → Tools with idempotency
Keep: Query, LLM → Can stay as-is temporarily
```

**Phase 2**: Add wait nodes
```
Add: Human approval → Wait node
Keep: Synchronous flow → TaskGraph with no waits
```

**Phase 3**: Full migration
```
All external calls → Tools
All LLM → LLM nodes
Complete TaskGraph
```

---

### Parallel Running (Risk Mitigation)

Run old agent and Aetheris agent in parallel during migration:

```go
// Old agent (production)
go oldAgent(ctx, input)

// New Aetheris agent (shadow mode)
go func() {
    job := createAetherisJob(agent, input)
    // Compare results
    if job.Result != oldResult {
        log.Warn("Aetheris result differs from old agent")
    }
}()
```

**Gradually shift traffic**: 1% → 10% → 50% → 100% to Aetheris.

---

## Tool Wrapping Guidelines

### Idempotency Key Placement

**For payment/email/webhook APIs**:

```go
func (t *PaymentTool) Execute(ctx, toolName, input, state) (executor.ToolResult, error) {
    key := executor.StepIdempotencyKeyForExternal(ctx, jobID, stepID)
    
    // Option 1: API supports idempotency key header
    req.Header.Set("Idempotency-Key", key)
    
    // Option 2: API uses request body field
    params.IdempotencyKey = key
    
    // Option 3: API doesn't support → use application-level dedup
    if alreadyExecuted := checkLocalCache(key); alreadyExecuted {
        return getCachedResult(key), nil
    }
    result := callAPI()
    cacheResult(key, result)
    return result, nil
}
```

---

### Error Handling

**Distinguish permanent vs retryable failures**:

```go
func (t *MyTool) Execute(...) (executor.ToolResult, error) {
    result, err := callAPI()
    
    if err != nil {
        // Retryable: network timeout, 5xx error
        if isRetryable(err) {
            return executor.ToolResult{Done: false, Err: err.Error()}, err
        }
        
        // Permanent: 4xx error, invalid input
        return executor.ToolResult{Done: true, Err: "permanent: " + err.Error()}, fmt.Errorf("permanent: %w", err)
    }
    
    return executor.ToolResult{Done: true, Output: result}, nil
}
```

**Runtime behavior**:
- Retryable error → Scheduler Requeue (with backoff)
- Permanent error → Job Failed (no retry)

---

### Stateful Tools (Multi-Step)

**For tools that need multiple calls** (e.g., long-running API, pagination):

```go
type PaginatedQueryTool struct{}

func (t *PaginatedQueryTool) Execute(ctx, toolName, input, state) (executor.ToolResult, error) {
    // First call: state == nil
    if state == nil {
        page1 := fetchPage(1)
        return executor.ToolResult{
            Done: false,          // Not done yet
            State: map[string]any{"page": 2, "results": page1},
            Output: page1,
        }, nil
    }
    
    // Subsequent calls: state has "page"
    stateMap, _ := state.(map[string]any)
    page, _ := stateMap["page"].(int)
    results, _ := stateMap["results"].(string)
    
    pageN := fetchPage(page)
    if isLastPage(pageN) {
        return executor.ToolResult{
            Done: true,           // Done
            Output: results + pageN,
        }, nil
    }
    
    return executor.ToolResult{
        Done: false,
        State: map[string]any{"page": page + 1, "results": results + pageN},
        Output: results + pageN,
    }, nil
}
```

**Runtime behavior**: Re-invokes tool with `state` until `Done: true`.

---

## Comparison: Before vs After

| Aspect | Before (Custom Agent) | After (Aetheris) |
|--------|----------------------|------------------|
| **Code complexity** | 100 lines | 150 lines (+50 for Tools/TaskGraph) |
| **Crash recovery** | None (lost state) | Automatic (Checkpoint + Replay) |
| **At-most-once** | Manual (if at all) | Automatic (Ledger + Effect Store) |
| **Human-in-the-loop** | Blocks thread | Non-blocking (Wait + Signal) |
| **Audit trail** | Logs (unreliable) | Event stream (immutable) |
| **Debugging** | Print statements | Replay + Trace + Evidence |
| **Scalability** | 1 agent = 1 thread | 1k agents waiting (StatusParked) |
| **Testing** | Hard (mock external APIs) | Easy (Replay verifies determinism) |

**Trade-off**: +50 lines of boilerplate → production-grade runtime.

---

## Real-World Migration: Stripe Payment Agent

**Original** (30 lines, no durability):

```python
def payment_agent(order_id):
    order = fetch_order(order_id)
    if order.amount > 1000:
        approval = wait_for_cfo()  # Blocks
    charge = stripe.Charge.create(amount=order.amount)  # May charge twice
    return charge.id
```

**Migrated** (80 lines, production-ready):

```go
// Tools: fetch_order, stripe_charge
// TaskGraph: fetch → decide → wait(if needed) → charge
// Result: at-most-once charge, crash recovery, audit trail
```

**Migration time**: 2 hours  
**Incidents before**: 2/month (duplicate charges due to retry)  
**Incidents after**: 0 (at-most-once + idempotency key)

---

## Next Steps

After migration:

1. **Test crash scenarios**: Kill Worker during execution, verify recovery
2. **Test replay**: Compare first execution vs replay (should be identical)
3. **Configure production**: Postgres, Effect Store, Ledger, WakeupQueue
4. **Monitor**: Prometheus metrics (see [docs/observability.md](../observability.md))
5. **Read contracts**: [design/step-contract.md](../../design/step-contract.md) (prohibited behaviors), [design/execution-guarantees.md](../../design/execution-guarantees.md) (guarantees)

---

## FAQ

### Q: Do I have to rewrite my agent?

**A**: Not entirely. You define Tools (wrap external calls) and TaskGraph (convert logic flow to DAG). Core logic can stay mostly the same.

### Q: What if my agent is too complex for a static TaskGraph?

**A**: Use dynamic planning. Your Planner can call LLM to generate TaskGraph based on input. Aetheris supports this (see [docs/getting-started-agents.md](../getting-started-agents.md)).

### Q: Can I use LangGraph/LangChain alongside Aetheris?

**A**: Yes. Use LangChain for prompt engineering, RAG; use Aetheris for execution runtime (durability, crash recovery). See `examples/langgraph_adapter/` (TODO).

### Q: How do I handle long Tool execution (>5 min)?

**A**: Configure step_timeout or break into smaller Tools. For very long operations (hours), use Wait node + external callback.

### Q: What about tool retries?

**A**: Configure `retry_max` in Worker. Runtime retries failed steps (with Replay: already-succeeded steps not re-executed).

---

## Support

- **Docs**: [docs/](../)
- **Design specs**: [design/](../../design/)
- **Examples**: [examples/](../../examples/)
- **Issues**: GitHub Issues (TODO: add link)
