# Tracing

This page describes **HTTP request OpenTelemetry tracing** (Hertz + obs-opentelemetry): each HTTP request gets a trace span and can be exported to an OTLP backend. This is different from **Job execution trace** (timeline and execution_tree for a single agent job); for that see [design/execution-trace.md](../design/execution-trace.md) and the "Execution trace" section in [usage.md](usage.md), and `GET /api/jobs/:id/trace`.

The API uses [Hertz tracing](https://www.cloudwego.io/zh/docs/hertz/tutorials/observability/tracing/) and [hertz-contrib/obs-opentelemetry](https://github.com/hertz-contrib/obs-opentelemetry) to send spans to OTLP backends (e.g. Jaeger, OpenTelemetry Collector).

## Configuration

In `configs/api.yaml` under `monitoring.tracing`:

```yaml
monitoring:
  tracing:
    enable: true
    service_name: "rag-api"        # Service name in traces
    export_endpoint: "localhost:4317"  # OTLP gRPC endpoint
    insecure: true                  # Use true for non-TLS
```

- If `export_endpoint` is not set, the **OTEL_EXPORTER_OTLP_ENDPOINT** env var is used (endpoint only, e.g. `localhost:4317`).
- When `enable` is false or no endpoint is configured, no provider is created and no traces are sent.

## Viewing traces locally (Jaeger)

1. Start Jaeger with OTLP gRPC:

   ```bash
   docker run -d --name jaeger \
     -p 16686:16686 -p 4317:4317 \
     jaegertracing/all-in-one:latest \
     --collector.otlp.enabled=true
   ```

2. Set the API `export_endpoint` to `localhost:4317` (or your host:4317) and `monitoring.tracing.enable: true`, then start the API.

3. After some requests (e.g. upload, query), open http://localhost:16686 and select service `rag-api` in the Jaeger UI.

## Notes

- Request context carries trace info into `ExecuteWorkflow` (ingest_pipeline, query_pipeline), so the full call chain is in one trace.
- To add child spans for loader/parser/retrieve/generate inside the pipeline, use `otel trace.SpanFromContext(ctx)` and `tracer.Start(ctx, "step_name", ...)` in `internal/runtime/eino/workflow_executors.go`.

## FAQ

- **No traces**: Ensure `monitoring.tracing.enable` is true and `export_endpoint` or `OTEL_EXPORTER_OTLP_ENDPOINT` is set; restart the API after changes.
- **With Jaeger**: Ensure OTLP is enabled (e.g. `--collector.otlp.enabled=true`), or the API’s spans will not be received.
