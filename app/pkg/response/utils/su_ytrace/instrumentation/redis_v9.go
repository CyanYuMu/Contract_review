package instrumentation

import (
	"context"
	"fmt"
	"net"
	"strings"
	"unicode/utf8"

	redis "github.com/redis/go-redis/v9"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_ytrace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	redisV9TracerName = "seago/utils/su_ytrace/redis_v9"
	// db.statement 最大长度，避免过大的 payload 影响性能
	maxStatementLength = 1024
)

// sanitizeUTF8 清理字符串中的非 UTF-8 字符，并限制长度
// 用于确保 span attribute 中的字符串是有效的 UTF-8
func sanitizeUTF8(s string, maxLen int) string {
	// 先限制长度
	if maxLen > 0 && len(s) > maxLen {
		s = s[:maxLen] + "...[truncated]"
	}

	// 检查是否是有效 UTF-8
	if utf8.ValidString(s) {
		return s
	}

	// 替换无效的 UTF-8 字符
	return strings.ToValidUTF8(s, "\uFFFD")
}

// RedisV9Hook Redis v9 追踪 Hook
type RedisV9Hook struct {
	addr string
}

// NewRedisV9Hook 创建 Redis v9 Hook
// addr: Redis 连接地址，如 "redis://localhost:6379"
func NewRedisV9Hook(addr string) *RedisV9Hook {
	return &RedisV9Hook{
		addr: addr,
	}
}

// getTracer 获取 tracer（每次操作时获取，确保追踪系统已初始化）
func (h *RedisV9Hook) getTracer() trace.Tracer {
	return otel.Tracer(redisV9TracerName)
}

// DialHook 连接钩子
func (h *RedisV9Hook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		// 检查追踪是否启用
		if !su_ytrace.IsEnabled() {
			// 追踪未启用，直接执行连接
			return next(ctx, network, addr)
		}

		tracer := h.getTracer()
		ctx, span := tracer.Start(ctx, "redis.dial",
			trace.WithSpanKind(trace.SpanKindClient),
		)
		defer su_ytrace.SafeEndSpan(span) // 使用安全的 End 方法
		su_ytrace.MaybeSetRequestID(ctx, span)

		span.SetAttributes(
			attribute.String("db.system", "redis"),
			attribute.String("net.peer.name", addr),
			attribute.String("net.transport", network),
		)

		conn, err := next(ctx, network, addr)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}

		return conn, err
	}
}

// ProcessHook 单个命令处理钩子
func (h *RedisV9Hook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		// 检查追踪是否启用
		if !su_ytrace.IsEnabled() {
			// 追踪未启用，直接执行命令
			return next(ctx, cmd)
		}

		tracer := h.getTracer()

		// 提取 span（支持 Worker 和 Fiber 服务）
		ctx = su_ytrace.ExtractSpanFromContext(ctx)

		cmdName := cmd.Name()
		ctx, span := tracer.Start(ctx, fmt.Sprintf("redis.%s", cmdName),
			trace.WithSpanKind(trace.SpanKindClient),
		)
		defer su_ytrace.SafeEndSpan(span) // 使用安全的 End 方法
		su_ytrace.MaybeSetRequestID(ctx, span)

		span.SetAttributes(
			attribute.String("db.system", "redis"),
			attribute.String("db.connection_string", h.addr),
			attribute.String("redis.command", cmdName),
			// 记录完整命令（包含参数，不包含执行结果）
			// 使用 sanitizeUTF8 清理非 UTF-8 字符并限制长度，避免导出失败
			attribute.String("db.statement", sanitizeUTF8(cmd.String(), maxStatementLength)),
		)

		err := next(ctx, cmd)

		if err != nil && err != redis.Nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}

		return err
	}
}

// ProcessPipelineHook Pipeline 处理钩子（覆盖 Pipeline 追踪）
func (h *RedisV9Hook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		// 检查追踪是否启用
		if !su_ytrace.IsEnabled() {
			// 追踪未启用，直接执行命令
			return next(ctx, cmds)
		}

		tracer := h.getTracer()

		// 提取 span（支持 Worker 和 Fiber 服务）
		ctx = su_ytrace.ExtractSpanFromContext(ctx)

		ctx, span := tracer.Start(ctx, "redis.pipeline",
			trace.WithSpanKind(trace.SpanKindClient),
		)
		defer su_ytrace.SafeEndSpan(span) // 使用安全的 End 方法
		su_ytrace.MaybeSetRequestID(ctx, span)

		span.SetAttributes(
			attribute.String("db.system", "redis"),
			attribute.String("db.connection_string", h.addr),
			attribute.Int("redis.pipeline.length", len(cmds)),
		)

		// 记录所有命令名称和完整语句（包含参数）
		cmdNames := make([]string, len(cmds))
		cmdStatements := make([]string, 0, len(cmds))
		for i, cmd := range cmds {
			cmdNames[i] = cmd.Name()
			// 记录完整命令（包含参数），但限制数量避免过大
			if i < 10 { // 只记录前10条命令的详细信息
				// 使用 sanitizeUTF8 清理非 UTF-8 字符
				cmdStatements = append(cmdStatements, sanitizeUTF8(cmd.String(), maxStatementLength/10))
			}
		}
		span.SetAttributes(
			attribute.String("redis.pipeline.commands", strings.Join(cmdNames, ",")),
		)
		if len(cmdStatements) > 0 {
			span.SetAttributes(
				// 使用 sanitizeUTF8 确保整个字符串也是有效 UTF-8
				attribute.String("redis.pipeline.statements", sanitizeUTF8(strings.Join(cmdStatements, "; "), maxStatementLength)),
			)
		}

		err := next(ctx, cmds)

		// 检查是否有错误，只记录错误数量，不记录命令结果
		// 原因：Pipeline 结果可能包含大量数据，会导致日志系统崩溃
		var errCount int
		for _, cmd := range cmds {
			if cmdErr := cmd.Err(); cmdErr != nil && cmdErr != redis.Nil {
				errCount++
			}
		}

		if errCount > 0 {
			span.SetAttributes(attribute.Int("redis.pipeline.errors", errCount))
			span.SetStatus(codes.Error, fmt.Sprintf("%d commands failed", errCount))
		} else if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}

		return err
	}
}
