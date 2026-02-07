# 链路追踪

API 服务通过 [Hertz 链路追踪](https://www.cloudwego.io/zh/docs/hertz/tutorials/observability/tracing/) 与 [hertz-contrib/obs-opentelemetry](https://github.com/hertz-contrib/obs-opentelemetry) 接入 OpenTelemetry，每个 HTTP 请求会自动创建 trace span，并支持将 span 上报到 OTLP 后端（如 Jaeger、OpenTelemetry Collector）。

## 配置

在 `configs/api.yaml` 的 `monitoring.tracing` 下配置：

```yaml
monitoring:
  tracing:
    enable: true
    service_name: "rag-api"        # 服务名，用于 trace 中标识
    export_endpoint: "localhost:4317"  # OTLP gRPC 端点
    insecure: true                  # 非 TLS 连接时设为 true
```

- 若不配置 `export_endpoint`，将读取环境变量 **OTEL_EXPORTER_OTLP_ENDPOINT**（仅 endpoint，不含协议；如 `localhost:4317`）。
- `enable: false` 或不配置 endpoint 时，不创建 provider、不上报 trace，行为与未接入时一致。

## 本地查看 Trace（Jaeger）

1. 使用 Docker 启动 Jaeger（含 OTLP gRPC 接收）：

   ```bash
   docker run -d --name jaeger \
     -p 16686:16686 -p 4317:4317 \
     jaegertracing/all-in-one:latest \
     --collector.otlp.enabled=true
   ```

2. 将 API 的 `export_endpoint` 设为 `localhost:4317`（或本机 IP:4317），并设置 `monitoring.tracing.enable: true`，启动 API。

3. 发起若干请求（如上传文档、查询）后，打开 http://localhost:16686 在 Jaeger UI 中选择服务 `rag-api` 查看 trace。

## 说明

- 请求的 context 会携带 trace 信息传递到 `ExecuteWorkflow`（ingest_pipeline、query_pipeline），因此整条调用链在同一 trace 下。
- 若需在 pipeline 内为 loader/parser/retrieve/generate 等步骤创建子 span，可在 `internal/runtime/eino/workflow_executors.go` 中使用 `otel trace.SpanFromContext(ctx)` 与 `tracer.Start(ctx, "step_name", ...)` 扩展。
