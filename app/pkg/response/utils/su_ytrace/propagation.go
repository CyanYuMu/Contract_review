package su_ytrace

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// TextMapCarrier 是一个通用的 TextMapCarrier 实现
type TextMapCarrier map[string]string

// Get 获取 key 对应的值
func (c TextMapCarrier) Get(key string) string {
	return c[key]
}

// Set 设置 key-value
func (c TextMapCarrier) Set(key, value string) {
	c[key] = value
}

// Keys 返回所有的 key
func (c TextMapCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// InjectContext 将 TraceContext 注入到 carrier
func InjectContext(ctx context.Context, carrier propagation.TextMapCarrier) {
	otel.GetTextMapPropagator().Inject(ctx, carrier)
}

// ExtractContext 从 carrier 提取 TraceContext
func ExtractContext(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}
