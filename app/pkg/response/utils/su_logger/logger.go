package su_logger

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/spf13/cast"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/enum"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/qwrobot"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// 对象池
var extraPool = sync.Pool{
	New: func() interface{} {
		return &Extra{
			fields: make([]zap.Field, 0, 8), // 预分配容量
		}
	},
}

// 先暖个场, 缓解高并发竞态问题
const extraPoolWarmupSize = 2048

func init() {
	// 预先初始化一定数量的对象放入池中，避免高并发时大量创建对象
	warmupPool()
}

func warmupPool() {
	extras := make([]*Extra, extraPoolWarmupSize)
	for i := 0; i < extraPoolWarmupSize; i++ {
		extras[i] = &Extra{
			fields: make([]zap.Field, 0, 8),
		}
	}
	// 全部放入池中
	for i := 0; i < extraPoolWarmupSize; i++ {
		extraPool.Put(extras[i])
	}
}

// E 函数现在从对象池中获取 Extra 实例
func E() *Extra {
	e := extraPool.Get().(*Extra)
	e.fields = e.fields[:0] // 清空切片但保留容量
	e.cond = nil
	return e
}

type Extra struct {
	cond   Condition
	fields []zap.Field
}

func (e *Extra) Release() {
	extraPool.Put(e)
}

func (e *Extra) Cond(c Condition) *Extra {
	e.cond = c

	return e
}

func (e *Extra) String(key string, value string) *Extra {
	e.fields = append(e.fields, zap.String(key, value))
	return e
}

func (e *Extra) Int(key string, v int) *Extra {
	e.fields = append(e.fields, zap.Int(key, v))

	return e
}

func (e *Extra) Int8(key string, v int8) *Extra {
	e.fields = append(e.fields, zap.Int8(key, v))

	return e
}

func (e *Extra) Int16(key string, v int16) *Extra {
	e.fields = append(e.fields, zap.Int16(key, v))

	return e
}

func (e *Extra) Int32(key string, v int32) *Extra {
	e.fields = append(e.fields, zap.Int32(key, v))

	return e
}

func (e *Extra) Int64(key string, v int64) *Extra {
	e.fields = append(e.fields, zap.Int64(key, v))

	return e
}

func (e *Extra) Uint(key string, v uint) *Extra {
	e.fields = append(e.fields, zap.Uint(key, v))

	return e
}

func (e *Extra) Uint8(key string, v uint8) *Extra {
	e.fields = append(e.fields, zap.Uint8(key, v))

	return e
}

func (e *Extra) Uint16(key string, v uint16) *Extra {
	e.fields = append(e.fields, zap.Uint16(key, v))

	return e
}

func (e *Extra) Uint32(key string, v uint32) *Extra {
	e.fields = append(e.fields, zap.Uint32(key, v))

	return e
}

func (e *Extra) Uint64(key string, v uint64) *Extra {
	e.fields = append(e.fields, zap.Uint64(key, v))

	return e
}

func (e *Extra) Float64(key string, v float64) *Extra {
	e.fields = append(e.fields, zap.Float64(key, v))

	return e
}

func (e *Extra) Bool(key string, v bool) *Extra {
	e.fields = append(e.fields, zap.Bool(key, v))

	return e
}

func (e *Extra) Any(key string, value interface{}) *Extra {
	e.Interface(key, value)

	return e
}

func (e *Extra) Error(err error) *Extra {
	e.fields = append(e.fields, zap.Error(err))

	return e
}

func (e *Extra) NamedError(key string, err error) *Extra {
	e.fields = append(e.fields, zap.NamedError(key, err))

	return e
}

