package su_logger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	jsoniter "github.com/json-iterator/go"
	"go.uber.org/zap"
	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
)

func kvEncoder() zapcore.Encoder {
	return NewKeyValueEncoder(&Config{
		Level:      zapcore.DebugLevel,
		Colorful:   true,
		Writer:     nil,
		CallerSkip: 6,
	})
}

var loggerWithCaller = NewZapLogger(&Config{
	Level:      zapcore.DebugLevel,
	Colorful:   true,
	Writer:     nil,
	CallerSkip: 1,
	Encoder:    kvEncoder(),
})
var loggerWithoutCaller = NewZapLogger(&Config{
	Level:      zapcore.DebugLevel,
	Colorful:   true,
	Writer:     nil,
	CallerSkip: 0,
	Encoder:    kvEncoder(),
})

// 重设日志实例
func Init(cnf *Config) {
	loggerWithCaller = NewZapLogger(cnf)
	// cnf.CallerSkip = 0
	loggerWithoutCaller = NewZapLogger(cnf)
}

const logTimeDefaultFormat = "2006-01-02 15:04:05.000"

// 日志格式
func (z *zapLogger) getEncoder(cnf *Config) zapcore.Encoder {
	if cnf.Encoder != nil {
		return cnf.Encoder
	}
	var encodeLevel zapcore.LevelEncoder
	if z.Colorful {
		encodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		encodeLevel = zapcore.CapitalLevelEncoder
	}
	encodingConfig := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey, //zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    encodeLevel,
		EncodeTime:     zapcore.TimeEncoderOfLayout(logTimeDefaultFormat),
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder, //zapcore.FullCallerEncoder zapcore.ShortCallerEncoder
		NewReflectedEncoder: func(w io.Writer) zapcore.ReflectedEncoder {
			encoder := jsoniter.NewEncoder(w)
			encoder.SetEscapeHTML(false)

			return encoder
		},
	}

	return zapcore.NewConsoleEncoder(encodingConfig)
}

// 日志写到哪里
func (z *zapLogger) getWriteSyncer() (zapcore.WriteSyncer, error) {
	if z.Output == "" || z.Output == "std" {
		return zapcore.AddSync(os.Stdout), nil
	} else {
		file, err := os.OpenFile(z.Output, os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			return nil, err
		}
		return zapcore.AddSync(file), nil
	}
}

type zapLogger struct {
	// zapInst  *zap.Logger
	Level    string `json:"level"`
	Colorful bool   `json:"color"`
	//Writer io.Writer
	// std(�������认) or 具体的文件路径
	Output string `json:"output"`
	// 调����栈往上走的层数
	CallerSkip int `json:"caller_skip"`
}

type Config struct {
	Level    zapcore.Level
	Colorful bool
	//Writer io.Writer
	Writer zapcore.WriteSyncer
	// 调用栈信息
	CallerSkip int
	// default json encoder
	Encoder zapcore.Encoder
	// 指定什么>=级别打印堆栈信息级别
	// StackLevel zapcore.Level
	// 指定什么>=级别打印调用栈信息
	PrintCallerLevel zapcore.Level
}

type FileWriter struct {
	writer io.Writer
}

func (f *FileWriter) Sync() error {
	return f.writer.(*os.File).Sync()
}

func NewFileWriter(ctx context.Context, fp *os.File) *FileWriter {
	w := &FileWriter{
		writer: fp,
	}
	go func() {
		for range ctx.Done() {
			_ = fp.Close()
			return
		}
	}()

	return w
}

type FieldPool struct {
	pool sync.Pool
}

func NewFieldPool() *FieldPool {
	return &FieldPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &zap.Field{}
			},
		},
	}
}

var fieldPool = NewFieldPool()

// GetString 从池中获取一个字符串类型的 zap.Field 对象
func (f *FieldPool) GetString(key string, value string) *zap.Field {
	field := f.get()
	field.Type = zapcore.StringType
	field.Key = key
	field.String = value
	return field
}

