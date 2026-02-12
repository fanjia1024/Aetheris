# Minimal Aetheris Agent

This project was scaffolded with `aetheris init`. It contains a minimal agent setup to run on Aetheris.

## Next steps

1. **Define your agent**: Add tools and a TaskGraph (see [Getting Started with Agents](https://github.com/your-org/rag-platform/blob/main/docs/getting-started-agents.md) in the Aetheris repo).
2. **Configure**: Edit `configs/api.yaml` if you need Postgres or different ports.
3. **Run**: Start the API and Worker, then create an agent and send messages.

```bash
# From the Aetheris repo root (or where your agent code lives)
make run
# Or: go run ./cmd/api & go run ./cmd/worker &

# Create agent and chat (use the CLI from the Aetheris repo)
aetheris agent create my-agent
aetheris chat <agent_id>
```

## Docs

- [Getting Started with Agents](https://github.com/your-org/rag-platform/blob/main/docs/getting-started-agents.md) — Full refund-approval example, tools, TaskGraph, signals.
- [Custom Agent Adapter](https://github.com/your-org/rag-platform/blob/main/docs/adapters/custom-agent.md) — Migrate existing agents to Aetheris.