func (e *Extra) Interface(key string, v interface{}) *Extra {
	if v == nil {
		e.fields = append(e.fields, zap.String(key, ""))
	} else {
		switch reflect.TypeOf(v).Kind() {
		case reflect.Slice, reflect.Array, reflect.Struct, reflect.Map, reflect.Ptr:
			// 对结构体, map, slice 进行特判
			s, err1 := jsoniter.MarshalToString(v)
			if err1 != nil {
				e.fields = append(e.fields, zap.Any(key, v))
			} else {
				e.fields = append(e.fields, zap.String(key, s))
			}
		default:
			e.fields = append(e.fields, zap.Any(key, v))
		}
	}

	return e
}

func (e *Extra) Ctx(ctx context.Context) *Extra {
	if curC, ok := ctx.(C); ok {
		e.Trace(curC.traceId)
		e.Span(curC.spanId)
	} else {
		traceId, spanId := trace.ParseFromContext(ctx)
		e.Trace(traceId)
		e.Span(spanId)
	}

	return e
}

func (e *Extra) Trace(traceId string) *Extra {
	if traceId != "" {
		e.fields = append(e.fields, zap.String("trace", traceId))
	}

	return e
}

func (e *Extra) NowMs() *Extra {
	e.fields = append(e.fields, zap.Int64("ms", time.Now().UnixNano()/1e6))

	return e
}

func (e *Extra) Span(spanId string) *Extra {
	if spanId != "" {
		e.fields = append(e.fields, zap.String("span", spanId))
	}

	return e
}

var logLevel zapcore.Level = -2

func isEnable(ctx context.Context, level zapcore.Level) bool {
	if ctx != nil {
		if d := ctx.Value(enum.CtxDebug); d != nil {
			return true
		}
	}

	if logLevel == -2 {
		// 获取当前的stage, 并设置对应的日志级别
		stage := os.Getenv(enum.StageKey)
		if stage == string(enum.EnvStageRelease) {
			logLevel = zapcore.InfoLevel
		} else {
			logLevel = zapcore.DebugLevel
		}
	}

	return level >= logLevel
}

func assembleField(ctx context.Context, err error, extra []*Extra) []zap.Field {
	var e *Extra
	if len(extra) == 0 || extra[0] == nil {
		e = E()
	} else {
		e = extra[0]
	}

	if err != nil {
		e.Error(err)
	}

	e.Ctx(ctx)

	return e.fields
}

func WithCaller() *Extra {
	return nil
}

func shouldLog(extra []*Extra) bool {
	if len(extra) == 0 || (extra)[0] == nil {
		return true
	}
	if extra[0].cond != nil {
		should, f := (extra)[0].cond.ShouldLog()
		if should && f != nil {
			extra[0].fields = append(extra[0].fields, *f)
		}

		return should
	} else {
		return true
	}
}

// 设置日志级别
func EnableLevel(l zapcore.Level) {
	logLevel = l
}

func Info(ctx context.Context, msg string, extra ...*Extra) {
	if isEnable(ctx, zapcore.InfoLevel) && shouldLog(extra) {
		if len(extra) > 1 {
			loggerWithCaller.Info(msg, assembleField(ctx, nil, extra)...)
		} else {
			fields := assembleField(ctx, nil, extra)
			loggerWithoutCaller.Info(msg, fields...)
		}
	}
	if len(extra) > 0 {
		extra[0].Release()
	}
}

func InfoWithNotify(ctx context.Context, msg string, extra ...*Extra) {
	if shouldLog(extra) {
		fields := assembleField(ctx, nil, extra)
		loggerWithCaller.Info(msg, fields...)
		Notify(zapcore.InfoLevel, msg, fields)
	}
	if len(extra) > 0 {
		extra[0].Release()
	}
}

func Debug(ctx context.Context, msg string, extra ...*Extra) {
	if isEnable(ctx, zapcore.DebugLevel) && shouldLog(extra) {
		if len(extra) > 1 {
			loggerWithCaller.Debug(msg, assembleField(ctx, nil, extra)...)
		} else {
			loggerWithoutCaller.Debug(msg, assembleField(ctx, nil, extra)...)
		}
	}
	if len(extra) > 0 {
		extra[0].Release()
	}
}

