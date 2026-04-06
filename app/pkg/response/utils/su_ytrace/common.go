package su_ytrace

import (
	"context"
	"encoding/hex"
	"log"
	"runtime"
	"sync"
	"sync/atomic"

	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/enum"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/trace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// SafeEndSpan 安全地结束 span，捕获可能的 panic
// 用于确保追踪代码不会影响业务逻辑
//
// 使用场景：
//   - 替代直接调用 span.End()
//   - 在 defer 语句中使用：defer su_ytrace.SafeEndSpan(span)
//
// 为什么需要这个函数：
//   - span.End() 内部可能因为 TracerProvider 状态问题导致 panic
//   - 特别是在服务 Shutdown 期间，已创建的 span 调用 End() 可能访问无效资源
//   - 我们不希望追踪代码的问题影响业务逻辑
func SafeEndSpan(span oteltrace.Span) {
	if span == nil {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			// 记录日志但不传播 panic，确保不影响业务
			log.Printf("[ERROR] ytrace: panic in span.End(): %v", r)
		}
	}()

	span.End()
}

// requestIDPending 用于在 context 中传递待设置的 request_id
// 使用互斥锁确保 request_id 只被设置一次（避免在嵌套 span 中重复设置）
type requestIDPending struct {
	requestID string
	mu        sync.Mutex
	used      bool
}

// trySet 尝试设置 request_id 到 span，只有第一次调用会成功
func (p *requestIDPending) trySet(span oteltrace.Span) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.used {
		return false // 已经使用过
	}

	span.SetAttributes(attribute.String("request_id", p.requestID))
	p.used = true
	return true
}

// requestIDPendingKey 是在 context 中存储 requestIDPending 的 key
type contextKey string

const requestIDPendingKey contextKey = "su_ytrace_request_id_pending"

// spanCleanup 用于 Finalizer 的 span 清理对象
// 当 cleanup 函数未被调用时，GC 会触发 finalizer 作为安全网
type spanCleanup struct {
	span      oteltrace.Span
	name      string
	requestID string
	cleaned   atomic.Bool // 使用原子操作标记是否已清理（避免重复 End 和数据竞争）
}

