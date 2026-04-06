package instrumentation

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_ytrace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	esTracerName = "seago/utils/su_ytrace/elasticsearch"
)

// ElasticsearchTransport 包装 http.RoundTripper 以添加 OpenTelemetry 追踪
type ElasticsearchTransport struct {
	transport http.RoundTripper
	addresses []string
}

// NewElasticsearchTransport 创建 Elasticsearch 追踪 Transport
// transport: 底层的 http.RoundTripper
// addresses: ES 集群地址列表
func NewElasticsearchTransport(transport http.RoundTripper, addresses []string) *ElasticsearchTransport {
	return &ElasticsearchTransport{
		transport: transport,
		addresses: addresses,
	}
}

// RoundTrip 实现 http.RoundTripper 接口
func (t *ElasticsearchTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()

	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		// 追踪未启用，直接执行请求
		return t.transport.RoundTrip(req)
	}

	// 提取 span（支持 Worker 和 Fiber 服务）
	ctx = su_ytrace.ExtractSpanFromContext(ctx)

	tracer := otel.Tracer(esTracerName)

	// 从 URL 中提取操作信息
	operation, index := t.parseOperation(req)

	ctx, span := tracer.Start(ctx, fmt.Sprintf("elasticsearch.%s", operation),
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer su_ytrace.SafeEndSpan(span) // 使用安全的 End 方法
	su_ytrace.MaybeSetRequestID(ctx, span)

	// 读取请求体（DSL）
	// 只记录前 500 字符用于调试，避免占用过多内存
	const maxDSLPreview = 500 // 500 字符（约 500 bytes）
	var dsl string
	var dslSize int
	var dslTruncated bool

	// 优先使用 GetBody（避免缓冲整个请求体到内存）
	if req.GetBody != nil {
		// GetBody 允许安全地重复读取请求体，无需缓冲
		bodyReader, err := req.GetBody()
		if err == nil {
			defer bodyReader.Close()
			// 只读取前 500 字符用于预览
			preview := make([]byte, maxDSLPreview)
			n, readErr := io.ReadFull(bodyReader, preview)
			if readErr == io.EOF || errors.Is(readErr, io.ErrUnexpectedEOF) || readErr == nil {
				if n > 0 {
					dsl = string(preview[:n])
					if n == maxDSLPreview {
						dslTruncated = true
					}
				}
			}
		}
		// 原始 req.Body 保持不变，可以被底层 transport 正常使用
	} else if req.Body != nil && req.Body != http.NoBody {
		// 回退方案：如果没有 GetBody，使用 TeeReader（会缓冲到内存）
		var buf bytes.Buffer
		tee := io.TeeReader(req.Body, &buf)
		preview := make([]byte, maxDSLPreview)
		n, err := io.ReadFull(tee, preview)
		if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) || err == nil {
			if n > 0 {
				dsl = string(preview[:n])
				if n == maxDSLPreview {
					dslTruncated = true
				}
			}
		}
		// 读取剩余部分到 buf
		io.Copy(&buf, req.Body)
		dslSize = buf.Len()
		// 恢复完整的 Body
		req.Body = io.NopCloser(&buf)
	}

	attrs := []attribute.KeyValue{
		attribute.String("db.system", "elasticsearch"),
		attribute.String("db.operation", operation),
		attribute.String("es.method", req.Method),
		attribute.String("es.url.path", req.URL.Path),
	}

	// 记录索引名称
	if index != "" {
		attrs = append(attrs, attribute.String("es.index", index))
	}

	// 记录 ES 集群地址
	if len(t.addresses) > 0 {
		attrs = append(attrs, attribute.String("es.addresses", strings.Join(t.addresses, ",")))
	}

	// 记录 DSL（限制长度避免日志过大）
	if dsl != "" {
		// 压缩 DSL：移除多余空格和换行
		compactDSL := compactJSON(dsl)
		if dslTruncated {
			// DSL 被截断，添加说明
			attrs = append(attrs,
				attribute.String("es.dsl", compactDSL+"..."),
				attribute.String("es.dsl.note", "preview only (first 500 chars)"),
			)
			if dslSize > 0 {
				attrs = append(attrs, attribute.Int("es.dsl.size", dslSize))
			}
		} else {
			attrs = append(attrs, attribute.String("es.dsl", compactDSL))
			if dslSize > 0 {
				attrs = append(attrs, attribute.Int("es.dsl.size", dslSize))
			}
		}
	}

	span.SetAttributes(attrs...)

	// 更新请求的 context
	req = req.WithContext(ctx)

	// 执行请求
	resp, err := t.transport.RoundTrip(req)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return resp, err
	}

	// 记录响应信息
	span.SetAttributes(
		attribute.Int("http.status_code", resp.StatusCode),
	)

	if resp.StatusCode >= 400 {
		span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", resp.StatusCode))
	} else {
		span.SetStatus(codes.Ok, "")
	}

	return resp, nil
}

// parseOperation 从请求 URL 中解析操作类型和索引名称
func (t *ElasticsearchTransport) parseOperation(req *http.Request) (operation string, index string) {
	path := req.URL.Path
	parts := strings.Split(strings.Trim(path, "/"), "/")

	// 常见的 ES API 路径模式：
	// /_search -> search (所有索引)
	// /index/_search -> search
	// /index/_doc/id -> get/index/update/delete
	// /index/_bulk -> bulk
	// /_bulk -> bulk
	// /index/_count -> count

	if len(parts) == 0 {
		return "unknown", ""
	}

	// 查找操作关键字（以 _ 开头）
	for i, part := range parts {
		if strings.HasPrefix(part, "_") {
			operation = strings.TrimPrefix(part, "_")
			// 获取索引名称（通常在操作关键字前面）
			if i > 0 && !strings.HasPrefix(parts[i-1], "_") {
				index = parts[i-1]
			}
			return
		}
	}

	// 如果没有找到操作关键字，根据 HTTP 方法判断
	switch req.Method {
	case "GET":
		operation = "get"
	case "POST":
		operation = "index"
	case "PUT":
		operation = "update"
	case "DELETE":
		operation = "delete"
	default:
		operation = strings.ToLower(req.Method)
	}

	// 尝试获取索引名称（第一个不以 _ 开头的部分）
	if len(parts) > 0 && !strings.HasPrefix(parts[0], "_") {
		index = parts[0]
	}

	return
}

// compactJSON 压缩 JSON 字符串（移除多余空格和换行）
func compactJSON(s string) string {
	if len(s) == 0 {
		return s
	}

	result := make([]byte, 0, len(s))
	prevSpace := false

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\n', '\r', '\t':
			// 跳过换行和制表符
			continue
		case ' ':
			// 保留单个空格，跳过连续空格
			if !prevSpace {
				result = append(result, c)
			}
			prevSpace = true
		default:
			result = append(result, c)
			prevSpace = false
		}
	}

	return string(result)
}
