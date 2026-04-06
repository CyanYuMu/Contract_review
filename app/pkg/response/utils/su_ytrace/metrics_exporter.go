package su_ytrace

import (
	"context"
	"log"
	"sync/atomic"
	"unicode/utf8"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// ExporterMetrics 导出器统计信息
type ExporterMetrics struct {
	// 导出成功的 span 数量
	ExportedSpans uint64
	// 导出失败的 span 数量
	FailedSpans uint64
	// 导出成功的批次数量
	ExportedBatches uint64
	// 导出失败的批次数量
	FailedBatches uint64
}

// MetricsExporter 带统计功能的 SpanExporter 包装器
type MetricsExporter struct {
	delegate sdktrace.SpanExporter
	metrics  ExporterMetrics

	// 配置选项
	logEnabled   bool // 是否打印日志
	logSpanInfo  bool // 是否打印 span 详情
	serviceName  string
}

// MetricsExporterOption 配置选项
type MetricsExporterOption func(*MetricsExporter)

// WithLogging 启用日志
func WithLogging(enabled bool) MetricsExporterOption {
	return func(e *MetricsExporter) {
		e.logEnabled = enabled
	}
}

// WithSpanInfo 启用 span 详情日志
func WithSpanInfo(enabled bool) MetricsExporterOption {
	return func(e *MetricsExporter) {
		e.logSpanInfo = enabled
	}
}

// WithServiceName 设置服务名称（用于日志前缀）
func WithServiceName(name string) MetricsExporterOption {
	return func(e *MetricsExporter) {
		e.serviceName = name
	}
}

// NewMetricsExporter 创建带统计功能的 exporter
func NewMetricsExporter(delegate sdktrace.SpanExporter, opts ...MetricsExporterOption) *MetricsExporter {
	e := &MetricsExporter{
		delegate:    delegate,
		logEnabled:  true, // 默认启用日志
		logSpanInfo: false,
	}

	for _, opt := range opts {
		opt(e)
	}

	return e
}

// ExportSpans 实现 SpanExporter 接口
func (e *MetricsExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	count := len(spans)
	if count == 0 {
		return nil
	}

	// 打印导出前的日志
	if e.logEnabled {
		log.Printf("[ytrace] exporting spans: count=%d, total_exported=%d, total_failed=%d, batches_ok=%d, batches_fail=%d",
			count,
			atomic.LoadUint64(&e.metrics.ExportedSpans),
			atomic.LoadUint64(&e.metrics.FailedSpans),
			atomic.LoadUint64(&e.metrics.ExportedBatches),
			atomic.LoadUint64(&e.metrics.FailedBatches),
		)
	}

	// 打印每个 span 的简要信息
	if e.logSpanInfo {
		for i, span := range spans {
			e.logSpan(i, span)
		}
	}

	// 调用实际的 exporter
	err := e.delegate.ExportSpans(ctx, spans)

	if err != nil {
		atomic.AddUint64(&e.metrics.FailedSpans, uint64(count))
		atomic.AddUint64(&e.metrics.FailedBatches, 1)

		if e.logEnabled {
			log.Printf("[ytrace] export FAILED: err=%v, dropped_spans=%d", err, count)
		}
		return err
	}

	atomic.AddUint64(&e.metrics.ExportedSpans, uint64(count))
	atomic.AddUint64(&e.metrics.ExportedBatches, 1)

	if e.logEnabled {
		log.Printf("[ytrace] export SUCCESS: spans=%d", count)
	}

	return nil
}

// Shutdown 实现 SpanExporter 接口
func (e *MetricsExporter) Shutdown(ctx context.Context) error {
	if e.logEnabled {
		log.Printf("[ytrace] shutdown: total_exported=%d, total_failed=%d, batches_ok=%d, batches_fail=%d",
			atomic.LoadUint64(&e.metrics.ExportedSpans),
			atomic.LoadUint64(&e.metrics.FailedSpans),
			atomic.LoadUint64(&e.metrics.ExportedBatches),
			atomic.LoadUint64(&e.metrics.FailedBatches),
		)
	}
	return e.delegate.Shutdown(ctx)
}

// GetMetrics 获取统计信息
func (e *MetricsExporter) GetMetrics() ExporterMetrics {
	return ExporterMetrics{
		ExportedSpans:   atomic.LoadUint64(&e.metrics.ExportedSpans),
		FailedSpans:     atomic.LoadUint64(&e.metrics.FailedSpans),
		ExportedBatches: atomic.LoadUint64(&e.metrics.ExportedBatches),
		FailedBatches:   atomic.LoadUint64(&e.metrics.FailedBatches),
	}
}

// logSpan 打印单个 span 的信息
func (e *MetricsExporter) logSpan(index int, span sdktrace.ReadOnlySpan) {
	duration := span.EndTime().Sub(span.StartTime())

	// 收集 attributes（限制长度，避免日志过大）
	var attrsPreview string
	for i, attr := range span.Attributes() {
		if i >= 3 {
			attrsPreview += "..."
			break
		}
		if i > 0 {
			attrsPreview += ", "
		}
		// 截断过长的值
		val := attr.Value.Emit()
		if len(val) > 50 {
			val = val[:50] + "..."
		}
		// 检查 UTF-8 有效性
		if !utf8.ValidString(val) {
			val = "[invalid UTF-8]"
		}
		attrsPreview += string(attr.Key) + "=" + val
	}

	log.Printf("[ytrace] span[%d]: name=%q, trace_id=%s, duration=%v, status=%s, attrs={%s}",
		index,
		span.Name(),
		span.SpanContext().TraceID().String(),
		duration,
		span.Status().Code.String(),
		attrsPreview,
	)
}

// sanitizeString 清理非 UTF-8 字符
func sanitizeString(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	// 替换无效字符
	v := make([]rune, 0, len(s))
	for i, r := range s {
		if r == utf8.RuneError {
			_, size := utf8.DecodeRuneInString(s[i:])
			if size == 1 {
				v = append(v, '\uFFFD') // 替换为 Unicode 替换字符
				continue
			}
		}
		v = append(v, r)
	}
	return string(v)
}