// NewRequestContext 为新任务创建独立的 request_id 和 root span
// 专门用于 Worker 服务的任务入口点（消息处理、定时任务等）
//
// 如果 ctx 中已存在 request_id（通过 trace.ParseFromContext 获取），则复用；
// 否则生成新的 request_id
//
// 使用场景：
//  1. Kafka/PubSub 消息处理的入口
//  2. 定时任务的入口
//  3. 后台任务的入口
//
// 使用示例：
//
//	// Kafka 消息处理，记录 task_id
//	func (w *Worker) ProcessMessage(msg *kafka.Message) error {
//	    ctx, cleanup := su_ytrace.NewRequestContext(
//	        context.Background(),
//	        "worker.kafka.process_message",
//	        attribute.String("task_id", msg.TaskID),
//	        attribute.String("topic", msg.Topic),
//	    )
//	    defer cleanup()
//
//	    su_logger.Info(ctx, "processing message")
//	    return w.process(ctx, msg)
//	}
//
//	// 定时任务，记录 job_name
//	func (c *Cron) SyncData() {
//	    ctx, cleanup := su_ytrace.NewRequestContext(
//	        context.Background(),
//	        "cron.sync_data",
//	        attribute.String("job_name", "sync_user_data"),
//	        attribute.Int("batch_size", 100),
//	    )
//	    defer cleanup()
//
//	    su_logger.Info(ctx, "start syncing")
//	}
//
// 参数说明：
//   - ctx: 父 context，可能已包含 request_id（通过 enum.CtxRequestId）
//   - operationName: 操作名称，作为 span 名称，建议格式: "worker.operation" 或 "cron.task_name"
//   - attrs: 可选的自定义属性，用于记录业务相关信息（如 task_id、user_id 等）
//
// 返回值：
//   - context.Context: 包含 request_id 和 root span 的 context
//   - func(): cleanup 函数，用于结束 span，必须使用 defer 调用
//
// 工作原理：
//   - 优先从 ctx 中获取已有的 request_id，若无则生成新的（32 hex chars = 128 bits）
//   - 将 request_id 作为 OpenTelemetry TraceID（日志和 trace 使用相同 ID）
//   - 创建 root span（后续的 Redis、DB、HTTP 等操作自动成为子 span）
//   - 返回的 context 通过标准 context 机制传播（不需要 Fiber 的 UserValue）
func NewRequestContext(ctx context.Context, operationName string, attrs ...attribute.KeyValue) (context.Context, func()) {
	// 1. 使用 EnsureTrace 确保 ctx 中有 request_id（优先复用已有的，没有则生成新的）
	ctx = trace.EnsureTrace(ctx)

	// 2. 获取 request_id（此时一定存在）
	requestID, _ := trace.ParseFromContext(ctx)

	// 3. 如果追踪已启用，创建 root span 并使用 request_id 作为 trace_id
	if IsEnabled() {
		tracer := otel.Tracer(getServiceName())

		// 将 request_id 转换为 TraceID
		traceID, valid := RequestIDToTraceID(requestID)
		if valid {
			spanCtx := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
				TraceID: traceID,
				SpanID:  oteltrace.SpanID{}, // 让 Start 自动生成
				//TraceFlags: oteltrace.FlagsSampled,
			})
			ctx = oteltrace.ContextWithSpanContext(ctx, spanCtx)
		}

		ctx, span := tracer.Start(ctx, operationName,
			oteltrace.WithSpanKind(oteltrace.SpanKindInternal),
			oteltrace.WithNewRoot(),
		)

		// 设置默认的 request_id attribute 和用户自定义的 attributes
		allAttrs := make([]attribute.KeyValue, 0, len(attrs)+1)
		allAttrs = append(allAttrs, attribute.String("request_id", requestID))
		allAttrs = append(allAttrs, attrs...)
		span.SetAttributes(allAttrs...)

		// 创建 cleanup 对象，设置 finalizer 作为安全网
		cleanup := &spanCleanup{
			span:      span,
			name:      operationName,
			requestID: requestID,
			// cleaned 默认为 false（atomic.Bool 零值）
		}

		// 设置 finalizer：如果 cleanup 函数未被调用，GC 时会触发
		runtime.SetFinalizer(cleanup, func(c *spanCleanup) {
			// 使用 CompareAndSwap 确保只执行一次，避免与正常 cleanup 竞争
			if c.cleaned.CompareAndSwap(false, true) && c.span.IsRecording() {
				// 记录警告：span 未正常结束（可能是忘记 defer cleanup()）
				c.span.SetAttributes(
					attribute.Bool("span.leaked", true),
					attribute.String("span.leak.hint", "cleanup function not called"),
				)
				c.span.SetStatus(codes.Error, "span leaked (cleanup not called)")
				SafeEndSpan(c.span) // 使用安全的 End 方法

				// 记录警告日志（使用标准库 log，避免循环依赖）
				log.Printf("[WARN] ytrace: span leaked and cleaned by finalizer | operation=%s request_id=%s hint=forgot to call defer cleanup()?",
					c.name, c.requestID)
			}
		})

		return ctx, func() {
			// 正常调用时，使用 CompareAndSwap 确保只执行一次
			if cleanup.cleaned.CompareAndSwap(false, true) {
				runtime.SetFinalizer(cleanup, nil) // 清除 finalizer
				SafeEndSpan(span)                  // 使用安全的 End 方法
			}
		}
	}

	// 追踪未启用，返回空的 cleanup 函数
	return ctx, func() {}
}

