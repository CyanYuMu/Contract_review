package instrumentation

import (
	"context"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_ytrace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	kafkaTracerName = "seago/utils/su_ytrace/kafka"
)

// KafkaHeaderCarrier 实现 propagation.TextMapCarrier 接口
type KafkaHeaderCarrier struct {
	headers *[]kafka.Header
}

// NewKafkaHeaderCarrier 创建 Kafka Header Carrier
func NewKafkaHeaderCarrier(headers *[]kafka.Header) *KafkaHeaderCarrier {
	if headers == nil {
		h := make([]kafka.Header, 0)
		headers = &h
	}
	return &KafkaHeaderCarrier{headers: headers}
}

// Get 获取 header 值
func (c *KafkaHeaderCarrier) Get(key string) string {
	for _, h := range *c.headers {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

// Set 设置 header 值
func (c *KafkaHeaderCarrier) Set(key, value string) {
	// 检查是否已存在
	for i, h := range *c.headers {
		if h.Key == key {
			(*c.headers)[i].Value = []byte(value)
			return
		}
	}
	// 不存在则添加
	*c.headers = append(*c.headers, kafka.Header{
		Key:   key,
		Value: []byte(value),
	})
}

// Keys 返回所有 key
func (c *KafkaHeaderCarrier) Keys() []string {
	keys := make([]string, len(*c.headers))
	for i, h := range *c.headers {
		keys[i] = h.Key
	}
	return keys
}

// InjectKafkaTrace 将 TraceContext 注入到 Kafka Message Headers
func InjectKafkaTrace(ctx context.Context, msg *kafka.Message) {
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		return
	}

	if msg == nil {
		return
	}

	carrier := NewKafkaHeaderCarrier(&msg.Headers)
	otel.GetTextMapPropagator().Inject(ctx, carrier)
}

// ExtractKafkaTrace 从 Kafka Message Headers 提取 TraceContext
func ExtractKafkaTrace(ctx context.Context, msg *kafka.Message) context.Context {
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		return ctx
	}

	if msg == nil {
		return ctx
	}

	carrier := NewKafkaHeaderCarrier(&msg.Headers)
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}

// StartKafkaProducerSpan 创建 Kafka Producer Span
func StartKafkaProducerSpan(ctx context.Context, topic string, msg *kafka.Message) (context.Context, trace.Span) {
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		// 追踪未启用，返回空 span
		return ctx, trace.SpanFromContext(ctx)
	}

	tracer := otel.Tracer(kafkaTracerName)

	// 提取 span（支持 Worker 和 Fiber 服务）
	ctx = su_ytrace.ExtractSpanFromContext(ctx)

	ctx, span := tracer.Start(ctx, "kafka.produce",
		trace.WithSpanKind(trace.SpanKindProducer),
	)
	su_ytrace.MaybeSetRequestID(ctx, span)

	span.SetAttributes(
		attribute.String("messaging.system", "kafka"),
		attribute.String("messaging.operation", "publish"),
		attribute.String("messaging.destination", topic),
	)

	if msg != nil {
		if msg.Key != nil {
			span.SetAttributes(attribute.String("messaging.kafka.message_key", string(msg.Key)))
		}
		if msg.TopicPartition.Partition >= 0 {
			span.SetAttributes(attribute.Int("messaging.kafka.partition", int(msg.TopicPartition.Partition)))
		}
	}

	// 注入 TraceContext
	InjectKafkaTrace(ctx, msg)

	return ctx, span
}

// StartKafkaConsumerSpan 创建 Kafka Consumer Span
func StartKafkaConsumerSpan(ctx context.Context, topic string, msg *kafka.Message) (context.Context, trace.Span) {
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		// 追踪未启用，返回空 span
		return ctx, trace.SpanFromContext(ctx)
	}

	// 提取 span（支持 Worker 和 Fiber 服务）
	ctx = su_ytrace.ExtractSpanFromContext(ctx)

	// 先提取 TraceContext
	ctx = ExtractKafkaTrace(ctx, msg)

	tracer := otel.Tracer(kafkaTracerName)

	ctx, span := tracer.Start(ctx, "kafka.consume",
		trace.WithSpanKind(trace.SpanKindConsumer),
	)
	su_ytrace.MaybeSetRequestID(ctx, span)

	span.SetAttributes(
		attribute.String("messaging.system", "kafka"),
		attribute.String("messaging.operation", "receive"),
		attribute.String("messaging.destination", topic),
	)

	if msg != nil {
		if msg.Key != nil {
			span.SetAttributes(attribute.String("messaging.kafka.message_key", string(msg.Key)))
		}
		span.SetAttributes(
			attribute.Int("messaging.kafka.partition", int(msg.TopicPartition.Partition)),
			attribute.Int64("messaging.kafka.offset", int64(msg.TopicPartition.Offset)),
		)
	}

	return ctx, span
}

// EndSpanWithError 结束 span 并记录错误（如果有）
func EndSpanWithError(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	su_ytrace.SafeEndSpan(span) // 使用安全的 End 方法
}
