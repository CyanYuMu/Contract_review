package trace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	jsoniter "github.com/json-iterator/go"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/enum"
	"google.golang.org/grpc/metadata"
)

// TraceInfo 链路追踪信息结构体
// type TraceInfo struct {
// 	TraceId string
// 	SpanId  string
// }

// GetRequestId 获取request id, 优先从context中获取
func GetRequestId(ctx context.Context) string {
	traceId, _ := ParseFromContext(ctx)
	return traceId
}

// GetSpanId 获取span id
func GetSpanId(ctx context.Context) string {
	_, spanId := ParseFromContext(ctx)
	return spanId
}

// ParseJA3 解析三元素
func ParseCore(ctx context.Context) (traceId string, spanId string, traceParam string, debug string) {
	if ctx == nil {
		return "", "", "", ""
	}
	traceId = GetFromContext(ctx, enum.CtxRequestId)
	spanId = GetFromContext(ctx, enum.CtxSpanId)
	traceParam = GetFromContext(ctx, enum.CtxTrackingParams)
	debug = GetFromContext(ctx, enum.CtxDebug)

	return
}

// ParseFromContext 基于context获取链路id以及spanId
func ParseFromContext(ctx context.Context) (traceId string, spanId string) {
	if ctx == nil {
		return "", ""
	}

	// 优先从普通context中获取
	if v := ctx.Value(enum.CtxRequestId); v != nil {
		if tid, ok := v.(string); ok {
			traceId = tid
		}
	}
	if v := ctx.Value(enum.CtxSpanId); v != nil {
		if sid, ok := v.(string); ok {
			spanId = sid
		}
	}

	// 如果已经从普通context获取到了完整信息，直接返回
	if traceId != "" && spanId != "" {
		return traceId, spanId
	}

	// 从gRPC metadata中获取缺失的信息
	// 由于我们没有 grpc, 所以我先注释掉了
	// if traceId == "" || spanId == "" {
	// 	// 一次性获取metadata，避免重复调用
	// 	var incomingMd, outgoingMd metadata.MD
	// 	var hasIncoming, hasOutgoing bool

	// 	if traceId == "" || spanId == "" {
	// 		incomingMd, hasIncoming = metadata.FromIncomingContext(ctx)
	// 	}
	// 	if !hasIncoming && (traceId == "" || spanId == "") {
	// 		outgoingMd, hasOutgoing = metadata.FromOutgoingContext(ctx)
	// 	}

	// 	// 从incoming metadata获取
	// 	if hasIncoming {
	// 		if traceId == "" {
	// 			if values := incomingMd.Get(enum.CtxRequestId); len(values) > 0 {
	// 				traceId = values[0]
	// 			}
	// 		}
	// 		if spanId == "" {
	// 			if values := incomingMd.Get(enum.CtxSpanId); len(values) > 0 {
	// 				spanId = values[0]
	// 			}
	// 		}
	// 	}

	// 	// 如果incoming metadata中没有找到，尝试outgoing metadata
	// 	if hasOutgoing && (traceId == "" || spanId == "") {
	// 		if traceId == "" {
	// 			if values := outgoingMd.Get(enum.CtxRequestId); len(values) > 0 {
	// 				traceId = values[0]
	// 			}
	// 		}
	// 		if spanId == "" {
	// 			if values := outgoingMd.Get(enum.CtxSpanId); len(values) > 0 {
	// 				spanId = values[0]
	// 			}
	// 		}
	// 	}
	// }

	return traceId, spanId
}

// GetTraceInfo 获取完整的链路信息
// func GetTraceInfo(ctx context.Context) TraceInfo {
// 	traceId, spanId := ParseFromContext(ctx)
// 	return TraceInfo{
// 		TraceId: traceId,
// 		SpanId:  spanId,
// 	}
// }

// getFromMetadata 从gRPC metadata中获取指定key的值
func getFromMetadata(ctx context.Context, key string) string {
	// 尝试从incoming metadata获取
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if values := md.Get(key); len(values) > 0 {
			return values[0]
		}
	}

	// 尝试从outgoing metadata获取
	if md, ok := metadata.FromOutgoingContext(ctx); ok {
		if values := md.Get(key); len(values) > 0 {
			return values[0]
		}
	}

	return ""
}

// NewContextWithTrace 基于context创建新的context, 并且添加request id和span id
func NewContextWithTrace(ctx context.Context, traceId, spanId string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return AssignToContext(ctx, enum.CtxRequestId, traceId, enum.CtxSpanId, spanId)
}

// NewContextWithRequestId 基于context创建新的context, 并且添加request id
func NewContextWithRequestId(ctx context.Context, reqId string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return AssignToContext(ctx, enum.CtxRequestId, reqId)
}

