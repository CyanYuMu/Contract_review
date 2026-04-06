package su_ytrace

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

// Environment 常量
const (
	EnvProduction = "prod"
	EnvTest       = "test"
	EnvLocal      = "local"
)

var (
	// 全局 TracerProvider
	globalProvider    *sdktrace.TracerProvider
	globalExporter    sdktrace.SpanExporter // 保存 exporter 引用，确保 Shutdown 时正确关闭
	globalMu          sync.Mutex
	initialized       bool
	globalServiceName atomic.Value // 存储服务名称，使用 atomic.Value 保证并发安全
	globalEnabled     atomic.Bool
)

// InitOption 初始化选项
type InitOption struct {
	// UseBuiltinConfig 是否使用内置配置
	UseBuiltinConfig bool

	// Environment 环境（仅在使用内置配置时有效）
	Environment string

	// ExternalConfig 外部传入的配置（来自业务侧）
	ExternalConfig *Config
}

// InitWithConfig 使用外部配置初始化（业务侧调用）
// cleanup 逻辑已封装在内部，业务方在程序退出时调用 Shutdown() 即可
func InitWithConfig(serviceName string, externalConfig *Config) error {
	option := &InitOption{
		UseBuiltinConfig: false,
		ExternalConfig:   externalConfig,
	}

	return InitWithOptions(serviceName, option)
}

// InitWithEnvironment 使用内置配置 + 环境初始化
// cleanup 逻辑已封装在内部，业务方在程序退出时调用 Shutdown() 即可
func InitWithEnvironment(serviceName, environment string) error {
	option := &InitOption{
		UseBuiltinConfig: true,
		Environment:      environment,
	}

	return InitWithOptions(serviceName, option)
}

// InitWithOptions 核心初始化函数
func InitWithOptions(serviceName string, option *InitOption) error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if initialized {
		return fmt.Errorf("tracer already initialized")
	}

	// 获取配置
	config, err := resolveConfig(option)
	if err != nil {
		return fmt.Errorf("failed to resolve config: %w", err)
	}

	// 使用传入的 serviceName 覆盖配置
	if serviceName != "" {
		config.ServiceName = serviceName
	}

	// 存储 serviceName 到全局变量（使用 atomic.Value 保证并发安全）
	globalServiceName.Store(config.ServiceName)

	// 验证配置
	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// 如果未启用追踪，标记为已初始化但不创建 provider
	if !config.Enabled {
		initialized = true
		return nil
	}

	// 创建资源
	res, err := resource.New(
		context.Background(),
		resource.WithAttributes(
			semconv.ServiceName(config.ServiceName),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create resource: %w", err)
	}

	// 创建 exporter
	exporter, err := createExporter(context.Background(), config)
	if err != nil {
		return fmt.Errorf("failed to create exporter: %w", err)
	}

	// 创建 TracerProvider
	// 使用 ParentBased Sampler：
	// 1、父 span 已采样，则子 span 也采样（跨服务调用时保持一致）
	// 2、没有父 span，使用本地采样率决策
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(
			sdktrace.TraceIDRatioBased(config.Sampling.Ratio),
		)),
	)

	// 设置全局 TracerProvider
	otel.SetTracerProvider(tp)

	// 设置全局 TextMapPropagator（W3C TraceContext）
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	globalProvider = tp
	globalExporter = exporter // 保存 exporter 引用
	initialized = true
	globalEnabled.Store(true) // 标记追踪已启用

	return nil
}

// resolveConfig 根据选项解析配置
func resolveConfig(option *InitOption) (*Config, error) {
	// 1. 如果有外部配置，直接使用
	if option.ExternalConfig != nil {
		return option.ExternalConfig, nil
	}

	// 2. 如果使用内置配置，根据环境返回
	if option.UseBuiltinConfig {
		return getBuiltinConfigByEnvironment(option.Environment), nil
	}

	// 3. 默认返回本地环境配置
	return getBuiltinConfigByEnvironment(EnvLocal), nil
}

