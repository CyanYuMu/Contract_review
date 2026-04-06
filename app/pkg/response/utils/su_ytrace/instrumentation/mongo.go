package instrumentation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_ytrace"
	"go.mongodb.org/mongo-driver/event"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	mongoTracerName = "seago/utils/su_ytrace/mongo"
	// 最大 span 保留时间
	maxSpanRetention = 5 * time.Minute
)

// spanEntry 带时间戳的 span 条目
type spanEntry struct {
	span      trace.Span
	createdAt time.Time
}

// MongoMonitor MongoDB 追踪 Monitor
type MongoMonitor struct {
	addr        string
	spans       sync.Map // map[int64]*spanEntry，使用 sync.Map 保证并发安全
	stopCleanup chan struct{}
	cleanupOnce sync.Once
	stopOnce    sync.Once // 确保 Stop() 只执行一次
}

// NewMongoMonitor 创建 MongoDB Monitor
// addr: MongoDB 连接地址，如 "mongodb://localhost:27017"
// 返回 *MongoMonitor，业务方可以调用 Stop()
//
// 注意：清理 goroutine 采用延迟启动策略，只有在追踪启用且有 span 创建时才会启动
// 这样可以避免在未开启 ytrace 的项目中产生不必要的 goroutine
func NewMongoMonitor(addr string) *MongoMonitor {
	m := &MongoMonitor{
		addr:        addr,
		stopCleanup: make(chan struct{}),
	}

	// 启动后台清理 goroutine
	// m.startCleanupRoutine()

	return m
}

// CommandMonitor 获取 MongoDB CommandMonitor 接口
// 用于设置到 MongoDB 客户端配置中
func (m *MongoMonitor) CommandMonitor() *event.CommandMonitor {
	return &event.CommandMonitor{
		Started:   m.Started,
		Succeeded: m.Succeeded,
		Failed:    m.Failed,
	}
}

// startCleanupRoutine 启动后台清理超时的 span
func (m *MongoMonitor) startCleanupRoutine() {
	m.cleanupOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(1 * time.Minute)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					m.cleanupStaleSpans()
				case <-m.stopCleanup:
					return
				}
			}
		}()
	})
}

// cleanupStaleSpans 清理超时的 span
func (m *MongoMonitor) cleanupStaleSpans() {
	now := time.Now()
	var staleKeys []int64

	// 找出所有超时的 span
	m.spans.Range(func(key, value interface{}) bool {
		requestID := key.(int64)
		entry := value.(*spanEntry)

		if now.Sub(entry.createdAt) > maxSpanRetention {
			staleKeys = append(staleKeys, requestID)
		}
		return true
	})

	// 清理超时的 span
	for _, requestID := range staleKeys {
		if value, loaded := m.spans.LoadAndDelete(requestID); loaded {
			entry := value.(*spanEntry)

			// 记录警告日志：发现孤立的 span，可能是 MongoDB 命令异常
			su_logger.Warn(context.Background(),
				"MongoDB span orphaned and cleaned up",
				su_logger.E().
					Int64("request_id", requestID).
					Float64("age_seconds", now.Sub(entry.createdAt).Seconds()).
					String("hint", "MongoDB command may have timed out or failed to trigger Succeeded/Failed callback"),
			)

			// 设置超时错误并结束 span
			entry.span.SetStatus(codes.Error, "mongodb command timeout or orphaned")
			entry.span.SetAttributes(attribute.Bool("db.mongodb.span_orphaned", true))
			su_ytrace.SafeEndSpan(entry.span) // 使用安全的 End 方法
		}
	}
}

// Stop 停止清理 goroutine（业务方在关闭 MongoDB 连接时必须调用）
// 使用 stopOnce 确保多次调用 Stop() 不会 panic
func (m *MongoMonitor) Stop() {
	m.stopOnce.Do(func() {
		close(m.stopCleanup)

		// 清理所有剩余的 span（防止内存泄露）
		m.spans.Range(func(key, value interface{}) bool {
			if entry, ok := value.(*spanEntry); ok {
				entry.span.SetStatus(codes.Error, "mongodb monitor stopped")
				entry.span.SetAttributes(attribute.Bool("db.mongodb.monitor_stopped", true))
				su_ytrace.SafeEndSpan(entry.span) // 使用安全的 End 方法
			}
			m.spans.Delete(key)
			return true
		})
	})
}