// NewChildSpanContext 在已有 request_id 的 context 上创建子 span
// 用于在任务处理过程中，为某个子操作创建独立的 span（但共享同一个 request_id）
//
// 使用场景：
//   - 在消息处理过程中，需要为某个耗时子操作创建独立 span
//   - 批量处理时，为每个 item 创建子 span
//
// 使用示例：
//
//	func (w *Worker) ProcessMessage(msg *kafka.Message) error {
//	    // 1. 为整个消息处理创建 root span
//	    ctx, cleanup := su_ytrace.NewRequestContext(
//	        context.Background(),
//	        "worker.process_message",
//	        attribute.String("message_id", msg.ID),
//	    )
//	    defer cleanup()
//
//	    // 2. 为支付处理创建子 span
//	    paymentCtx, paymentCleanup := su_ytrace.NewChildSpanContext(
//	        ctx,
//	        "worker.process_payment",
//	        attribute.String("payment_id", payment.ID),
//	        attribute.Int64("amount", payment.Amount),
//	    )
//	    defer paymentCleanup()
//	    w.processPayment(paymentCtx, payment)
//
//	    // 3. 为通知发送创建子 span
//	    notifyCtx, notifyCleanup := su_ytrace.NewChildSpanContext(
//	        ctx,
//	        "worker.send_notification",
//	        attribute.String("user_id", user.ID),
//	    )
//	    defer notifyCleanup()
//	    w.sendNotification(notifyCtx, user)
//
//	    return nil
//	}
//
// 参数说明：
//   - parentCtx: 父 context，必须包含 request_id（通常来自 NewRequestContext）
//   - operationName: 子操作名称，建议格式: "parent_operation.sub_operation"
//   - attrs: 可选的自定义属性，用于记录子操作相关信息
//
// 返回值：
//   - context.Context: 包含子 span 的 context（复用父 request_id）
//   - func(): cleanup 函数，用于结束子 span，必须使用 defer 调用
func NewChildSpanContext(parentCtx context.Context, operationName string, attrs ...attribute.KeyValue) (context.Context, func()) {
	if parentCtx == nil {
		// 没有父 context，回退到创建新的 root span
		return NewRequestContext(context.Background(), operationName, attrs...)
	}

	// 如果追踪已启用，创建子 span
	if IsEnabled() {
		tracer := otel.Tracer(getServiceName())

		ctx, span := tracer.Start(parentCtx, operationName,
			oteltrace.WithSpanKind(oteltrace.SpanKindInternal),
		)

		// 设置用户自定义的 attributes
		if len(attrs) > 0 {
			span.SetAttributes(attrs...)
		}

		// 创建 cleanup 对象，设置 finalizer 作为安全网
		cleanup := &spanCleanup{
			span:      span,
			name:      operationName,
			requestID: "", // 子 span 不需要记录 request_id（父 span 已有）
			// cleaned 默认为 false（atomic.Bool 零值）
		}

		// 设置 finalizer：如果 cleanup 函数未被调用，GC 时会触发
		runtime.SetFinalizer(cleanup, func(c *spanCleanup) {
			// 使用 CompareAndSwap 确保只执行一次，避免与正常 cleanup 竞争
			if c.cleaned.CompareAndSwap(false, true) && c.span.IsRecording() {
				// 记录警告：span 未正常结束
				c.span.SetAttributes(
					attribute.Bool("span.leaked", true),
					attribute.String("span.leak.hint", "cleanup function not called"),
				)
				c.span.SetStatus(codes.Error, "span leaked (cleanup not called)")
				SafeEndSpan(c.span) // 使用安全的 End 方法

				// 记录警告日志（使用标准库 log，避免循环依赖）
				log.Printf("[WARN] ytrace: child span leaked and cleaned by finalizer | operation=%s hint=forgot to call defer cleanup()?",
					c.name)
			}
		})

		return ctx, func() {
			// 正常调用时，使用 CompareAndSwap 确保只执行一次
			if cleanup.cleaned.CompareAndSwap(false, true) {
				runtime.SetFinalizer(cleanup, nil) // 清除 finalizer
				SafeEndSpan(span)                  // 使用安全的 End 方法
			}
		}
	}

	// 追踪未启用，返回原始 context 和空的 cleanup 函数
	return parentCtx, func() {}
}

// RequestIDToTraceID 将 request_id 转换为 OpenTelemetry TraceID
// 支持两种格式：
//  1. 自定义格式: timestamp(16 hex) + counter(8 hex) + random(8 hex) = 32 hex (16 bytes)
//  2. UUID 格式: b01795cf-54e7-41b1-9387-5c46d29581de (36 字符，去掉 `-` 后为 32 hex)
//
// 两种格式都正好是 OpenTelemetry TraceID 的标准格式 (128 bits = 16 bytes)
//
// 返回值：
//   - traceID: 转换后的 TraceID
//   - valid: 是否转换成功
func RequestIDToTraceID(requestID string) (oteltrace.TraceID, bool) {
	if requestID == "" {
		return oteltrace.TraceID{}, false
	}

	// 处理 UUID 格式：去掉 `-` 分隔符
	// UUID 格式：b01795cf-54e7-41b1-9387-5c46d29581de (36 字符)
	// 去掉 `-` 后：b01795cf54e741b193875c46d29581de (32 字符)
	cleanRequestID := requestID
	if len(requestID) == 36 && requestID[8] == '-' && requestID[13] == '-' &&
		requestID[18] == '-' && requestID[23] == '-' {
		// 这是标准 UUID 格式，去掉 `-`
		cleanRequestID = requestID[0:8] + requestID[9:13] + requestID[14:18] +
			requestID[19:23] + requestID[24:36]
	}

	// request_id 必须是 32 个十六进制字符（去掉 `-` 后）
	if len(cleanRequestID) != 32 {
		return oteltrace.TraceID{}, false
	}

	// 直接解析十六进制字符串为字节数组
	bytes, err := hex.DecodeString(cleanRequestID)
	if err != nil {
		// 无效格式，静默失败（避免日志量）
		return oteltrace.TraceID{}, false
	}

	// 验证长度
	if len(bytes) != 16 {
		// 长度不匹配，静默失败
		return oteltrace.TraceID{}, false
	}

	// 构造 TraceID
	var traceID oteltrace.TraceID
	copy(traceID[:], bytes)

	return traceID, true
}

