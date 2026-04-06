# HTTP请求工具

基于 github.com/imroc/req/v3 二次封装的HTTP客户端工具，提供简洁易用的链式调用API。

## 功能特点

- **链式调用风格**：简洁直观的API设计
- **链路追踪集成**：自动添加请求ID和Span ID
- **调试支持**：可开启详细日志，查看请求和响应内容
- **TLS支持**：支持指定证书调用或忽略证书校验
- **灵活请求配置**：支持表单提交、文件上传、JSON编码等
- **重试机制**：可自定义重试次数和条件
- **代理支持**：可配置HTTP代理
- **HTTP/2支持**：可强制使用HTTP/1或HTTP/2
- **连接池管理**：优化性能和资源使用

## 基本用法

### 创建客户端

```go
import (
    "context"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/http/req"
    "time"
)

func main() {
    // 创建基本客户端
    ctx := context.Background()
    client := req.New(ctx)
    
    // 设置超时时间
    client = client.Timeout(10 * time.Second)
    
    // 开启调试模式
    client = client.Debug()
}
```

### 发送GET请求

```go
// 简单GET请求
resp, err := req.New(ctx).Get("https://api.example.com/users")
if err != nil {
    // 处理错误
}

// 获取响应内容
var result map[string]interface{}
err = resp.UnmarshalJson(&result)
if err != nil {
    // 处理错误
}
```

### 发送POST请求

```go
// JSON格式的POST请求
data := map[string]interface{}{
    "name": "张三",
    "age": 30,
}

resp, err := req.New(ctx).
    BodyJson(data).
    Post("https://api.example.com/users")
if err != nil {
    // 处理错误
}

// 获取响应内容
var result struct {
    Code int    `json:"code"`
    Msg  string `json:"msg"`
    Data interface{} `json:"data"`
}
err = resp.UnmarshalJson(&result)
```

### 表单提交

```go
// 普通表单提交
formData := map[string]string{
    "username": "admin",
    "password": "123456",
}

resp, err := req.New(ctx).
    FormData(formData).
    Post("https://api.example.com/login")
```

### 文件上传

```go
// 上传文件
resp, err := req.New(ctx).
    File("avatar", "/path/to/file.jpg").
    FormData(map[string]string{
        "user_id": "12345",
    }).
    Post("https://api.example.com/upload")
```

## 高级功能

### 设置请求头

```go
resp, err := req.New(ctx).
    Header(map[string]string{
        "X-API-Key": "your-api-key",
        "X-Custom-Header": "custom-value",
    }).
    Get("https://api.example.com/data")
```

### 设置认证信息

```go
// Basic认证
client := req.New(ctx).SetBasicToken("username", "password")

// Bearer Token认证
client := req.New(ctx).SetBearerToken("your-jwt-token")

// 自定义Token
client := req.New(ctx).SetToken("Custom your-token-value")
```

### 重试机制

```go
// 简单重试
resp, err := req.New(ctx).
    TryTimes(3).
    Get("https://api.example.com/users")

// 高级重试配置
resp, err := req.New(ctx).
    Retry(&req.RetryConfig{
        Count:     3,
        Interval:  time.Second,
        Condition: func(resp *req.Response, err error) bool {
            // 当响应状态码为429或500以上时重试
            return err != nil || resp.StatusCode == 429 || resp.StatusCode >= 500
        },
    }).
    Get("https://api.example.com/users")
```

### 代理设置

```go
// 设置代理
resp, err := req.New(ctx).
    Proxy("http://proxy.example.com:8080").
    Get("https://api.example.com/users")

// 禁用代理
resp, err := req.New(ctx).
    NoProxy().
    Get("https://api.example.com/users")
```

### TLS配置

```go
// 忽略证书验证
resp, err := req.New(ctx).
    Insecure().
    Get("https://api.example.com/users")

// 设置客户端证书
resp, err := req.New(ctx).
    TLS(&req.TLSConfig{
        CertFile: "/path/to/cert.pem",
        KeyFile:  "/path/to/key.pem",
    }).
    Get("https://api.example.com/users")

// 设置根证书
resp, err := req.New(ctx).
    SetRootCertsFromFile("/path/to/ca.pem").
    Get("https://api.example.com/users")
```

### HTTP版本控制

```go
// 强制使用HTTP/1.1
resp, err := req.New(ctx).
    ForceHttp1().
    Get("https://api.example.com/users")

// 强制使用HTTP/2
resp, err := req.New(ctx).
    ForceHttp2().
    Get("https://api.example.com/users")
```

## 连接池管理

对于需要频繁访问同一主机的场景，可以使用连接池管理来提高性能：

```go
// 获取或创建针对特定主机的客户端
client := req.GetHostClient("api.example.com", &req.Options{
    MaxIdleConns:        100,
    MaxConnsPerHost:     10,
    IdleConnTimeout:     90 * time.Second,
    ResponseHeaderTimeout: 10 * time.Second,
})

// 使用该客户端发送请求
resp, err := client.
    BodyJson(data).
    Post("https://api.example.com/users")
```

## 最佳实践

1. **合理设置超时**：始终为HTTP请求设置合理的超时时间，避免长时间阻塞。

2. **正确处理响应**：请求结束后，需要检查错误并读取响应主体内容，无需关闭响应（库自动处理）。

3. **使用连接池**：对于频繁请求同一服务的场景，使用`GetHostClient`获取针对特定主机的优化客户端。

4. **调试问题**：使用`Debug()`方法开启调试模式，查看详细的请求和响应内容。

5. **处理重试**：对于不稳定的网络环境，配置合理的重试策略，但避免过度重试。

## 注意事项

1. 在生产环境中谨慎使用`Insecure()`跳过证书验证，这可能导致安全风险。

2. 请求完成后无需手动关闭响应体，库会自动处理。

3. 在处理大量并发请求时，注意控制goroutine数量，避免资源耗尽。

4. 链式调用风格意味着每个方法调用都会返回修改后的客户端对象，确保使用返回值进行后续操作。

5. 对于文件上传等大数据传输，应设置足够的超时时间。