// GetInt 从池中获取一个整数类型的 zap.Field 对象
func (f *FieldPool) GetInt(key string, value int) *zap.Field {
	field := f.get()
	field.Type = zapcore.Int64Type
	field.Key = key
	field.Integer = int64(value)
	return field
}

// GetInt8 从池中获取一个 int8 类型的 zap.Field 对象
func (f *FieldPool) GetInt8(key string, value int8) *zap.Field {
	field := f.get()
	field.Type = zapcore.Int64Type
	field.Key = key
	field.Integer = int64(value)
	return field
}

// GetInt16
func (f *FieldPool) GetInt16(key string, value int16) *zap.Field {
	field := f.get()
	field.Type = zapcore.Int64Type
	field.Key = key
	field.Integer = int64(value)
	return field
}

// GetInt32 从池中获取一个 int32 类型的 zap.Field 对象
func (f *FieldPool) GetInt32(key string, value int32) *zap.Field {
	field := f.get()
	field.Type = zapcore.Int64Type
	field.Key = key
	field.Integer = int64(value)
	return field
}

// GetInt64 从池中获取一个 int64 类型的 zap.Field 对象
func (f *FieldPool) GetInt64(key string, value int64) *zap.Field {
	field := f.get()
	field.Type = zapcore.Int64Type
	field.Key = key
	field.Integer = value
	return field
}

// GetFloat32 从池中获取一个 float32 类型的 zap.Field 对象
func (f *FieldPool) GetFloat32(key string, value float32) *zap.Field {
	field := f.get()
	field.Type = zapcore.Float32Type
	field.Key = key
	field.Interface = value

	return field
}

// GetUint8
func (f *FieldPool) GetUint8(key string, value uint8) *zap.Field {
	field := f.get()
	field.Type = zapcore.Uint64Type
	field.Key = key
	field.Integer = int64(value)

	return field
}

// GetUint16
func (f *FieldPool) GetUint16(key string, value uint16) *zap.Field {
	field := f.get()
	field.Type = zapcore.Uint64Type
	field.Key = key
	field.Integer = int64(value)

	return field
}

// GetUint32
func (f *FieldPool) GetUint32(key string, value uint32) *zap.Field {
	field := f.get()
	field.Type = zapcore.Uint64Type
	field.Key = key
	field.Integer = int64(value)

	return field
}

// GetUint64
func (f *FieldPool) GetUint64(key string, value uint64) *zap.Field {
	field := f.get()
	field.Type = zapcore.Uint64Type
	field.Key = key
	field.Integer = int64(value)

	return field
}

// GetUintptr
func (f *FieldPool) GetUintptr(key string, value uintptr) *zap.Field {
	field := f.get()
	field.Type = zapcore.UintptrType
	field.Key = key
	field.Interface = value

	return field
}

// GetFloat64 从池中获取一个 float64 类型的 zap.Field 对象
func (f *FieldPool) GetFloat64(key string, value float64) *zap.Field {
	field := f.get()
	field.Type = zapcore.Float64Type
	field.Key = key
	field.Interface = value

	return field
}

func (f *FieldPool) GetBool(key string, value bool) *zap.Field {
	field := f.get()
	field.Type = zapcore.BoolType
	field.Key = key
	field.Integer = 0
	field.Interface = value

	return field
}

// GetError
func (f *FieldPool) GetError(key string, value error) *zap.Field {
	field := f.get()
	field.Type = zapcore.ErrorType
	field.Key = key
	field.Interface = value

	return field

}

// GetInterface 从池中获取一个接口类型的 zap.Field 对象
func (f *FieldPool) GetInterface(key string, value interface{}) *zap.Field {
	field := f.get()
	field.Type = zapcore.ReflectType
	field.Key = key
	field.Interface = value

	return field
}

// Put 将字段对象放回池中
func (f *FieldPool) Put(field ...*zap.Field) {
	for i := range field {
		// 清空字段对象
		field[i].Key = ""
		field[i].String = ""
		field[i].Integer = 0
		field[i].Interface = nil

		f.pool.Put(field[i])
	}
}

