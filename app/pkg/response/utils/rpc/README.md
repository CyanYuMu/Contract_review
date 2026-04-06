# RPC服务调用组件

## 概述

RPC组件是基于HTTP/1.1协议的底层通信工具，专注于服务间HTTP调用，集成了熔断保护、重试策略和链路追踪功能。该组件基于现有的`req`包和`breaker`包构建，为微服务架构提供可靠的HTTP调用能力。

## 设计理念

- **最小功能闭环**: 专注于核心的HTTP调用功能，避免过度设计
- **高可用性**: 通过可配置的熔断策略保护系统免受级联故障影响
- **易用性**: 提供简洁直接的API，降低使用门槛
- **可扩展性**: 基于现有组件构建，支持未来功能扩展

## 核心功能

### 🔥 服务间HTTP调用
- 支持context、method、url、request参数
- 统一的Request对象封装body、headers、cookies
- 支持GET、POST、PUT、DELETE等HTTP方法

### 🛡️ 熔断保护
- 集成breaker熔断器组件
- 可配置的熔断策略
- 自动降级处理

### 🔄 重试机制
- 可配置重试次数
- 支持超时控制
- 智能重试策略

### 🔧 连接池管理
- 创建时指定连接池大小
- 优化性能和资源使用

## 核心设计

### Request对象
```go
type Request struct {
    Body    interface{}            // 请求体数据
    Headers map[string]string      // 请求头
    Cookies []*http.Cookie         // Cookie信息
}
```

### RPCClient结构
```go
type RPCClient struct {
    client      *req.Client        // HTTP客户端
    breaker     *breaker.Breaker   // 熔断器
    poolSize    int                // 连接池大小
    serviceName string             // 服务名称
}
```

### CallOptions配置
```go
type CallOptions struct {
    Timeout    time.Duration       // 超时时间
    RetryCount int                 // 重试次数
}
```

### 核心调用方法
```go
func (c *RPCClient) Call(ctx context.Context, method string, url string, request *Request, opts *CallOptions) (*Response, error)
```

## 基本用法

### 创建RPC客户端

```go
import (
    "context"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/rpc"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/breaker"
    "time"
)

func main() {
    ctx := context.Background()
    
    // 创建熔断器配置
    breakerConf := breaker.Conf{
        Timeout:                3 * time.Second,
        MaxConcurrentRequests:  100,
        RequestVolumeThreshold: 10,
        ErrorPercentThreshold:  40,
        SleepWindow:            15 * time.Second,
    }
    
    // 创建RPC客户端，指定连接池大小为100
    client := rpc.NewRPC("user-service", 100, breakerConf)
}
```

### 发送GET请求

```go
// 构建请求对象
req := &rpc.Request{
    Headers: map[string]string{
        "Authorization": "Bearer token",
        "X-API-Version": "v1",
    },
}

// 调用选项
opts := &rpc.CallOptions{
    Timeout:    5 * time.Second,
    RetryCount: 3,
}

// 执行GET请求
resp, err := client.Call(ctx, "GET", "http://192.168.1.100:8080/api/users/123", req, opts)
if err != nil {
    su_logger.Error(ctx, err, "get user failed", 
        su_logger.E().String("service", "user-service"))
    return err
}

// 解析响应
var user User
err = resp.Unmarshal(&user)
```
// 构建请求对象
req := &rpc.Request{
    Headers: map[string]string{
        "Authorization": "Bearer token",
        "X-API-Version": "v1",
    },
}

// 调用选项
opts := &rpc.CallOptions{
    Timeout:    5 * time.Second,
    RetryCount: 3,
}

// 执行GET请求
resp, err := client.Call(ctx, "GET", "http://192.168.1.100:8080/api/users/123", req, opts)
if err != nil {
    su_logger.Error(ctx, err, "get user failed", 
        su_logger.E().String("service", "user-service"))
    return err
}

// 解析响应
var user User
err = resp.Unmarshal(&user)
```

### 发送POST请求

```go
// 构建请求对象
req := &rpc.Request{
    Body: map[string]interface{}{
        "name": "张三",
        "email": "zhangsan@example.com",
    },
    Headers: map[string]string{
        "Content-Type": "application/json",
        "Authorization": "Bearer token",
    },
    Cookies: []*http.Cookie{
        {Name: "session", Value: "abc123"},
    },
}

// 调用选项
opts := &rpc.CallOptions{
    Timeout:    10 * time.Second,
    RetryCount: 2,
}

// 执行POST请求
resp, err := client.Call(ctx, "POST", "http://192.168.1.100:8080/api/users", req, opts)
if err != nil {
    su_logger.Error(ctx, err, "create user failed", 
        su_logger.E().String("service", "user-service"))
    return err
}
```
```

