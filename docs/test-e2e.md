# End-to-end testing

This document describes the full flow: upload → parse → split → index → retrieve. You can use a **PDF** or **AGENTS.md** as the test file.

## Prerequisites

- **Go**: 1.25.7+ (aligned with go.mod and [usage.md](usage.md)).
- **Config**: `configs/model.yaml` is set up; if using OpenAI, set `OPENAI_API_KEY`. Without it the API still starts but query/upload may use placeholders or fail.
- **Storage**: Default is **memory**; data is lost on restart; for local validation only.

## 1. Start the API

```bash
go run ./cmd/api
```

Default listen: `http://localhost:8080`. Health check:

```bash
curl http://localhost:8080/api/health
```

Expected: 200, service OK.

## 2. Upload a document

### Using a PDF

Use a PDF file:

```bash
curl -X POST http://localhost:8080/api/documents/upload \
  -F "file=@/path/to/your.pdf"
```

**Expected**: 200 with `doc_id`, `chunks`, etc.; PDF content is extracted, then parsed, split, embedded, and indexed.

### Using AGENTS.md (quick check without PDF)

Use the repo’s `AGENTS.md` (or a copy as `AGENTS.txt`):

```bash
curl -X POST http://localhost:8080/api/documents/upload \
  -F "file=@./AGENTS.md"
```

Or:

```bash
cp AGENTS.md AGENTS.txt
curl -X POST http://localhost:8080/api/documents/upload \
  -F "file=@./AGENTS.txt"
```

**Expected**: 200, same flow as PDF.

## 3. List documents

```bash
curl http://localhost:8080/api/documents/
```

Expected: 200, list includes the uploaded document (id, metadata, etc.).

## 4. Query (deprecated; prefer "Send message via Agent" below)

Run a query related to the uploaded content:

```bash
curl -X POST http://localhost:8080/api/query \
  -H "Content-Type: application/json" \
  -d '{"query": "Your question", "top_k": 10}'
```

**Expected**: 200 with retrieval-based `answer` if LLM and Embedding are configured. This endpoint is deprecated; use the "Send message via Agent" flow below.

## 5. Send message via Agent (recommended E2E)

Recommended flow: create an agent, send a message, poll job status, optionally view execution trace.

1. **Create agent**:
   ```bash
   curl -s -X POST http://localhost:8080/api/agents -H "Content-Type: application/json" -d '{"name":"e2e-test"}'
   ```
   Note the returned `id` as `agent_id`.

2. **Send message**:
   ```bash
   curl -s -X POST http://localhost:8080/api/agents/<agent_id>/message \
     -H "Content-Type: application/json" \
     -d '{"message": "Your question"}'
   ```
   Returns 202 with `job_id`.

3. **Poll job status**:
   ```bash
   curl -s http://localhost:8080/api/agents/<agent_id>/jobs/<job_id>
   ```
   Until `status` is `completed` or `failed`.

4. **(Optional) View execution trace**:
   ```bash
   curl -s http://localhost:8080/api/jobs/<job_id>/trace
   ```
   Or open `http://localhost:8080/api/jobs/<job_id>/trace/page` in a browser.

## 6. Acceptance

- **PDF**: After uploading a real PDF, body text is readable; split, embed, and index succeed; a question related to the PDF returns a retrieval-based answer via `/api/query`.
- **AGENTS.md / text**: Same flow: upload, list, query.
- With default **memory** storage, data is cleared after API restart; re-upload before querying again.

Full API and flows: [usage.md](usage.md); CLI: [cli.md](cli.md).

## Optional: E2E script

The script `scripts/test-e2e.sh` runs health check → upload → list → query when the API is already running. Example:

```bash
./scripts/test-e2e.sh ./AGENTS.md
# or
./scripts/test-e2e.sh /path/to/your.pdf
```

The script prints key response fields for manual verification.