// getBuiltinConfigByEnvironment 根据环境获取内置配置
func getBuiltinConfigByEnvironment(env string) *Config {
	switch env {
	case EnvProduction:
		return &Config{
			Enabled:     true,
			ServiceName: "unknown-service",
			Exporter:    ExporterOTLP,
			OTLP: OTLPConfig{
				Endpoint: "tempo.internal:4317",
				Insecure: false,
				Timeout:  10 * time.Second,
			},
			Sampling: SamplingConfig{
				Ratio: 0.1, // 生产环境 10% 采样
			},
		}
	case EnvTest:
		return &Config{
			Enabled:     true,
			ServiceName: "unknown-service",
			Exporter:    ExporterJaeger,
			Jaeger: JaegerConfig{
				Endpoint: "http://jaeger-test.internal:14268/api/traces",
			},
			Sampling: SamplingConfig{
				Ratio: 1.0, // 测试环境全采样
			},
		}
	case EnvLocal:
		return &Config{
			Enabled:     true,
			ServiceName: "unknown-service",
			Exporter:    ExporterStdout, // 本地直接输出到控制台
			Sampling: SamplingConfig{
				Ratio: 1.0, // 本地环境全采样
			},
		}
	default:
		// 默认当作 local 环境处理
		return getBuiltinConfigByEnvironment(EnvLocal)
	}
}

// GetTracer 获取 Tracer
// name: tracer 名称，通常是包名
func GetTracer(name string) trace.Tracer {
	return otel.Tracer(name)
}

// Shutdown 关闭追踪系统
func Shutdown(ctx context.Context) error {
	// 先获取 provider 和 exporter 并清空全局状态
	globalMu.Lock()
	provider := globalProvider
	exporter := globalExporter
	globalProvider = nil
	globalExporter = nil
	initialized = false
	globalMu.Unlock()

	// 立即标记为未启用（避免新的追踪操作）
	globalEnabled.Store(false)

	// 在释放锁后再调用 Shutdown，避免阻塞其他操作
	// 先关闭 provider（会 flush 所有 pending spans），再关闭 exporter（释放连接）
	var errs []error

	if provider != nil {
		if err := provider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("provider shutdown failed: %w", err))
		}
	}

	// 显式关闭 exporter，确保 gRPC 连接等资源完全释放
	if exporter != nil {
		if err := exporter.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("exporter shutdown failed: %w", err))
		}
	}

	// 如果有多个错误，返回第一个（provider 的错误优先级更高）
	if len(errs) > 0 {
		return errs[0]
	}

	return nil
}

// IsEnabled 检查追踪是否已启用
// 无锁检查
func IsEnabled() bool {
	return globalEnabled.Load()
}

// ShutdownTracing 关闭追踪系统（业务方在程序退出时调用）
// 这是 Shutdown 的别名，提供更明确的语义
func ShutdownTracing(ctx context.Context) error {
	return Shutdown(ctx)
}

// Setup 初始化追踪系统并返回清理函数
// 这是一个便捷函数，封装了 InitWithConfig 和 Shutdown 的调用
//
// 使用示例：
//
//	cfg := &su_ytrace.Config{
//	    Enabled:     true,
//	    ServiceName: "my-service",
//	    Exporter:    su_ytrace.ExporterOTLP,
//	    // ...
//	}
//	cleanup, err := su_ytrace.Setup(ctx, "my-service", cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer cleanup()
func Setup(ctx context.Context, serviceName string, cfg *Config) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := InitWithConfig(serviceName, cfg); err != nil {
		return nil, err
	}
	cleanup := func() {
		_ = Shutdown(ctx)
	}
	return cleanup, nil
}

// SetupWithEnvironment 使用内置环境配置初始化追踪系统
// 这是一个便捷函数，封装了 InitWithEnvironment 和 Shutdown 的调用
//
// 使用示例：
//
//	cleanup, err := su_ytrace.SetupWithEnvironment(ctx, "my-service", su_ytrace.EnvProduction)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer cleanup()
func SetupWithEnvironment(ctx context.Context, serviceName, environment string) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := InitWithEnvironment(serviceName, environment); err != nil {
		return nil, err
	}
	cleanup := func() {
		_ = Shutdown(ctx)
	}
	return cleanup, nil
}