### 发送PUT请求

```go
// 构建请求对象
req := &rpc.Request{
    Body: map[string]interface{}{
        "name": "李四",
        "email": "lisi@example.com",
    },
    Headers: map[string]string{
        "Content-Type": "application/json",
    },
}

// 执行PUT请求
resp, err := client.Call(ctx, "PUT", "http://192.168.1.100:8080/api/users/123", req, opts)
```

### 发送DELETE请求

```go
// 构建请求对象
req := &rpc.Request{
    Headers: map[string]string{
        "Authorization": "Bearer token",
    },
}

// 执行DELETE请求
resp, err := client.Call(ctx, "DELETE", "http://192.168.1.100:8080/api/users/123", req, opts)
```

## 错误处理

### 错误类型判断

```go
if err != nil {
    switch {
    case rpc.IsCircuitBreakerError(err):
        // 熔断错误处理
        su_logger.Warn(ctx, "circuit breaker triggered", 
            su_logger.E().String("service", "user-service"))
        return handleCircuitBreaker()
        
    case rpc.IsTimeoutError(err):
        // 超时错误处理
        su_logger.Error(ctx, err, "request timeout", 
            su_logger.E().String("service", "user-service"))
        return handleTimeout()
        
    default:
        // 其他错误
        su_logger.Error(ctx, err, "rpc call failed", 
            su_logger.E().String("service", "user-service"))
        return err
    }
}
```

## 配置建议

### 不同环境的配置

```go
// 生产环境配置
func getProdConfig() breaker.Conf {
    return breaker.Conf{
        Timeout:                3 * time.Second,
        MaxConcurrentRequests:  100,
        RequestVolumeThreshold: 10,
        ErrorPercentThreshold:  40,
        SleepWindow:            15 * time.Second,
    }
}

// 测试环境配置
func getTestConfig() breaker.Conf {
    return breaker.Conf{
        Timeout:                5 * time.Second,
        MaxConcurrentRequests:  50,
        RequestVolumeThreshold: 5,
        ErrorPercentThreshold:  50,
        SleepWindow:            10 * time.Second,
    }
}

// 使用示例
func createRPCClient(env string, serviceName string) *rpc.RPCClient {
    var conf breaker.Conf
    switch env {
    case "prod":
        conf = getProdConfig()
    case "test":
        conf = getTestConfig()
    default:
        conf = getTestConfig()
    }
    
    return rpc.NewRPC(serviceName, 100, conf)
}
```

## 最佳实践

### 1. 合理设置连接池大小
```go
// 根据服务负载设置连接池大小
// 轻量级服务：50-100
// 中等负载服务：100-200  
// 高负载服务：200-500
client := rpc.NewRPC("user-service", 100, breakerConf)
```

### 2. 统一错误处理
```go
func handleRPCError(ctx context.Context, err error, serviceName string) error {
    switch {
    case rpc.IsCircuitBreakerError(err):
        su_logger.Warn(ctx, "circuit breaker activated", 
            su_logger.E().String("service", serviceName))
        return enum.ErrServiceUnavailable
        
    case rpc.IsTimeoutError(err):
        su_logger.Error(ctx, err, "request timeout", 
            su_logger.E().String("service", serviceName))
        return enum.ErrRequestTimeout
        
    default:
        su_logger.Error(ctx, err, "rpc call failed", 
            su_logger.E().String("service", serviceName))
        return err
    }
}
```

### 3. 请求对象复用
```go
// 创建基础请求对象
func createBaseRequest() *rpc.Request {
    return &rpc.Request{
        Headers: map[string]string{
            "Content-Type": "application/json",
            "X-Client-Version": "v1.0.0",
        },
    }
}

// 使用示例
func getUser(ctx context.Context, userID string) (*User, error) {
    req := createBaseRequest()
    req.Headers["Authorization"] = "Bearer " + getToken()
    
    resp, err := client.Call(ctx, "GET", 
        fmt.Sprintf("http://api.example.com/users/%s", userID), 
        req, opts)
    // ... 处理响应
}
```

## 注意事项

1. **资源管理**: 正确关闭响应体，避免内存泄漏
2. **超时设置**: 根据服务特性设置合理的超时时间
3. **重试策略**: 避免对幂等性操作设置过多重试次数
4. **熔断配置**: 根据服务稳定性调整熔断阈值
5. **连接池**: 根据并发量合理设置连接池大小

## 未来规划

- **gRPC支持**: 集成gRPC协议支持
- **服务发现**: 集成服务注册与发现
- **负载均衡**: 支持多种负载均衡策略
- **配置中心**: 支持动态配置更新
- **指标监控**: 集成Prometheus指标导出 