package middleware

import (
	"context"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/enum"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_ytrace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	fiberTracerName = "seago/utils/su_fiber"
)

// OtelTracingConfig OTEL 追踪中间件配置
type OtelTracingConfig struct {
	// Skipper 跳过追踪的路径
	Skipper func(*fiber.Ctx) bool

	// SpanNameFormatter 自定义 span 名称
	SpanNameFormatter func(*fiber.Ctx) string
}

// DefaultOtelTracingConfig 默认配置
var DefaultOtelTracingConfig = OtelTracingConfig{
	Skipper: func(c *fiber.Ctx) bool {
		// 默认不跳过任何路径
		return false
	},
	SpanNameFormatter: func(c *fiber.Ctx) string {
		// 默认使用 "HTTP Method Path" 格式
		return c.Method() + " " + c.OriginalURL()
	},
}

// OtelTracing OpenTelemetry 追踪中间件
func OtelTracing(config ...OtelTracingConfig) fiber.Handler {
	cfg := DefaultOtelTracingConfig
	if len(config) > 0 {
		cfg = config[0]
		if cfg.Skipper == nil {
			cfg.Skipper = DefaultOtelTracingConfig.Skipper
		}
		if cfg.SpanNameFormatter == nil {
			cfg.SpanNameFormatter = DefaultOtelTracingConfig.SpanNameFormatter
		}
	}

	return func(c *fiber.Ctx) error {
		// 如果追踪未启用，直接执行
		if !su_ytrace.IsEnabled() {
			return c.Next()
		}

		// 检查是否跳过
		if cfg.Skipper(c) {
			return c.Next()
		}

		// 在请求时获取 tracer，确保追踪系统已初始化
		tracer := otel.Tracer(fiberTracerName)

		// 从 HTTP Headers 提取 TraceContext
		// 注意：这里使用 c.UserContext() 作为父 context，而不是 c.Context()
		// 因为业务代码会使用 c.Context()，而 c.Context() 返回的是 fasthttp.RequestCtx
		parentCtx := c.UserContext()
		if parentCtx == nil {
			parentCtx = context.Background()
		}

		extractedCtx := otel.GetTextMapPropagator().Extract(
			parentCtx,
			propagation.HeaderCarrier(c.GetReqHeaders()),
		)

		// 检查是否从 HTTP Header 提取到了有效的 TraceContext（跨服务调用场景）
		extractedSpanCtx := trace.SpanContextFromContext(extractedCtx)
		hasRemoteTrace := extractedSpanCtx.IsValid() && extractedSpanCtx.IsRemote()

		var ctx context.Context
		var span trace.Span
		spanName := cfg.SpanNameFormatter(c)

		// 1. 如果有远程 TraceContext（跨服务调用），直接使用，不覆盖
		// 2. 如果没有远程 TraceContext（第一个入口），尝试用 request_id 作为 TraceID
		if hasRemoteTrace {
			// 跨服务调用：直接使用提取的 TraceContext，保持连通
			ctx, span = tracer.Start(extractedCtx, spanName,
				trace.WithSpanKind(trace.SpanKindServer),
			)

			// 记录 request_id（如果有）作为额外属性
			if reqIDValue := c.Context().UserValue(enum.CtxRequestId); reqIDValue != nil {
				if requestID, ok := reqIDValue.(string); ok && requestID != "" {
					span.SetAttributes(attribute.String("request_id", requestID))
				}
			}
		} else {
			// 第一个入口：尝试使用 request_id 作为 TraceID
			if reqIDValue := c.Context().UserValue(enum.CtxRequestId); reqIDValue != nil {
				if requestID, ok := reqIDValue.(string); ok && requestID != "" {
					// 尝试将 request_id 转换为 TraceID
					if traceID, valid := su_ytrace.RequestIDToTraceID(requestID); valid {
						// 成功转换，创建新的 root span，使用 request_id 作为 TraceID
						spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
							TraceID:    traceID,
							SpanID:     trace.SpanID{}, // SpanID 会由 Start 自动生成
							TraceFlags: trace.FlagsSampled,
						})

						// 将 SpanContext 注入到 context 中
						ctxWithSpan := trace.ContextWithSpanContext(extractedCtx, spanCtx)

						// 创建 root span
						ctx, span = tracer.Start(ctxWithSpan, spanName,
							trace.WithSpanKind(trace.SpanKindServer),
						)

						// 记录 request_id 属性
						span.SetAttributes(attribute.String("request_id", requestID))
					} else {
						// request_id 格式不正确，使用默认方式
						ctx, span = tracer.Start(extractedCtx, spanName,
							trace.WithSpanKind(trace.SpanKindServer),
						)
						// 记录 request_id 但不作为 TraceID
						span.SetAttributes(attribute.String("request_id", requestID))
					}
				} else {
					// request_id 不是 string 或为空，使用默认方式
					ctx, span = tracer.Start(extractedCtx, spanName,
						trace.WithSpanKind(trace.SpanKindServer),
					)
				}
			} else {
				// 没有 request_id，使用默认方式
				ctx, span = tracer.Start(extractedCtx, spanName,
					trace.WithSpanKind(trace.SpanKindServer),
				)
			}
		}
		defer su_ytrace.SafeEndSpan(span) // 使用安全的 End 方法

		// 设置 span 属性
		span.SetAttributes(
			semconv.HTTPMethod(c.Method()),
			semconv.HTTPTarget(c.OriginalURL()),
			semconv.HTTPScheme(c.Protocol()),
			semconv.NetHostName(c.Hostname()),
		)

		// 安全地设置路由路径（c.Route() 在未匹配路由时可能返回 nil）
		if route := c.Route(); route != nil {
			span.SetAttributes(semconv.HTTPRoute(route.Path))
		}

		// 获取客户端 IP
		if ip := c.IP(); ip != "" {
			span.SetAttributes(semconv.HTTPClientIP(ip))
		}

		// User-Agent
		if ua := c.Get("User-Agent"); ua != "" {
			span.SetAttributes(semconv.HTTPUserAgent(ua))
		}

		// 1. 设置 UserContext (用于 c.UserContext())
		c.SetUserContext(ctx)

		// 2. 将 span 注入到 fasthttp.RequestCtx (用于 c.Context())
		// 存储 span 对象，让下游 instrumentation 可以使用 trace.ContextWithSpan 重建标准 context
		reqCtx := c.Context()
		reqCtx.SetUserValue("otel-span", span)

		// 执行下一个处理器
		err := c.Next()

		// 记录响应状态
		statusCode := c.Response().StatusCode()
		span.SetAttributes(semconv.HTTPStatusCode(statusCode))

		// 根据状态码设置 span 状态
		if statusCode >= 400 {
			// 只记录状态码，不复制完整响应体
			span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", statusCode))
			if err != nil {
				span.RecordError(err)
			}
		} else {
			span.SetStatus(codes.Ok, "")
		}

		// 检测是否是流式响应（SSE 等使用 SetBodyStreamWriter 的场景）
		// 流式响应判断条件：
		// 1. BodyStream() 返回非 nil（使用了 SetBodyStreamWriter）
		// 2. Content-Type 为 text/event-stream（SSE）
		// 3. Transfer-Encoding 包含 chunked
		contentType := string(c.Response().Header.ContentType())
		isStreaming := c.Response().BodyStream() != nil ||
			contentType == "text/event-stream" ||
			strings.Contains(contentType, "text/event-stream") ||
			strings.Contains(string(c.Response().Header.Peek("Transfer-Encoding")), "chunked")

		if !isStreaming {
			// 非流式响应：记录响应大小
			span.SetAttributes(
				attribute.Int("http.response.body.size", len(c.Response().Body())),
			)
		} else {
			// 流式响应：标记为 streaming，不读取 body
			span.SetAttributes(
				attribute.Bool("http.response.streaming", true),
				attribute.String("http.response.content_type", contentType),
			)
		}

		return err
	}
}

// OtelTracingWithSkip 创建带跳过路径的追踪中间件
func OtelTracingWithSkip(skipPaths ...string) fiber.Handler {
	skipMap := make(map[string]bool, len(skipPaths))
	for _, path := range skipPaths {
		skipMap[path] = true
	}

	return OtelTracing(OtelTracingConfig{
		Skipper: func(c *fiber.Ctx) bool {
			return skipMap[c.Path()]
		},
	})
}