// get 从池中获取字段对象
func (f *FieldPool) get() *zap.Field {
	field := f.pool.Get().(*zap.Field)
	return field
}

func NewZapLogger(cnf *Config) (l *zap.Logger) {
	z := &zapLogger{
		Colorful: cnf.Colorful,
	}
	encoder := z.getEncoder(cnf)
	var writer zapcore.WriteSyncer
	if cnf.Writer != nil {
		writer = cnf.Writer
	} else {
		writer, _ = z.getWriteSyncer()
	}

	core := zapcore.NewCore(encoder, writer, cnf.Level)
	if cnf.CallerSkip > 0 {
		//  仅当设置了caller才会获取调用栈信��
		l = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(cnf.CallerSkip))
	} else {
		l = zap.New(core)
	}

	return l
}

func (z *zapLogger) init(jsonConfig string) error {
	if jsonConfig == "" {
		jsonConfig = "{}"
	}
	if jsonConfig != "{}" {
		_, _ = fmt.Fprintf(os.Stdout, "zap logger init with config:%s\n", jsonConfig)
	}

	err := json.Unmarshal([]byte(jsonConfig), z)
	z.Colorful = true

	return err
}

// 添加 buffer pool
type BufferPool struct {
	pool buffer.Pool
}

func NewBufferPool() *BufferPool {
	return &BufferPool{
		pool: buffer.NewPool(),
	}
}

func (p *BufferPool) Get() *buffer.Buffer {
	return p.pool.Get()
}

func (p *BufferPool) Put(buf *buffer.Buffer) {
	if buf != nil {
		// buf.Reset()
		buf.Free()

		// p.pool.put(buf)
	}
}

var globalBufferPool = NewBufferPool()

// KeyValueEncoder 实现 zapcore.Encoder 接口
type KeyValueEncoder struct {
	zapcore.MapObjectEncoder
	// 添加配置字段，避免重复计算
	timeLayout     string
	kvSeparator    string
	fieldSeparator string
	pool           *BufferPool
	colorful       bool
	level          zapcore.Level
	callerSkip     int
	// 打印调用栈的级别(>=)
	printCallerLevel zapcore.Level
}

// 优化 NewKeyValueEncoder 构造函数
func NewKeyValueEncoder(cnf *Config) zapcore.Encoder {
	encoder := &KeyValueEncoder{
		timeLayout:       "2006-01-02 15:04:05.000",
		kvSeparator:      "=",
		fieldSeparator:   "&",
		pool:             globalBufferPool,
		colorful:         cnf.Colorful,
		level:            cnf.Level,
		callerSkip:       cnf.CallerSkip,
		printCallerLevel: zapcore.WarnLevel,
	}

	return encoder
}

// Clone 实现 zapcore.Encoder 接口
func (e *KeyValueEncoder) Clone() zapcore.Encoder {
	return &KeyValueEncoder{
		timeLayout:     e.timeLayout,
		kvSeparator:    e.kvSeparator,
		fieldSeparator: e.fieldSeparator,
		pool:           e.pool,
	}
}