func Warn(ctx context.Context, msg string, extra ...*Extra) {
	if isEnable(ctx, zapcore.WarnLevel) && shouldLog(extra) {
		loggerWithCaller.Warn(msg, assembleField(ctx, nil, extra)...)
	}
	if len(extra) > 0 {
		extra[0].Release()
	}
}

func WarnWithNotify(ctx context.Context, msg string, extra ...*Extra) {
	if shouldLog(extra) {
		fields := assembleField(ctx, nil, extra)
		loggerWithCaller.Warn(msg, fields...)
		Notify(zapcore.WarnLevel, msg, fields)
	}
	if len(extra) > 0 {
		extra[0].Release()
	}
}

func Notify(level zapcore.Level, msg string, fields []zap.Field) {
	robot := qwrobot.Get()
	if robot != nil {
		content := fieldsToStringV2(fields)
		qwMsg := qwrobot.Message{
			Title:   msg,
			Content: content,
		}
		if level == zapcore.InfoLevel {
			robot.Info(qwMsg)
		} else if level == zapcore.WarnLevel {
			robot.Warn(qwMsg)
		} else if level == zapcore.ErrorLevel {
			robot.Error(qwMsg)
		} else if level == zapcore.PanicLevel || level == zapcore.DPanicLevel || level == zapcore.FatalLevel {
			robot.Fatal(qwMsg)
		}
	}
}

func Error(ctx context.Context, err error, msg string, extra ...*Extra) {
	if isEnable(ctx, zapcore.ErrorLevel) && shouldLog(extra) {
		loggerWithCaller.Error(msg, assembleField(ctx, err, extra)...)
	}
	if len(extra) > 0 {
		extra[0].Release()
	}
}

/*Fatal
* @Description: 致命错误
* @param ctx
* @param msg
* @param extra
 */
//func Fatal(ctx context.Context, msg string, extra ...*Extra) {
//	loggerWithCaller.Fatal(msg, assembleField(ctx, nil, extra)...)
//}

/*FatalWithNotify
* @Description: 致命错误
* @param ctx
* @param err
* @param msg
* @param extra
 */
//func FatalWithNotify(ctx context.Context, err error, msg string, extra ...*Extra) {
//	fields := assembleField(ctx, err, extra)
//	loggerWithCaller.Fatal(msg, fields...)
//	Notify(zapcore.FatalLevel, msg, fields)
//}

func ErrorWithNotify(ctx context.Context, err error, msg string, extra ...*Extra) {
	if shouldLog(extra) {
		fields := assembleField(ctx, err, extra)
		loggerWithCaller.Error(msg, fields...)
		Notify(zapcore.FatalLevel, msg, fields)
	}
	if len(extra) > 0 {
		extra[0].Release()
	}
}

func Panic(ctx context.Context, msg string, extra ...*Extra) {
	//stack := FormatStackInline(debug.Stack(), 5)
	stack := debug.Stack()
	err := errors.New(string(stack))

	loggerWithCaller.DPanic(msg, assembleField(ctx, err, extra)...)
	if len(extra) > 0 {
		extra[0].Release()
	}
}

func PanicWithNotify(ctx context.Context, msg string, extra ...*Extra) {
	stack := FormatStackInline(debug.Stack(), 5)
	err := errors.New(stack)
	fields := assembleField(ctx, err, extra)
	loggerWithCaller.DPanic(msg, fields...)

	Notify(zapcore.DPanicLevel, msg, fields)
	if len(extra) > 0 {
		extra[0].Release()
	}
}

//func Fatal(ctx context.Contextk)

func DoPanicWithNotifyWithoutCaller(ctx context.Context, msg string, extra ...*Extra) {
	fields := assembleField(ctx, nil, extra)
	loggerWithCaller.DPanic(msg, fields...)

	Notify(zapcore.DPanicLevel, msg, fields)
	if len(extra) > 0 {
		extra[0].Release()
	}
}