// getServiceName 获取当前服务名称
func getServiceName() string {
	if v := globalServiceName.Load(); v != nil {
		if name, ok := v.(string); ok && name != "" {
			return name
		}
	}
	return "unknown-service"
}

// ExtractSpanFromContext 从 context 中提取 span，支持双重提取机制
// 1. 首先尝试标准方式获取（Worker 服务、标准 context.Context）
// 2. 如果失败，尝试从 UserValue 提取（Fiber 服务、fasthttp.RequestCtx）
// 3. 判断是否需要设置 request_id，如果需要则在 context 中添加 pending 标记
//
// 使用场景：
//   - 所有 instrumentation 代码（Redis、DB、Kafka、PubSub 等）
//   - 需要从 context 提取 parent span 的场景
//
// 返回值：
//   - 如果找到有效 span，返回包含 span 的 context
//   - 如果未找到，返回原始 context（不修改）
//   - 如果需要设置 request_id，返回的 context 中包含 pending 标记
func ExtractSpanFromContext(ctx context.Context) context.Context {
	var extractedCtx context.Context

	// 1. 首先尝试标准方式获取 span（Worker 服务走这里）
	if span := oteltrace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		// 已有有效 span，直接使用
		extractedCtx = ctx
	} else if spanValue := ctx.Value("otel-span"); spanValue != nil {
		// 2. 尝试从 UserValue 提取（Fiber 服务走这里）
		if parentSpan, ok := spanValue.(oteltrace.Span); ok {
			extractedCtx = oteltrace.ContextWithSpan(ctx, parentSpan)
		} else {
			// UserValue 存在但不是 span
			extractedCtx = ctx
		}
	} else {
		// 3. 没有找到任何 span
		extractedCtx = ctx
	}

	// 4. 检查是否需要设置 request_id
	// 判断依据：没有有效的本地父 span，或者只有远程父 span
	parentSpan := oteltrace.SpanFromContext(extractedCtx)
	shouldSetRequestID := !parentSpan.SpanContext().IsValid() || parentSpan.SpanContext().IsRemote()

	if shouldSetRequestID {
		// 从 context 读取 request_id
		if reqIDValue := ctx.Value(enum.CtxRequestId); reqIDValue != nil {
			if requestID, ok := reqIDValue.(string); ok && requestID != "" {
				// 创建 pending 标记，并添加到 context
				pending := &requestIDPending{
					requestID: requestID,
					used:      false,
				}
				extractedCtx = context.WithValue(extractedCtx, requestIDPendingKey, pending)
			}
		}
	}

	return extractedCtx
}

// MaybeSetRequestID 尝试设置 request_id 到 span attributes
//
// 工作原理：
//   - 检查 context 中是否有 ExtractSpanFromContext 添加的 pending 标记
//   - 如果有，调用 pending.trySet 尝试设置（只有第一次会成功）
//   - 使用互斥锁确保在多层嵌套 span 中 request_id 只被设置一次
//
// 参数说明：
//   - ctx: tracer.Start 返回的 context（包含新创建的 span 和可能的 pending 标记）
//   - span: 新创建的 span
//
// 使用示例：
//
//	// 标准模式
//	ctx = su_ytrace.ExtractSpanFromContext(ctx)
//	ctx, span := tracer.Start(ctx, "redis.get", ...)
//	defer span.End()
//	su_ytrace.MaybeSetRequestID(ctx, span)
//
//	// Consumer 模式（有额外的 Extract）
//	ctx = su_ytrace.ExtractSpanFromContext(ctx)
//	ctx = ExtractKafkaTrace(ctx, msg)
//	ctx, span := tracer.Start(ctx, "kafka.consume", ...)
//	defer span.End()
//	su_ytrace.MaybeSetRequestID(ctx, span)
func MaybeSetRequestID(ctx context.Context, span oteltrace.Span) {
	if ctx == nil || span == nil {
		return
	}

	// 检查 context 中是否有 pending 标记
	pendingValue := ctx.Value(requestIDPendingKey)
	if pendingValue == nil {
		return
	}

	// 类型断言
	pending, ok := pendingValue.(*requestIDPending)
	if !ok {
		return
	}

	// 尝试设置 request_id（只有第一次调用会成功）
	pending.trySet(span)
}
