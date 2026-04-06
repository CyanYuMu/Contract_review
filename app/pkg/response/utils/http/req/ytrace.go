package req

import (
	"context"
	"net/http"
	"strconv"
	"sync"

	reqv3 "github.com/imroc/req/v3"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_ytrace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	httpClientTracerName = "seago/utils/http/req"
)

// TracingRoundTripper HTTP Client 追踪 RoundTripper
type TracingRoundTripper struct {
	next   http.RoundTripper
	tracer trace.Tracer
}

var (
	tracedReqClients  sync.Map // map[*reqv3.Client]struct{}
	tracedHTTPClients sync.Map // map[*http.Client]struct{}
)

// NewTracingRoundTripper 创建追踪 RoundTripper
func NewTracingRoundTripper(next http.RoundTripper) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}

	return &TracingRoundTripper{
		next: next,
	}
}

// getTracer 获取 tracer（延迟获取，确保追踪系统已初始化）
func (t *TracingRoundTripper) getTracer() trace.Tracer {
	return otel.Tracer(httpClientTracerName)
}

// RoundTrip 实现 http.RoundTripper 接口
func (t *TracingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()

	// 如果追踪未启用，直接执行请求
	if !su_ytrace.IsEnabled() {
		return t.next.RoundTrip(req)
	}

	// 提取父 span（支持 Worker 和 Fiber 服务）
	ctx = su_ytrace.ExtractSpanFromContext(ctx)

	// 创建 span
	tracer := t.getTracer()
	spanName := "HTTP " + req.Method
	ctx, span := tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer su_ytrace.SafeEndSpan(span)
	su_ytrace.MaybeSetRequestID(ctx, span)

	// 设置 span 属性
	span.SetAttributes(
		semconv.HTTPMethod(req.Method),
		semconv.HTTPURL(req.URL.String()),
		semconv.HTTPScheme(req.URL.Scheme),
		semconv.NetPeerName(req.URL.Hostname()),
	)

	if portStr := req.URL.Port(); portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil {
			span.SetAttributes(semconv.NetPeerPort(port))
		}
	}

	// 注入 TraceContext 到 HTTP Headers
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	// 执行请求
	req = req.WithContext(ctx)
	resp, err := t.next.RoundTrip(req)

	// 记录响应
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return resp, err
	}

	span.SetAttributes(semconv.HTTPStatusCode(resp.StatusCode))

	if resp.StatusCode >= 400 {
		span.SetStatus(codes.Error, resp.Status)
	} else {
		span.SetStatus(codes.Ok, "")
	}

	return resp, nil
}

// WithTracingRoundTripper 为 Client 配置追踪 RoundTripper
func (c *Client) WithTracingRoundTripper() *Client {
	if c == nil {
		return c
	}

	// 方案一：使用 req 库的 Transport 中间件（推荐）
	if c.c != nil {
		if _, loaded := tracedReqClients.LoadOrStore(c.c, struct{}{}); !loaded {
			if transport := c.c.GetTransport(); transport != nil {
				transport.WrapRoundTripFunc(func(rt http.RoundTripper) reqv3.HttpRoundTripFunc {
					tracingRT := NewTracingRoundTripper(rt)
					return reqv3.HttpRoundTripFunc(tracingRT.RoundTrip)
				})
			}
		}
	}

	// 方案二：为原生 httpClient 设置 RoundTripper
	if c.httpClient != nil {
		if _, loaded := tracedHTTPClients.LoadOrStore(c.httpClient, struct{}{}); !loaded {
			c.httpClient.Transport = NewTracingRoundTripper(c.httpClient.Transport)
		}
	}

	return c
}

// NewWithTracing 创建带追踪的 HTTP Client
func NewWithTracing(ctx context.Context) *Client {
	client := New(ctx)
	return client.WithTracingRoundTripper()
}
