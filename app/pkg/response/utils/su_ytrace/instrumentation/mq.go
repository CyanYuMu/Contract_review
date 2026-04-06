package instrumentation

import (
	"context"
	"fmt"
	"sync"
	"unicode/utf8"

	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/mq"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_ytrace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	mqTracerName      = "seago/utils/su_ytrace/mq"
	maxPayloadPreview = 50 // 消息内容预览的最大字符数
)

// WrapMQ 为 mq.MQ 增加追踪能力
func WrapMQ(original mq.MQ, system string) mq.MQ {
	if original == nil {
		return nil
	}
	return &tracedMQ{
		underlying: original,
		system:     system,
	}
}

type tracedMQ struct {
	underlying mq.MQ
	system     string
}

func (t *tracedMQ) NewProducer(ctx context.Context, conf mq.ProducerConf) (mq.Producer, error) {
	producer, err := t.underlying.NewProducer(ctx, conf)
	if err != nil {
		return producer, err
	}
	return &tracedProducer{
		underlying: producer,
		system:     t.system,
		topic:      conf.Topic,
		tracer:     otel.Tracer(mqTracerName),
	}, nil
}

func (t *tracedMQ) NewConsumer(ctx context.Context, conf mq.ConsumerConf) (mq.Consumer, error) {
	consumer, err := t.underlying.NewConsumer(ctx, conf)
	if err != nil {
		return consumer, err
	}
	return &tracedConsumer{
		underlying: consumer,
		system:     t.system,
		topic:      conf.Topic,
		group:      conf.Group,
		tracer:     otel.Tracer(mqTracerName),
	}, nil
}

type tracedProducer struct {
	underlying mq.Producer
	system     string
	topic      string
	tracer     trace.Tracer
}

// truncatePayload 截取 payload 前 N 个字符作为预览
//  1. 只处理前 maxChars 个 UTF-8 字符，不转换整个 payload
//  2. 正确处理 UTF-8 多字节字符边界，避免乱码
//  3. 对于大 payload（如 10MB），只扫描前几百字节
func truncatePayload(payload []byte, maxChars int) string {
	if len(payload) == 0 {
		return ""
	}

	if len(payload) <= maxChars {
		return string(payload)
	}

	var charCount int
	var byteIndex int

	for byteIndex < len(payload) && charCount < maxChars {
		// 解码一个 UTF-8 字符（rune）
		_, size := utf8.DecodeRune(payload[byteIndex:])
		if size == 0 {
			// 遇到无效的 UTF-8 序列，停止
			break
		}
		byteIndex += size
		charCount++
	}

	// 只转换前 byteIndex 个字节
	return string(payload[:byteIndex])
}

func (p *tracedProducer) Send(ctx context.Context, msg *mq.ProducerMessage) error {
	if !su_ytrace.IsEnabled() {
		return p.underlying.Send(ctx, msg)
	}

	ctx = su_ytrace.ExtractSpanFromContext(ctx)
	ctx, span := p.tracer.Start(ctx, fmt.Sprintf("%s.produce", p.system),
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", p.system),
			attribute.String("messaging.operation", "publish"),
			attribute.String("messaging.destination", p.topic),
		),
	)
	defer su_ytrace.SafeEndSpan(span) // 使用安全的 End 方法
	su_ytrace.MaybeSetRequestID(ctx, span)

	// 记录消息内容预览
	if msg != nil && len(msg.Payload) > 0 {
		preview := truncatePayload(msg.Payload, maxPayloadPreview)
		span.SetAttributes(attribute.String("messaging.message.payload_preview", preview))
	}

	p.injectCarrier(ctx, msg)

	err := p.underlying.Send(ctx, msg)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	return err
}

func (p *tracedProducer) BatchSend(ctx context.Context, msgList []*mq.ProducerMessage, forceFlush bool) error {
	if !su_ytrace.IsEnabled() {
		return p.underlying.BatchSend(ctx, msgList, forceFlush)
	}

	ctx = su_ytrace.ExtractSpanFromContext(ctx)
	ctx, span := p.tracer.Start(ctx, fmt.Sprintf("%s.batch_produce", p.system),
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", p.system),
			attribute.String("messaging.operation", "batch_publish"),
			attribute.String("messaging.destination", p.topic),
			attribute.Int("messaging.batch.message_count", len(msgList)),
		),
	)
	defer su_ytrace.SafeEndSpan(span) // 使用安全的 End 方法
	su_ytrace.MaybeSetRequestID(ctx, span)

	// 记录第一条消息的内容预览（批量发送时）
	if len(msgList) > 0 && msgList[0] != nil && len(msgList[0].Payload) > 0 {
		preview := truncatePayload(msgList[0].Payload, maxPayloadPreview)
		span.SetAttributes(attribute.String("messaging.message.payload_preview", preview))
	}

	for _, msg := range msgList {
		p.injectCarrier(ctx, msg)
	}

	err := p.underlying.BatchSend(ctx, msgList, forceFlush)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	return err
}