// getTracer 获取 tracer（每次操作时获取，确保追踪系统已初始化）
func (m *MongoMonitor) getTracer() trace.Tracer {
	return otel.Tracer(mongoTracerName)
}

// Started 命令开始时调用
func (m *MongoMonitor) Started(ctx context.Context, evt *event.CommandStartedEvent) {
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		// 追踪未启用，直接返回
		return
	}

	// 延迟启动清理 goroutine（只有在追踪启用且首次创建 span 时才启动）
	// 使用 sync.Once 确保只启动一次
	m.startCleanupRoutine()

	tracer := m.getTracer()

	// 提取 span（支持 Worker 和 Fiber 服务）
	// 使用统一的 ExtractSpanFromContext 替代手动提取逻辑
	ctx = su_ytrace.ExtractSpanFromContext(ctx)

	ctx, span := tracer.Start(ctx, fmt.Sprintf("mongo.%s", evt.CommandName),
		trace.WithSpanKind(trace.SpanKindClient),
	)
	su_ytrace.MaybeSetRequestID(ctx, span)

	span.SetAttributes(
		attribute.String("db.system", "mongodb"),
		attribute.String("db.connection_string", m.addr),
		attribute.String("db.operation", evt.CommandName),
		attribute.String("db.name", evt.DatabaseName),
		attribute.Int64("db.mongodb.request_id", evt.RequestID),
	)

	// 尝试提取 collection 名称
	if collName, ok := evt.Command.Lookup("collection").StringValueOK(); ok {
		span.SetAttributes(attribute.String("db.mongodb.collection", collName))
	} else if collName, ok := evt.Command.Lookup("find").StringValueOK(); ok {
		span.SetAttributes(attribute.String("db.mongodb.collection", collName))
	} else if collName, ok := evt.Command.Lookup("insert").StringValueOK(); ok {
		span.SetAttributes(attribute.String("db.mongodb.collection", collName))
	} else if collName, ok := evt.Command.Lookup("update").StringValueOK(); ok {
		span.SetAttributes(attribute.String("db.mongodb.collection", collName))
	} else if collName, ok := evt.Command.Lookup("delete").StringValueOK(); ok {
		span.SetAttributes(attribute.String("db.mongodb.collection", collName))
	}

	span.SetAttributes(attribute.String("db.statement", evt.Command.String()))

	// 使用 sync.Map 存储，带时间戳
	m.spans.Store(evt.RequestID, &spanEntry{
		span:      span,
		createdAt: time.Now(),
	})
}

// Succeeded 命令成功时调用
func (m *MongoMonitor) Succeeded(ctx context.Context, evt *event.CommandSucceededEvent) {
	// ！！！ 此处不要校验是否启用，勿动
	if value, loaded := m.spans.LoadAndDelete(evt.RequestID); loaded {
		entry := value.(*spanEntry)
		entry.span.SetAttributes(
			attribute.Int64("db.mongodb.duration_ms", evt.DurationNanos/1000000),
		)
		entry.span.SetStatus(codes.Ok, "")
		su_ytrace.SafeEndSpan(entry.span) // 使用安全的 End 方法
	}
}

// Failed 命令失败时调用
func (m *MongoMonitor) Failed(ctx context.Context, evt *event.CommandFailedEvent) {
	// ！！！ 此处不要校验是否启用，勿动
	if value, loaded := m.spans.LoadAndDelete(evt.RequestID); loaded {
		entry := value.(*spanEntry)
		entry.span.SetAttributes(
			attribute.Int64("db.mongodb.duration_ms", evt.DurationNanos/1000000),
		)
		entry.span.RecordError(fmt.Errorf("%s", evt.Failure))
		entry.span.SetStatus(codes.Error, evt.Failure)
		su_ytrace.SafeEndSpan(entry.span) // 使用安全的 End 方法
	}
}
