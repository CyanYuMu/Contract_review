# Trace 包使用说明

trace 包提供了链路追踪和自定义参数透传的功能，支持在 gRPC 和普通 context 中透传链路信息和自定义参数。

## 主要功能

### 1. 链路追踪功能

#### 基本使用
```go
// 创建新的链路追踪
ctx := trace.NewTrace()
traceId, spanId := trace.ParseFromContext(ctx)

// 基于父链路创建子链路
childCtx := trace.NewTraceWithParent(ctx)

// 确保context包含链路信息
ctx = trace.EnsureTrace(ctx)

// 获取链路信息
traceId := trace.GetRequestId(ctx)
spanId := trace.GetSpanId(ctx)
```

#### 创建 outgoing context
```go
// 用于gRPC调用的context
outCtx := trace.NewOutgoingContext(ctx)
```

### 2. 自定义参数透传功能 ⭐️

#### Wrapper - 添加自定义参数
```go
// 基本类型参数
params := map[string]any{
    "user_id": "12345",
    "role":    "admin", 
    "count":   42,
    "active":  true,
}
ctx = trace.Wrapper(ctx, params)

// 复杂类型参数（自动JSON序列化）
complexParams := map[string]any{
    "user_info": map[string]interface{}{
        "name": "张三",
        "age":  30,
    },
    "permissions": []string{"read", "write", "admin"},
}
ctx = trace.Wrapper(ctx, complexParams)
```

#### UnWrapper - 获取自定义参数
```go
// 获取所有自定义参数
allParams := trace.UnWrapper(ctx)

// 获取指定参数
userID, exists := trace.GetCustomParam(ctx, "user_id")
if exists {
    fmt.Printf("User ID: %v\n", userID)
}

// 类型安全的获取方法
userIDStr := trace.GetCustomParamString(ctx, "user_id")
count, exists := trace.GetCustomParamInt(ctx, "count")
isActive, exists := trace.GetCustomParamBool(ctx, "active")
```

#### 同时设置链路和自定义参数
```go
params := map[string]any{
    "user_id": "12345",
    "action":  "create_order",
}
ctx = trace.WrapperWithTrace(ctx, "trace-123", "span-456", params)
```

## 支持的参数类型

### 基本类型
- `string`
- `int`, `int8`, `int16`, `int32`, `int64`
- `uint`, `uint8`, `uint16`, `uint32`, `uint64`
- `float32`, `float64`
- `bool`

### 复杂类型
- `map[string]interface{}`
- `[]interface{}`
- 任何可JSON序列化的类型

## 使用场景

### 1. 微服务间参数透传
```go
// 服务A
params := map[string]any{
    "tenant_id": "tenant_123",
    "user_role": "admin",
    "request_source": "web",
}
ctx = trace.Wrapper(ctx, params)

// 调用服务B
response, err := serviceB.Call(ctx, request)

// 服务B中获取参数
tenantID := trace.GetCustomParamString(ctx, "tenant_id")
userRole := trace.GetCustomParamString(ctx, "user_role")
```

### 2. 请求上下文信息传递
```go
// 在HTTP中间件中设置
func TracingMiddleware(c *fiber.Ctx) error {
    params := map[string]any{
        "request_id": c.Get("X-Request-ID"),
        "user_agent": c.Get("User-Agent"),
        "client_ip":  c.IP(),
    }
    
    ctx := trace.Wrapper(c.Context(), params)
    c.SetUserContext(ctx)
    
    return c.Next()
}

// 在业务逻辑中使用
func BusinessLogic(ctx context.Context) error {
    requestID := trace.GetCustomParamString(ctx, "request_id")
    clientIP := trace.GetCustomParamString(ctx, "client_ip")
    
    su_logger.Info(ctx, "processing request", 
        su_logger.E().String("request_id", requestID).String("client_ip", clientIP))
    
    return nil
}
```

### 3. gRPC 调用中的参数透传
```go
// 客户端
params := map[string]any{
    "correlation_id": "corr_123",
    "priority": "high",
}
ctx = trace.Wrapper(ctx, params)
ctx = trace.NewOutgoingContext(ctx) // 转换为outgoing context

response, err := grpcClient.Method(ctx, request)

// 服务端
func (s *Server) Method(ctx context.Context, req *Request) (*Response, error) {
    correlationID := trace.GetCustomParamString(ctx, "correlation_id")
    priority := trace.GetCustomParamString(ctx, "priority")
    
    // 业务逻辑...
    return &Response{}, nil
}
```

## 性能特性

- **Wrapper**: ~203 ns/op, 386 B/op, 4 allocs/op
- **UnWrapper**: ~614 ns/op, 520 B/op, 12 allocs/op
- 使用 jsoniter 进行高性能 JSON 序列化
- 支持普通 context 和 gRPC metadata 两种存储方式

## 注意事项

1. **参数大小限制**: 建议单个参数值不超过 1KB，避免影响性能
2. **类型转换**: 数字类型会自动转换，字符串数字会被反序列化为对应的数字类型
3. **优先级**: 普通 context 中的参数优先级高于 gRPC metadata 中的参数
4. **序列化**: 复杂类型会被 JSON 序列化，确保类型可序列化
5. **gRPC 兼容**: 在 gRPC metadata 中，自定义参数会添加 `x-custom-` 前缀

## 最佳实践

1. **参数命名**: 使用清晰的参数名，避免与系统参数冲突
2. **类型一致性**: 在同一个参数名下保持类型一致
3. **错误处理**: 使用类型安全的获取方法，检查参数是否存在
4. **性能考虑**: 对于频繁调用的场景，考虑缓存反序列化结果
5. **文档化**: 在团队中明确定义透传参数的规范和用途 