# Breaker熔断器使用手册

## 概述

`breaker`包提供了一个基于[hystrix-go](https://github.com/afex/hystrix-go/hystrix)的熔断器实现，用于保护系统免受过载或故障级联影响。熔断器可以在检测到高错误率或超时时，自动切断对外部服务的请求，避免系统资源耗尽，并在服务恢复后自动恢复正常请求。

## 主要功能

```go
// 创建新的熔断器
func New(name string, conf Conf) *Breaker

// 使用熔断器执行函数
func (b *Breaker) Do(ctx context.Context, run func(ctx context.Context) error, fallback func(context.Context, error) error) error
```

## 配置参数

熔断器配置使用`Conf`结构体：

```go
type Conf struct {
    // 单次操作的超时时间, 超过设定时间直接返回错误
    Timeout time.Duration
    
    // 相同命令(name)在同一时间的最大并发数
    MaxConcurrentRequests int
    
    // 触发熔断器检测的最小处理数量
    RequestVolumeThreshold int
    
    // 错误率阈值, 达到阈值, 启动熔断, n%
    ErrorPercentThreshold int
    
    // 当熔断开始后, 尝试恢复正常的探测周期
    SleepWindow time.Duration
}
```

## 使用示例

### 基本用法

```go
import (
    "context"
    "errors"
    "fmt"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/breaker"
    "time"
)

func main() {
    // 创建熔断器配置
    conf := breaker.Conf{
        Timeout:                2 * time.Second,     // 超时时间
        MaxConcurrentRequests:  100,                 // 最大并发数
        RequestVolumeThreshold: 10,                  // 最小请求阈值
        ErrorPercentThreshold:  50,                  // 错误百分比阈值
        SleepWindow:            5 * time.Second,     // 熔断恢复时间窗口
    }
    
    // 创建熔断器实例
    b := breaker.New("my_service", conf)
    
    // 使用熔断器执行函数
    err := b.Do(
        context.Background(),
        // 主要执行函数
        func(ctx context.Context) error {
            // 这里放置调用外部服务的代码
            // 如果服务异常(错误或超时)，将触发fallback
            return callExternalService(ctx)
        },
        // 降级函数(fallback)，当主函数失败时执行
        func(ctx context.Context, err error) error {
            fmt.Printf("服务调用失败: %v，执行降级策略\n", err)
            return errors.New("服务暂时不可用")
        },
    )
    
    if err != nil {
        fmt.Println("最终结果:", err)
    }
}

func callExternalService(ctx context.Context) error {
    // 模拟外部服务调用
    // ...
    return nil
}
```

### 多个外部服务调用

```go
// 为每个外部服务创建独立的熔断器
userServiceBreaker := breaker.New("user_service", userServiceConf)
orderServiceBreaker := breaker.New("order_service", orderServiceConf)
paymentServiceBreaker := breaker.New("payment_service", paymentServiceConf)

// 调用用户服务
userErr := userServiceBreaker.Do(
    ctx,
    func(ctx context.Context) error { return callUserService(ctx) },
    func(ctx context.Context, err error) error { return handleUserServiceFailure(ctx, err) },
)

// 调用订单服务
orderErr := orderServiceBreaker.Do(
    ctx,
    func(ctx context.Context) error { return callOrderService(ctx) },
    func(ctx context.Context, err error) error { return handleOrderServiceFailure(ctx, err) },
)
```

## 工作原理

熔断器有三种状态：

1. **关闭状态(Closed)**：正常状态，请求正常通过熔断器。但如果错误率超过阈值，转为开启状态。

2. **开启状态(Open)**：所有请求被拒绝，直接进入fallback流程。在经过`SleepWindow`时间后，进入半开状态。

3. **半开状态(Half-Open)**：允许部分请求通过以探测服务是否恢复：
   - 如果请求成功，则认为服务已恢复，回到关闭状态
   - 如果请求失败，则回到开启状态，继续等待`SleepWindow`时间

## 配置建议

根据不同服务的特性，建议以下配置值：

- **Timeout**: 根据服务的正常响应时间确定，通常设为正常响应时间的2倍
- **MaxConcurrentRequests**: 根据系统的处理能力确定，避免过高造成资源耗尽
- **RequestVolumeThreshold**: 建议至少设为10-20，避免偶发错误触发熔断
- **ErrorPercentThreshold**: 通常设为50%，即错误率超过50%时触发熔断
- **SleepWindow**: 根据服务恢复时间确定，通常在5-30秒之间

## 日志记录

熔断器会自动将状态变化和错误记录到日志系统。日志使用`su_logger`包进行记录，无需额外配置。

## 注意事项

1. **合理命名**: 为每个服务或操作使用唯一的名称创建熔断器，便于监控和排查

2. **降级策略**: 确保fallback函数提供有意义的降级服务，例如：
   - 返回缓存数据
   - 返回默认值
   - 提供有限功能

3. **超时设置**: 合理设置Timeout，避免设置过短导致过早触发熔断

4. **避免级联失败**: 在一个调用链上的多个服务间，每个服务都应该有独立的熔断配置

5. **监控**: 定期检查熔断器的状态和触发情况，及时调整配置
