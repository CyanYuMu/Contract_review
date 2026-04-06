# su_logger

项目统一的日志组件

## 使用须知

su_logger.E() 的作用标识追加额外信息, 额外信息是作为可选项存在的

## 日志方法

### Info - 记录普通信息

```go
// 记录用户登录信息
su_logger.Info(ctx, "用户登录成功", su_logger.E().Int("user_id", 123))
```

### Debug - 记录调试信息

```go
// 记录请求详情，仅在开发/测试环境显示
su_logger.Debug(ctx, "接收到请求", su_logger.E().String("path", "/api/users").Int("size", 10))
```

### Warn - 记录警告信息

```go
// 记录可能的问题，但不影响主要功能
su_logger.Warn(ctx, "API响应缓慢", su_logger.E().Float64("response_time", 1.5))
```

### Error - 记录错误信息

```go
// 记录错误，第二个参数是error对象
if err != nil {
    su_logger.Error(ctx, err, "数据库查询失败", su_logger.E().String("table", "users"))
}
```

### Panic - 记录严重错误并捕获堆栈

```go
// 记录导致程序崩溃的严重错误
su_logger.Panic(ctx, "未处理的异常", su_logger.E().Any("data", requestData))
```

### InfoWithNotify - 记录信息并发送通知

```go
// 重要信息，需要通知团队
su_logger.InfoWithNotify(ctx, "定时任务完成", su_logger.E().Int("processed", 1000))
```

### WarnWithNotify - 记录警告并发送通知

```go
// 警告信息，需要及时关注
su_logger.WarnWithNotify(ctx, "接口限流触发", su_logger.E().Int("current_qps", 1000))
```

### ErrorWithNotify - 记录错误并发送通知

```go
// 严重错误，需要立即处理
if err != nil {
    su_logger.ErrorWithNotify(ctx, err, "支付系统异常", su_logger.E().String("order_id", "123456"))
}
```

### PanicWithNotify - 记录崩溃并发送通知

```go
// 系统崩溃，自动捕获堆栈并通知
su_logger.PanicWithNotify(ctx, "系统崩溃", su_logger.E().String("module", "core"))
```

## 链路追踪

```go
// 从上下文提取trace_id和span_id
su_logger.Info(ctx, "处理中", su_logger.E().Ctx(ctx))

// 生成新的链路上下文（用于定时任务等）
autoCtx := su_logger.AutoCtx(context.Background())
```

## 结构化字段

通过`su_logger.E()`添加字段：

```go
su_logger.E().String("key", "value")  // 字符串
su_logger.E().Int("count", 42)        // 整数
su_logger.E().Bool("success", true)   // 布尔值
su_logger.E().Float64("score", 9.5)   // 浮点数
su_logger.E().Error(err)              // 错误对象
su_logger.E().Any("data", myStruct)   // 任意类型（JSON序列化）
```

## 性能注意事项

1. 使用`E()`的对象池自动回收
2. 避免热路径中使用`Any()`（JSON序列化开销大）
3. 不要记录敏感信息
4. 谨慎使用带通知的方法 