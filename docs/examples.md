# Examples

This page briefly describes each example under [examples/](../examples/) and how to run it.

## basic_agent

Uses **eino-ext** OpenAI ChatModel and **ChatModelAgent** to run one in-process conversation. Demonstrates eino ADK agent usage.

**Run**: Set `OPENAI_API_KEY`.

```bash
OPENAI_API_KEY=sk-xxx go run ./examples/basic_agent
```

Does not require a running API server.

---

## simple_chat_agent

Minimal Aetheris agent example: no HTTP, calls `pkg/agent` `Agent.Run` for one turn. Shows checkpointed execution in the Aetheris runtime.

**Run**:

```bash
OPENAI_API_KEY=sk-xxx go run ./examples/simple_chat_agent
```

Does not require a running API server.

---

## streaming

Uses eino ChatModel + ChatModelAgent with **streaming** output. Demonstrates streaming response handling.

**Run**: Set `OPENAI_API_KEY`.

```bash
OPENAI_API_KEY=sk-xxx go run ./examples/streaming
```

Does not require a running API server.

---

## tool

Registers **tools** in eino; the agent calls them during the conversation. Shows how to define tools and have the agent use them.

**Run**: Set `OPENAI_API_KEY`.

```bash
OPENAI_API_KEY=sk-xxx go run ./examples/tool
```

Does not require a running API server.

---

## workflow

Uses **eino compose** to build a DAG (Graph), define input/output types and nodes, and run it. Demonstrates a pure DAG workflow (no agent).

**Run**:

```bash
go run ./examples/workflow
```

Does not require the API or external model services.

---

## Using with the API

These examples are **standalone processes** using eino / pkg/agent and do not need `go run ./cmd/api`. To create agents, send messages, and query jobs over HTTP, use the flows in [usage.md](usage.md) or the CLI ([cli.md](cli.md)) and start the API first (default http://localhost:8080).