// FormatStackInline
// 调用栈信息, 进行压缩, 否则在日志平台显示出来内容过多
func FormatStackInline(stack []byte, jumpLevel int) string {
	if stack == nil {
		return ""
	}
	r := bytes.NewReader(stack)
	reader := bufio.NewReader(r)
	s := strings.Builder{}
	sep := []byte{'/'}
	var i int
	for {
		line, _, err := reader.ReadLine()
		if i >= jumpLevel {
			if err == nil && s.Len() > 0 {
				s.WriteString("->")
			}

			cnt := bytes.Count(line, sep)
			if cnt > 3 {
				byteSlice := bytes.Split(line, sep)
				sliceLen := len(byteSlice)
				fromPos := sliceLen - 3
				s.Write(bytes.Join(byteSlice[fromPos:], []byte{'/'}))
			} else {
				s.Write(line)
			}
		}
		i++

		if err == io.EOF {
			break
		}
	}

	return s.String()
}

func fieldsToStringV2(fields []zap.Field) string {
	s := strings.Builder{}
	// 事件点
	fields = append(fields, zap.Field{
		Key:    "ts",
		Type:   zapcore.StringType,
		String: time.Now().Format("2006-01-02 15:04:05.000"),
	})
	// 触发代码位置
	_, file, line, ok := runtime.Caller(3)
	if ok {
		caller := toShortCaller(file)
		fields = append(fields, zap.Field{
			Key:    "caller",
			Type:   zapcore.StringType,
			String: caller + ":" + cast.ToString(line),
		})
	}

	var errStr strings.Builder

	for i := range fields {
		if fields[i].Type == zapcore.ErrorType {
			errStr.WriteString(fields[i].Key)
			errStr.WriteString(": ")
			errStr.WriteString(fields[i].Interface.(error).Error())
			errStr.WriteString("\n")
		} else {
			if s.Len() > 0 {
				s.WriteByte('\n')
			}
			s.WriteString("> ")
			s.WriteString(fields[i].Key)
			s.WriteString(": ")
			switch fields[i].Type {
			case zapcore.StringType:
				s.WriteString(fields[i].String)
			case zapcore.Int8Type, zapcore.Int16Type, zapcore.Int32Type, zapcore.Int64Type, zapcore.Uint8Type, zapcore.Uint16Type, zapcore.Uint32Type, zapcore.Uint64Type, zapcore.BoolType, zapcore.Float32Type, zapcore.Float64Type:
				s.WriteString(cast.ToString(fields[i].Integer))
			default:
				s.WriteString(fmt.Sprintf("%v", fields[i].Interface))
			}
		}

	}

	errStr.WriteString(s.String())

	return errStr.String()
}

func toShortCaller(line string) string {
	list := strings.Split(line, "/")
	if len(list) > 1 {
		max := len(list)
		return strings.Join(list[max-2:max], "/")
	} else {
		return strings.Join(list, "/")
	}
}

type C struct {
	context.Context
	traceId string
	spanId  string
}

func (c C) TraceId() string {
	return c.traceId
}
func (c C) SpanId() string {
	return c.spanId
}

func (c C) Trace() (traceId, spanId string) {
	return c.traceId, c.spanId
}

func Ctx(ctx context.Context) context.Context {
	traceId, spanId := trace.ParseFromContext(ctx)

	return C{
		Context: nil,
		traceId: traceId,
		spanId:  spanId,
	}
}

// 尝试从当前ctx中获取链路, 获取失败则自动填充链路信息, 用于 脚本, 定时任务, 没有上下文的场景
func AutoCtx(ctx context.Context) context.Context {
	traceId, spanId := trace.ParseFromContext(ctx)
	if traceId == "" {
		traceId = trace.NewRequestId()
	}
	if spanId == "" {
		spanId = trace.NewSpanId()
	}

	return C{
		Context: nil,
		traceId: traceId,
		spanId:  spanId,
	}
}