// NewContextWithSpanId 基于context创建新的context, 并且添加span id
func NewContextWithSpanId(ctx context.Context, spanId string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return AssignToContext(ctx, enum.CtxSpanId, spanId)
}

// NewTrace 创建新的链路追踪context
func NewTrace() context.Context {
	traceId := NewTraceId()
	spanId := NewSpanId()
	return NewContextWithTrace(context.Background(), traceId, spanId)
}

// NewTraceWithParent 基于父context创建新的链路追踪context，保持traceId，生成新的spanId
func NewTraceWithParent(parent context.Context) context.Context {
	traceId := GetRequestId(parent)
	if traceId == "" {
		traceId = NewTraceId()
	}
	spanId := NewSpanId()
	return NewContextWithTrace(parent, traceId, spanId)
}

// New 创建带有指定trace和span的context
func New(traceId string, spanId ...string) context.Context {
	if traceId == "" {
		traceId = NewTraceId()
	}
	var sId string
	if len(spanId) > 0 {
		sId = spanId[0]
	}
	if sId == "" {
		sId = NewSpanId()
	}

	return NewContextWithTrace(context.Background(), traceId, sId)
}

// GetFromContext 从ctx中获取指定的key, 优先从普通ctx, 再从metadata中获取
func GetFromContext(ctx context.Context, key string) string {
	if ctx == nil {
		return ""
	}

	// 优先从普通context中获取
	if v := ctx.Value(key); v != nil {
		if value, ok := v.(string); ok {
			return value
		}
	}

	// 从gRPC metadata中获取
	// return getFromMetadata(ctx, key)
	return ""
}

// AssignToContext 赋值到context中, 支持 grpc 以及 常规的context
func AssignToContext(ctx context.Context, kv ...string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	// 检查参数数量是否为偶数
	if len(kv)%2 != 0 {
		return ctx
	}

	// 检查是否为gRPC context
	// if md, ok := metadata.FromIncomingContext(ctx); ok {
	// 	newMd := md.Copy()
	// 	for i := 0; i < len(kv); i += 2 {
	// 		newMd.Set(kv[i], kv[i+1])
	// 	}
	// 	return metadata.NewIncomingContext(ctx, newMd)
	// }

	// if md, ok := metadata.FromOutgoingContext(ctx); ok {
	// 	newMd := md.Copy()
	// 	for i := 0; i < len(kv); i += 2 {
	// 		newMd.Set(kv[i], kv[i+1])
	// 	}
	// 	return metadata.NewOutgoingContext(ctx, newMd)
	// }

	// 普通context
	for i := 0; i < len(kv); i += 2 {
		ctx = context.WithValue(ctx, kv[i], kv[i+1])
	}

	return ctx
}

// ID生成器相关
var (
	idGeneratorOnce sync.Once
	idGenerator     func() string
)

// initIdGenerator 初始化ID生成器
func initIdGenerator() {
	idGeneratorOnce.Do(func() {
		idGenerator = newSecureIdGenerator()
	})
}

// newSecureIdGenerator 创建安全的ID生成器
func newSecureIdGenerator() func() string {
	var mu sync.Mutex
	counter := uint64(0)

	return func() string {
		mu.Lock()
		defer mu.Unlock()

		// 使用时间戳 + 计数器 + 随机数确保唯一性
		timestamp := time.Now().UnixNano()
		counter++

		// 生成4字节随机数
		randomBytes := make([]byte, 4)
		if _, err := rand.Read(randomBytes); err != nil {
			// 如果随机数生成失败，使用时间戳的低位
			randomBytes = []byte{
				byte(timestamp),
				byte(timestamp >> 8),
				byte(timestamp >> 16),
				byte(timestamp >> 24),
			}
		}

		// 组合生成ID: 时间戳(16位) + 计数器(8位) + 随机数(8位)
		return fmt.Sprintf("%016x%08x%s", timestamp, counter, hex.EncodeToString(randomBytes))
	}
}

// NewTraceId 生成新的trace id
func NewTraceId() string {
	initIdGenerator()
	return idGenerator()
}

// NewRequestId 兼容旧的函数名
func NewRequestId() string {
	return NewTraceId()
}

// NewSpanId 生成新的span id
func NewSpanId() string {
	initIdGenerator()
	return idGenerator()
}

// EnsureTrace 确保context中包含链路信息，如果没有则自动生成
func EnsureTrace(ctx context.Context) context.Context {
	if ctx == nil {
		return NewTrace()
	}

	traceId, spanId := ParseFromContext(ctx)
	needUpdate := false

	if traceId == "" {
		traceId = NewTraceId()
		needUpdate = true
	}
	if spanId == "" {
		spanId = NewSpanId()
		needUpdate = true
	}

	if needUpdate {
		return NewContextWithTrace(ctx, traceId, spanId)
	}

	return ctx
}