// 优化 EncodeEntry 方法
func (e *KeyValueEncoder) EncodeEntry(entry zapcore.Entry, fields []zapcore.Field) (*buffer.Buffer, error) {
	// 从 pool 获取 buffer
	buf := e.pool.Get()

	// 分配合适的容量 (根据经验值设置)
	final := make([]byte, 0, 1024)

	// 添加时间
	final = append(final, entry.Time.Format(e.timeLayout)...)

	// 添加日志级别
	final = append(final, ' ')
	// final = append(final, entry.Level.CapitalString()...)
	if e.colorful {
		var levelColor Color
		switch entry.Level {
		case zapcore.DebugLevel:
			levelColor = Magenta
		case zapcore.InfoLevel:
			levelColor = Blue
		case zapcore.WarnLevel:
			levelColor = Yellow
		case zapcore.ErrorLevel, zapcore.DPanicLevel, zapcore.PanicLevel, zapcore.FatalLevel:
			levelColor = Red
		default:
			levelColor = White
		}
		coloredLevel := levelColor.Add(entry.Level.CapitalString())
		final = append(final, coloredLevel...)
	} else {
		final = append(final, entry.Level.CapitalString()...)
	}

	if entry.Level >= e.printCallerLevel {
		final = append(final, ' ')
		// 获取当前的调用栈
		pc := make([]uintptr, 1)
		n := runtime.Callers(e.callerSkip, pc)
		if n > 0 {
			frames := runtime.CallersFrames(pc[:n])
			frame, _ := frames.Next()
			// 截取文件路径，只保留最后三层
			filePathParts := strings.Split(frame.File, "/")
			if len(filePathParts) > 3 {
				frame.File = strings.Join(filePathParts[len(filePathParts)-3:], "/")
			}
			caller := fmt.Sprintf("%s:%d", frame.File, frame.Line)
			final = append(final, caller...)
		}
	}

	// 添加消息
	if entry.Message != "" {
		final = append(final, ' ')
		final = append(final, entry.Message...)
	}

	// 添加字段
	if len(fields) > 0 {
		final = append(final, "\t"...)
		final = e.appendKVFields(final, fields)
	}

	// 添加换行符
	final = append(final, '\n')

	buf.Write(final)

	return buf, nil
}

// appendString 优化字符串追加
func appendString(buf []byte, s string) []byte {
	// 对特殊字符进行处理
	if needsQuoting(s) {
		return strconv.AppendQuote(buf, s)
	}
	return append(buf, s...)
}

// appendKVFields 优化字段追加，使用 & 分隔
func (e *KeyValueEncoder) appendKVFields(buf []byte, fields []zapcore.Field) []byte {
	for i := range fields {
		if i > 0 {
			buf = append(buf, e.fieldSeparator...)
		}
		buf = append(buf, fields[i].Key...)
		buf = append(buf, e.kvSeparator...)
		buf = e.appendFieldValue(buf, fields[i])
	}
	return buf
}

// appendFieldValue 优化字段值追加
func (e *KeyValueEncoder) appendFieldValue(buf []byte, field zapcore.Field) []byte {
	switch field.Type {
	case zapcore.StringType:
		// 尝试解析字符串是否为JSON对象
		if len(field.String) > 0 && field.String[0] == '{' {
			var js interface{}
			if err := jsoniter.Unmarshal([]byte(field.String), &js); err == nil {
				// 是有效的JSON对象，直接追加
				return append(buf, field.String...)
			}
		}
		return appendString(buf, field.String)
	case zapcore.Int64Type:
		return strconv.AppendInt(buf, field.Integer, 10)
	case zapcore.Int32Type:
		return strconv.AppendInt(buf, int64(field.Integer), 10)
	case zapcore.Int16Type:
		return strconv.AppendInt(buf, int64(field.Integer), 10)
	case zapcore.Int8Type:
		return strconv.AppendInt(buf, int64(field.Integer), 10)
	case zapcore.Uint64Type:
		return strconv.AppendUint(buf, uint64(field.Integer), 10)
	case zapcore.Uint32Type:
		return strconv.AppendUint(buf, uint64(field.Integer), 10)
	case zapcore.Uint16Type:
		return strconv.AppendUint(buf, uint64(field.Integer), 10)
	case zapcore.Uint8Type:
		return strconv.AppendUint(buf, uint64(field.Integer), 10)
	case zapcore.Float64Type:
		return strconv.AppendFloat(buf, math.Float64frombits(uint64(field.Integer)), 'f', -1, 64)
	case zapcore.Float32Type:
		return strconv.AppendFloat(buf, float64(math.Float32frombits(uint32(field.Integer))), 'f', -1, 32)
	case zapcore.BoolType:
		return strconv.AppendBool(buf, field.Integer == 1)
	case zapcore.ErrorType:
		if field.Interface != nil {
			if err, ok := field.Interface.(error); ok {
				return appendString(buf, err.Error())
			}
		}
		return append(buf, "null"...)
	case zapcore.TimeType:
		return appendTime(buf, field.Interface, e.timeLayout)
	case zapcore.DurationType:
		return strconv.AppendInt(buf, int64(time.Duration(field.Integer).Milliseconds()), 10)
	case zapcore.ReflectType:
		return e.appendReflectValue(buf, field.Interface)
	default:
		return e.appendReflectValue(buf, field.Interface)
	}
}