func (p *tracedProducer) AsyncSend(ctx context.Context, msg *mq.ProducerMessage, handler mq.AsyncMsgHandler) error {
	if !su_ytrace.IsEnabled() {
		return p.underlying.AsyncSend(ctx, msg, handler)
	}

	ctx = su_ytrace.ExtractSpanFromContext(ctx)
	ctx, span := p.tracer.Start(ctx, fmt.Sprintf("%s.async_produce", p.system),
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", p.system),
			attribute.String("messaging.operation", "async_publish"),
			attribute.String("messaging.destination", p.topic),
		),
	)
	su_ytrace.MaybeSetRequestID(ctx, span)

	// 记录消息内容预览
	if msg != nil && len(msg.Payload) > 0 {
		preview := truncatePayload(msg.Payload, maxPayloadPreview)
		span.SetAttributes(attribute.String("messaging.message.payload_preview", preview))
	}

	p.injectCarrier(ctx, msg)

	// 使用标志位确保 span 只被 End 一次
	var spanEnded bool
	var mu sync.Mutex

	wrappedHandler := func(cbCtx context.Context, msgID string, msg *mq.ProducerMessage, err error) {
		mu.Lock()
		defer mu.Unlock()
		if !spanEnded {
			defer su_ytrace.SafeEndSpan(span) // 使用安全的 End 方法
			spanEnded = true
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			} else {
				span.SetStatus(codes.Ok, "")
			}
		}
		if handler != nil {
			handler(cbCtx, msgID, msg, err)
		}
	}

	err := p.underlying.AsyncSend(ctx, msg, wrappedHandler)
	if err != nil {
		mu.Lock()
		if !spanEnded {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			su_ytrace.SafeEndSpan(span) // 使用安全的 End 方法
			spanEnded = true
		}
		mu.Unlock()
	}
	return err
}

func (p *tracedProducer) Flush() error {
	return p.underlying.Flush()
}

func (p *tracedProducer) injectCarrier(ctx context.Context, msg *mq.ProducerMessage) {
	if msg == nil {
		return
	}
	if msg.Attributes == nil {
		msg.Attributes = make(map[string]string)
	}
	carrier := su_ytrace.TextMapCarrier(msg.Attributes)
	su_ytrace.InjectContext(ctx, carrier)
	if len(msg.Payload) > 0 {
		msg.Attributes["messaging.payload_size"] = fmt.Sprintf("%d", len(msg.Payload))
	}
}

type tracedConsumer struct {
	underlying mq.Consumer
	system     string
	topic      string
	group      string
	tracer     trace.Tracer
}

func (c *tracedConsumer) Consume(ctx context.Context, handler mq.MsgHandler) error {
	if !su_ytrace.IsEnabled() {
		return c.underlying.Consume(ctx, handler)
	}

	wrapped := func(msgCtx context.Context, msg *mq.ConsumerMessage) error {
		if msgCtx == nil {
			msgCtx = context.Background()
		}
		if msg != nil && msg.Attributes != nil {
			msgCtx = su_ytrace.ExtractContext(msgCtx, su_ytrace.TextMapCarrier(msg.Attributes))
		}
		msgCtx = su_ytrace.ExtractSpanFromContext(msgCtx)

		spanName := fmt.Sprintf("%s.consume", c.system)
		msgCtx, span := c.tracer.Start(msgCtx, spanName,
			trace.WithSpanKind(trace.SpanKindConsumer),
			trace.WithAttributes(
				attribute.String("messaging.system", c.system),
				attribute.String("messaging.operation", "receive"),
				attribute.String("messaging.destination", c.topic),
				attribute.String("messaging.consumer.group", c.group),
			),
		)
		su_ytrace.MaybeSetRequestID(msgCtx, span)
		if msg != nil {
			if msg.ID != "" {
				span.SetAttributes(attribute.String("messaging.message_id", msg.ID))
			}
			if len(msg.Payload) > 0 {
				span.SetAttributes(attribute.Int("messaging.message.payload_size", len(msg.Payload)))
				// 记录消息内容预览
				preview := truncatePayload(msg.Payload, maxPayloadPreview)
				span.SetAttributes(attribute.String("messaging.message.payload_preview", preview))
			}
		}

		err := handler(msgCtx, msg)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}
		su_ytrace.SafeEndSpan(span) // 使用安全的 End 方法
		return err
	}

	return c.underlying.Consume(ctx, wrapped)
}
