// Copyright 2026 fanjia1024
// OpenTelemetry integration for distributed tracing

package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

// OTelConfig OpenTelemetry 配置
type OTelConfig struct {
	ServiceName    string
	ExportEndpoint string
	Insecure       bool
}

// InitTracer 初始化 OpenTelemetry tracer
func InitTracer(config OTelConfig) (*sdktrace.TracerProvider, error) {
	ctx := context.Background()

	// 创建 OTLP exporter
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(config.ExportEndpoint),
	}
	if config.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	exporter, err := otlptrace.New(ctx, otlptracehttp.NewClient(opts...))
	if err != nil {
		return nil, err
	}

	// 创建 resource
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(config.ServiceName),
		),
	)
	if err != nil {
		return nil, err
	}

	// 创建 tracer provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	return tp, nil
}

// StartJobSpan 开始 job execution span
func StartJobSpan(ctx context.Context, jobID string, agentID string) (context.Context, trace.Span) {
	tracer := otel.Tracer("aetheris")
	ctx, span := tracer.Start(ctx, "job.execute",
		trace.WithAttributes(
			attribute.String("job.id", jobID),
			attribute.String("agent.id", agentID),
		),
	)
	return ctx, span
}

// StartNodeSpan 开始 node execution span
func StartNodeSpan(ctx context.Context, nodeID string, nodeType string) (context.Context, trace.Span) {
	tracer := otel.Tracer("aetheris")
	ctx, span := tracer.Start(ctx, "node.execute",
		trace.WithAttributes(
			attribute.String("node.id", nodeID),
			attribute.String("node.type", nodeType),
		),
	)
	return ctx, span
}

// StartToolSpan 开始 tool invocation span
func StartToolSpan(ctx context.Context, toolName string, idempotencyKey string) (context.Context, trace.Span) {
	tracer := otel.Tracer("aetheris")
	ctx, span := tracer.Start(ctx, "tool.invoke",
		trace.WithAttributes(
			attribute.String("tool.name", toolName),
			attribute.String("tool.idempotency_key", idempotencyKey),
		),
	)
	return ctx, span
}
