package instrumentation

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	redis "github.com/go-redis/redis/v8"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_ytrace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	redisV8TracerName = "seago/utils/su_ytrace/redis_v8"
	// db.statement 最大长度，避免过大的 payload 影响性能
	maxStatementLengthV8 = 1024
)

// sanitizeUTF8V8 清理字符串中的非 UTF-8 字符，并限制长度
func sanitizeUTF8V8(s string, maxLen int) string {
	if maxLen > 0 && len(s) > maxLen {
		s = s[:maxLen] + "...[truncated]"
	}
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToValidUTF8(s, "\uFFFD")
}

// RedisV8Hook Redis v8 追踪 Hook
type RedisV8Hook struct {
	addr string
}

// NewRedisV8Hook 创建 Redis v8 Hook
// addr: Redis 连接地址，如 "redis://localhost:6379"
func NewRedisV8Hook(addr string) *RedisV8Hook {
	return &RedisV8Hook{
		addr: addr,
	}
}

// getTracer 获取 tracer（每次操作时获取，确保追踪系统已初始化）
func (h *RedisV8Hook) getTracer() trace.Tracer {
	return otel.Tracer(redisV8TracerName)
}

// BeforeProcess 单个命令执行前
func (h *RedisV8Hook) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		// 追踪未启用，直接返回
		return ctx, nil
	}

	tracer := h.getTracer()

	// 提取 span（支持 Worker 和 Fiber 服务）
	ctx = su_ytrace.ExtractSpanFromContext(ctx)

	cmdName := cmd.Name()
	ctx, span := tracer.Start(ctx, fmt.Sprintf("redis.%s", cmdName),
		trace.WithSpanKind(trace.SpanKindClient),
	)
	su_ytrace.MaybeSetRequestID(ctx, span)

	span.SetAttributes(
		attribute.String("db.system", "redis"),
		attribute.String("db.connection_string", h.addr),
		attribute.String("redis.command", cmdName),
		// 记录完整命令（包含参数，不包含执行结果）
		// 使用 sanitizeUTF8V8 清理非 UTF-8 字符并限制长度，避免导出失败
		attribute.String("db.statement", sanitizeUTF8V8(cmd.String(), maxStatementLengthV8)),
		attribute.String("redis.phase", "before_execution"),
	)

	return ctx, nil
}

// AfterProcess 单个命令执行后
func (h *RedisV8Hook) AfterProcess(ctx context.Context, cmd redis.Cmder) error {
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		// 追踪未启用，直接返回
		return nil
	}

	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return nil
	}

	// 标记执行阶段（默认为执行结果）
	span.SetAttributes(attribute.String("redis.phase", "after_execution"))

	// 只记录错误状态，不记录命令结果
	// 原因：Redis 结果可能非常大（如大的 list、hash），会导致日志系统崩溃
	if err := cmd.Err(); err != nil && err != redis.Nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}

	su_ytrace.SafeEndSpan(span) // 使用安全的 End 方法
	return nil
}

// BeforeProcessPipeline Pipeline 执行前（覆盖 Pipeline 追踪）
func (h *RedisV8Hook) BeforeProcessPipeline(ctx context.Context, cmds []redis.Cmder) (context.Context, error) {
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		// 追踪未启用，直接返回
		return ctx, nil
	}

	tracer := h.getTracer()

	// 提取 span（支持 Worker 和 Fiber 服务）
	ctx = su_ytrace.ExtractSpanFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.pipeline",
		trace.WithSpanKind(trace.SpanKindClient),
	)
	su_ytrace.MaybeSetRequestID(ctx, span)

	span.SetAttributes(
		attribute.String("db.system", "redis"),
		attribute.String("db.connection_string", h.addr),
		attribute.Int("redis.pipeline.length", len(cmds)),
		attribute.String("redis.phase", "before_execution"),
	)

	// 记录所有命令名称和完整语句（包含参数）
	cmdNames := make([]string, len(cmds))
	cmdStatements := make([]string, 0, len(cmds))
	for i, cmd := range cmds {
		cmdNames[i] = cmd.Name()
		// 记录完整命令（包含参数），但限制数量避免过大
		if i < 10 { // 只记录前10条命令的详细信息
			// 使用 sanitizeUTF8V8 清理非 UTF-8 字符
			cmdStatements = append(cmdStatements, sanitizeUTF8V8(cmd.String(), maxStatementLengthV8/10))
		}
	}
	span.SetAttributes(
		attribute.String("redis.pipeline.commands", strings.Join(cmdNames, ",")),
	)
	if len(cmdStatements) > 0 {
		span.SetAttributes(
			// 使用 sanitizeUTF8V8 确保整个字符串也是有效 UTF-8
			attribute.String("redis.pipeline.statements", sanitizeUTF8V8(strings.Join(cmdStatements, "; "), maxStatementLengthV8)),
		)
	}

	return ctx, nil
}

// AfterProcessPipeline Pipeline 执行后
func (h *RedisV8Hook) AfterProcessPipeline(ctx context.Context, cmds []redis.Cmder) error {
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		// 追踪未启用，直接返回
		return nil
	}

	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return nil
	}

	// 标记执行阶段（默认为执行结果）
	span.SetAttributes(attribute.String("redis.phase", "after_execution"))

	// 检查是否有错误，只记录错误数量，不记录命令结果
	// 原因：Pipeline 结果可能包含大量数据，会导致日志系统崩溃
	var errCount int
	for _, cmd := range cmds {
		if err := cmd.Err(); err != nil && err != redis.Nil {
			errCount++
		}
	}

	if errCount > 0 {
		span.SetAttributes(attribute.Int("redis.pipeline.errors", errCount))
		span.SetStatus(codes.Error, fmt.Sprintf("%d commands failed", errCount))
	} else {
		span.SetStatus(codes.Ok, "")
	}

	su_ytrace.SafeEndSpan(span) // 使用安全的 End 方法
	return nil
}