// NewOutgoingContext 创建用于outgoing调用的context
func NewOutgoingContext(ctx context.Context) context.Context {
	if ctx == nil {
		return metadata.NewOutgoingContext(
			context.Background(),
			metadata.Pairs(enum.CtxRequestId, NewTraceId()),
		)
	}

	// 检查当前ctx是否携带链路id
	if md, ok := metadata.FromOutgoingContext(ctx); ok {
		if len(md.Get(enum.CtxRequestId)) == 0 {
			traceId := GetRequestId(ctx)
			if traceId == "" {
				traceId = NewTraceId()
			}
			newMd := md.Copy()
			newMd.Set(enum.CtxRequestId, traceId)
			return metadata.NewOutgoingContext(ctx, newMd)
		}
		return ctx
	}

	if md, ok := metadata.FromIncomingContext(ctx); ok {
		traceId := GetRequestId(ctx)
		if traceId == "" {
			traceId = NewTraceId()
		}
		newMd := metadata.Pairs(enum.CtxRequestId, traceId)
		// 复制原有的metadata
		for k, v := range md {
			if k != enum.CtxRequestId && len(v) > 0 {
				newMd.Set(k, v[0])
			}
		}
		return metadata.NewOutgoingContext(ctx, newMd)
	}

	// 普通context
	traceId := GetRequestId(ctx)
	if traceId == "" {
		traceId = NewTraceId()
	}
	return metadata.NewOutgoingContext(ctx, metadata.Pairs(enum.CtxRequestId, traceId))
}

// 兼容旧的函数名
func NewOutgoingCtx(ctx context.Context) context.Context {
	return NewOutgoingContext(ctx)
}

// IsValidTraceId 检查trace id是否有效
func IsValidTraceId(traceId string) bool {
	return traceId != "" && len(traceId) >= 16
}

// IsValidSpanId 检查span id是否有效
func IsValidSpanId(spanId string) bool {
	return spanId != "" && len(spanId) >= 16
}

// 废弃的函数，保留向后兼容
// Deprecated: 使用NewContextWithTrace替代
func NewContextWithRequestIdAndSpanId(ctx context.Context, reqId, spanId string) context.Context {
	return NewContextWithTrace(ctx, reqId, spanId)
}

// Deprecated: 移除了WithContext函数，因为它使用了unsafe操作
// 如果需要类似功能，请使用EnsureTrace或NewContextWithTrace

// 自定义参数透传相关常量
const (
	// CustomParamsKey 自定义参数在context中的key
	CustomParamsKey = "CustomParams"
	// CustomParamsMetadataPrefix 自定义参数在gRPC metadata中的前缀
	CustomParamsMetadataPrefix = "x-custom-"
)

// Wrapper 对指定context添加自定义透传参数
// params: 需要透传的自定义参数，支持基本类型和可JSON序列化的复杂类型
// 返回包含自定义参数的新context
func Wrapper(ctx context.Context, params map[string]any) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	if len(params) == 0 {
		return ctx
	}

	// 过滤和序列化参数
	serializedParams := make(map[string]string)
	for key, value := range params {
		if key == "" || value == nil {
			continue
		}

		// 将参数值序列化为字符串
		var strValue string
		switch v := value.(type) {
		case string:
			strValue = v
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			strValue = fmt.Sprintf("%v", v)
		case float32, float64:
			strValue = fmt.Sprintf("%v", v)
		case bool:
			if v {
				strValue = "true"
			} else {
				strValue = "false"
			}
		default:
			// 复杂类型转换为字符串
			strValue = fmt.Sprintf("%v", v)
		}

		serializedParams[key] = strValue
	}

	if len(serializedParams) == 0 {
		return ctx
	}

	// 检查是否为gRPC context
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		newMd := md.Copy()
		for key, value := range serializedParams {
			newMd.Set(CustomParamsMetadataPrefix+key, value)
		}
		return metadata.NewIncomingContext(ctx, newMd)
	}

	if md, ok := metadata.FromOutgoingContext(ctx); ok {
		newMd := md.Copy()
		for key, value := range serializedParams {
			newMd.Set(CustomParamsMetadataPrefix+key, value)
		}
		return metadata.NewOutgoingContext(ctx, newMd)
	}

	// 普通context，将参数存储在context中
	return context.WithValue(ctx, CustomParamsKey, serializedParams)
}