// appendTime 优化时间格式化
func appendTime(buf []byte, val interface{}, layout string) []byte {
	if t, ok := val.(time.Time); ok {
		return append(buf, t.Format(layout)...)
	}
	return append(buf, "invalid-time"...)
}

// appendReflectValue 优化反射值处理
func (e *KeyValueEncoder) appendReflectValue(buf []byte, val interface{}) []byte {
	if val == nil {
		return append(buf, "null"...)
	}

	switch v := val.(type) {
	case bool:
		return strconv.AppendBool(buf, v)
	case int:
		return strconv.AppendInt(buf, int64(v), 10)
	case int8:
		return strconv.AppendInt(buf, int64(v), 10)
	case int16:
		return strconv.AppendInt(buf, int64(v), 10)
	case int32:
		return strconv.AppendInt(buf, int64(v), 10)
	case int64:
		return strconv.AppendInt(buf, v, 10)
	case uint:
		return strconv.AppendUint(buf, uint64(v), 10)
	case uint8:
		return strconv.AppendUint(buf, uint64(v), 10)
	case uint16:
		return strconv.AppendUint(buf, uint64(v), 10)
	case uint32:
		return strconv.AppendUint(buf, uint64(v), 10)
	case uint64:
		return strconv.AppendUint(buf, v, 10)
	case float32:
		return strconv.AppendFloat(buf, float64(v), 'f', -1, 32)
	case float64:
		return strconv.AppendFloat(buf, v, 'f', -1, 64)
	case string:
		// 尝试解析字符串是否为JSON对象
		if len(v) > 0 && v[0] == '{' {
			var js interface{}
			if err := jsoniter.Unmarshal([]byte(v), &js); err == nil {
				// 是有效的JSON对象，直接追加
				return append(buf, v...)
			}
		}
		return appendString(buf, v)
	case json.Marshaler:
		if data, err := v.MarshalJSON(); err == nil {
			return append(buf, data...)
		}
	case error:
		return appendString(buf, v.Error())
	case fmt.Stringer:
		return appendString(buf, v.String())
	case []byte:
		// 尝试解析字节数组是否为JSON对象
		if len(v) > 0 && v[0] == '{' {
			var js interface{}
			if err := jsoniter.Unmarshal(v, &js); err == nil {
				// 是有效的JSON对象，直接追加
				return append(buf, v...)
			}
		}
		return appendString(buf, string(v))
	}

	// 对于其他复杂类型，使用 jsoniter 进行高效序列化
	config := jsoniter.Config{
		EscapeHTML:    false,
		SortMapKeys:   true,
		IndentionStep: 0,
	}.Froze()

	if data, err := config.Marshal(val); err == nil {
		return append(buf, data...)
	}

	// 最后的兜底方案，使用 fmt.Sprintf
	return appendString(buf, fmt.Sprintf("%v", val))
}

// needsQuoting 检查字符串是否需要引号
func needsQuoting(s string) bool {
	if len(s) == 0 {
		return true
	}
	for i := 0; i < len(s); i++ {
		if s[i] <= ' ' || s[i] == '=' || s[i] == '"' || s[i] == '\'' {
			return true
		}
	}
	return false
}

// 在使用完 buffer 后，将其放回 pool
//
//	func putBuffer(buf *buffer.Buffer) {
//		if buf != nil {
//			buf.Reset()
//			globalBufferPool.Put(buf)
//		}
//	}
