package su_ytrace

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// createExporter 根据配置创建对应的 exporter
func createExporter(ctx context.Context, config *Config) (trace.SpanExporter, error) {
	var baseExporter trace.SpanExporter
	var err error

	switch config.Exporter {
	case ExporterJaeger:
		baseExporter, err = createJaegerExporter(config)
	case ExporterOTLP:
		baseExporter, err = createOTLPExporter(ctx, config)
	case ExporterStdout:
		baseExporter, err = createStdoutExporter()
	default:
		return nil, fmt.Errorf("unsupported exporter type: %s", config.Exporter)
	}

	if err != nil {
		return nil, err
	}

	// 包装 MetricsExporter，启用统计和日志
	return NewMetricsExporter(baseExporter,
		WithLogging(config.Debug),
		WithSpanInfo(config.Debug),
		WithServiceName(config.ServiceName),
	), nil
}

// createJaegerExporter 创建 Jaeger exporter
func createJaegerExporter(config *Config) (trace.SpanExporter, error) {
	return jaeger.New(
		jaeger.WithCollectorEndpoint(
			jaeger.WithEndpoint(config.Jaeger.Endpoint),
		),
	)
}

// createOTLPExporter 创建 OTLP exporter
func createOTLPExporter(ctx context.Context, config *Config) (trace.SpanExporter, error) {
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(config.OTLP.Endpoint),
		otlptracegrpc.WithTimeout(config.OTLP.Timeout),
	}

	if config.OTLP.Insecure {
		opts = append(opts, otlptracegrpc.WithTLSCredentials(insecure.NewCredentials()))
		opts = append(opts, otlptracegrpc.WithDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))
	}

	return otlptracegrpc.New(ctx, opts...)
}

// createStdoutExporter 创建 Stdout exporter（用于调试）
func createStdoutExporter() (trace.SpanExporter, error) {
	return stdouttrace.New(
		stdouttrace.WithPrettyPrint(),
	)
}