// UnWrapper 解析当前context中的自定义参数
// 返回自定义参数的map，如果没有自定义参数则返回空map
func UnWrapper(ctx context.Context) map[string]any {
	if ctx == nil {
		return make(map[string]any)
	}

	result := make(map[string]any)

	// 优先从普通context中获取
	if v := ctx.Value(CustomParamsKey); v != nil {
		if params, ok := v.(map[string]string); ok {
			for key, value := range params {
				result[key] = deserializeValue(value)
			}
		}
	}

	// 从gRPC metadata中获取
	var md metadata.MD
	var hasMd bool

	// 尝试从incoming metadata获取
	if incomingMd, ok := metadata.FromIncomingContext(ctx); ok {
		md = incomingMd
		hasMd = true
	} else if outgoingMd, ok := metadata.FromOutgoingContext(ctx); ok {
		// 如果没有incoming metadata，尝试outgoing metadata
		md = outgoingMd
		hasMd = true
	}

	if hasMd {
		for key, values := range md {
			if len(values) > 0 && len(key) > len(CustomParamsMetadataPrefix) &&
				key[:len(CustomParamsMetadataPrefix)] == CustomParamsMetadataPrefix {

				paramKey := key[len(CustomParamsMetadataPrefix):]
				// metadata中的参数优先级低于context中的参数
				if _, exists := result[paramKey]; !exists {
					result[paramKey] = deserializeValue(values[0])
				}
			}
		}
	}

	return result
}

// deserializeValue 反序列化参数值
func deserializeValue(value string) any {
	if value == "" {
		return ""
	}

	// 尝试解析为基本类型
	switch value {
	case "true":
		return true
	case "false":
		return false
	}

	// 尝试解析为数字
	if len(value) > 0 {
		// 检查是否为整数
		if value[0] == '-' || (value[0] >= '0' && value[0] <= '9') {
			// 简单的数字检查
			isInt := true
			hasDecimal := false
			for i, c := range value {
				if i == 0 && c == '-' {
					continue
				}
				if c == '.' && !hasDecimal {
					hasDecimal = true
					isInt = false
					continue
				}
				if c < '0' || c > '9' {
					isInt = false
					hasDecimal = false
					break
				}
			}

			if isInt && !hasDecimal {
				// 尝试解析为整数
				var intVal int64
				if _, err := fmt.Sscanf(value, "%d", &intVal); err == nil {
					return intVal
				}
			} else if hasDecimal {
				// 尝试解析为浮点数
				var floatVal float64
				if _, err := fmt.Sscanf(value, "%f", &floatVal); err == nil {
					return floatVal
				}
			}
		}
	}

	// 尝试JSON反序列化
	if len(value) > 0 && (value[0] == '{' || value[0] == '[' || value[0] == '"') {
		var jsonValue any
		if err := jsoniter.Unmarshal([]byte(value), &jsonValue); err == nil {
			return jsonValue
		}
	}

	// 默认返回字符串
	return value
}

// WrapperWithTrace 便捷函数：同时设置链路信息和自定义参数
func WrapperWithTrace(ctx context.Context, traceId, spanId string, params map[string]any) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	// 先设置链路信息
	ctx = NewContextWithTrace(ctx, traceId, spanId)

	// 再设置自定义参数
	return Wrapper(ctx, params)
}

// GetCustomParam 获取指定的自定义参数
func GetCustomParam(ctx context.Context, key string) (any, bool) {
	params := UnWrapper(ctx)
	value, exists := params[key]
	return value, exists
}

// GetCustomParamString 获取指定的自定义参数并转换为字符串
func GetCustomParamString(ctx context.Context, key string) string {
	if value, exists := GetCustomParam(ctx, key); exists {
		if str, ok := value.(string); ok {
			return str
		}
		return fmt.Sprintf("%v", value)
	}
	return ""
}

// GetCustomParamInt 获取指定的自定义参数并转换为整数
func GetCustomParamInt(ctx context.Context, key string) (int64, bool) {
	if value, exists := GetCustomParam(ctx, key); exists {
		switch v := value.(type) {
		case int64:
			return v, true
		case int:
			return int64(v), true
		case int32:
			return int64(v), true
		case float64:
			return int64(v), true
		case string:
			var result int64
			if _, err := fmt.Sscanf(v, "%d", &result); err == nil {
				return result, true
			}
		}
	}
	return 0, false
}

// GetCustomParamBool 获取指定的自定义参数并转换为布尔值
func GetCustomParamBool(ctx context.Context, key string) (bool, bool) {
	if value, exists := GetCustomParam(ctx, key); exists {
		switch v := value.(type) {
		case bool:
			return v, true
		case string:
			return v == "true", true
		case int64:
			return v != 0, true
		case float64:
			return v != 0, true
		}
	}
	return false, false
}
