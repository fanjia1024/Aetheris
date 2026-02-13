# Adapter Index

This page compares available migration adapters and helps you choose one.

| Adapter | Best for | Effort | Checkpoint granularity |
|---------|----------|--------|------------------------|
| [Custom Agent Adapter](custom-agent.md) | Existing imperative/custom agents | Low-Medium | Step-level (TaskGraph-based) |
| [LangGraph Adapter](langgraph.md) | Existing LangGraph flows | Medium | Bridge-level first, then step-level |

## Selection guide

- Pick **Custom Agent Adapter** when your current agent logic is framework-neutral and you can extract tools/planner directly.
- Pick **LangGraph Adapter** when you already rely on LangGraph state transitions and want staged migration to Aetheris runtime guarantees.

## Common requirements

Regardless of adapter:

1. External side effects must go through Aetheris Tool path.
2. Wait/signal must use Aetheris wait contract (`correlation_key`).
3. Replay determinism must be validated in staging before production rollout.

