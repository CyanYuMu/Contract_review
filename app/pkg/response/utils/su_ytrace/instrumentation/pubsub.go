package instrumentation

import (
	"context"

	"cloud.google.com/go/pubsub"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_ytrace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	pubsubTracerName = "seago/utils/su_ytrace/pubsub"
)

// PubSubAttributeCarrier 实现 propagation.TextMapCarrier 接口
type PubSubAttributeCarrier struct {
	attributes *map[string]string
}

// NewPubSubAttributeCarrier 创建 Pub/Sub Attribute Carrier
func NewPubSubAttributeCarrier(attributes *map[string]string) *PubSubAttributeCarrier {
	if attributes == nil {
		attrs := make(map[string]string)
		attributes = &attrs
	}
	return &PubSubAttributeCarrier{attributes: attributes}
}

// Get 获取 attribute 值
func (c *PubSubAttributeCarrier) Get(key string) string {
	if c.attributes == nil {
		return ""
	}
	return (*c.attributes)[key]
}

// Set 设置 attribute 值
func (c *PubSubAttributeCarrier) Set(key, value string) {
	if c.attributes == nil {
		attrs := make(map[string]string)
		c.attributes = &attrs
	}
	(*c.attributes)[key] = value
}

// Keys 返回所有 key
func (c *PubSubAttributeCarrier) Keys() []string {
	if c.attributes == nil {
		return []string{}
	}
	keys := make([]string, 0, len(*c.attributes))
	for k := range *c.attributes {
		keys = append(keys, k)
	}
	return keys
}

// InjectPubSubTrace 将 TraceContext 注入到 Pub/Sub Message Attributes
func InjectPubSubTrace(ctx context.Context, msg *pubsub.Message) {
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		return
	}

	if msg == nil {
		return
	}

	if msg.Attributes == nil {
		msg.Attributes = make(map[string]string)
	}

	carrier := NewPubSubAttributeCarrier(&msg.Attributes)
	otel.GetTextMapPropagator().Inject(ctx, carrier)
}

// ExtractPubSubTrace 从 Pub/Sub Message Attributes 提取 TraceContext
func ExtractPubSubTrace(ctx context.Context, msg *pubsub.Message) context.Context {
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		return ctx
	}

	if msg == nil {
		return ctx
	}

	carrier := NewPubSubAttributeCarrier(&msg.Attributes)
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}

// StartPubSubPublisherSpan 创建 Pub/Sub Publisher Span
func StartPubSubPublisherSpan(ctx context.Context, topic string, msg *pubsub.Message) (context.Context, trace.Span) {
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		// 追踪未启用，返回空 span
		return ctx, trace.SpanFromContext(ctx)
	}

	tracer := otel.Tracer(pubsubTracerName)

	// 提取 span（支持 Worker 和 Fiber 服务）
	ctx = su_ytrace.ExtractSpanFromContext(ctx)

	ctx, span := tracer.Start(ctx, "pubsub.publish",
		trace.WithSpanKind(trace.SpanKindProducer),
	)
	su_ytrace.MaybeSetRequestID(ctx, span)

	span.SetAttributes(
		attribute.String("messaging.system", "pubsub"),
		attribute.String("messaging.operation", "publish"),
		attribute.String("messaging.destination", topic),
	)

	if msg != nil {
		span.SetAttributes(
			attribute.Int("messaging.message.body.size", len(msg.Data)),
		)
		if msg.OrderingKey != "" {
			span.SetAttributes(attribute.String("messaging.pubsub.ordering_key", msg.OrderingKey))
		}
	}

	// 注入 TraceContext
	InjectPubSubTrace(ctx, msg)

	return ctx, span
}

// StartPubSubSubscriberSpan 创建 Pub/Sub Subscriber Span
func StartPubSubSubscriberSpan(ctx context.Context, subscription string, msg *pubsub.Message) (context.Context, trace.Span) {
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		// 追踪未启用，返回空 span
		return ctx, trace.SpanFromContext(ctx)
	}

	// 提取 span（支持 Worker 和 Fiber 服务）
	ctx = su_ytrace.ExtractSpanFromContext(ctx)

	// 先提取 TraceContext
	ctx = ExtractPubSubTrace(ctx, msg)

	tracer := otel.Tracer(pubsubTracerName)

	ctx, span := tracer.Start(ctx, "pubsub.receive",
		trace.WithSpanKind(trace.SpanKindConsumer),
	)
	su_ytrace.MaybeSetRequestID(ctx, span)

	span.SetAttributes(
		attribute.String("messaging.system", "pubsub"),
		attribute.String("messaging.operation", "receive"),
		attribute.String("messaging.pubsub.subscription", subscription),
	)

	if msg != nil {
		span.SetAttributes(
			attribute.String("messaging.message.id", msg.ID),
			attribute.Int("messaging.message.body.size", len(msg.Data)),
		)
		if msg.OrderingKey != "" {
			span.SetAttributes(attribute.String("messaging.pubsub.ordering_key", msg.OrderingKey))
		}
		if msg.DeliveryAttempt != nil {
			span.SetAttributes(attribute.Int("messaging.pubsub.delivery_attempt", *msg.DeliveryAttempt))
		}
	}

	return ctx, span
}

// EndPubSubSpan 结束 Pub/Sub span
func EndPubSubSpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	su_ytrace.SafeEndSpan(span) // 使用安全的 End 方法
}
