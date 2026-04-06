# 分布式锁工具使用手册

## 概述

`utils/locker`包提供了简单易用的分布式锁实现，可应用于分布式系统中的并发控制、资源同步和临界区保护。该包支持多种后端存储（如Redis）实现锁机制，并提供了看门狗(WatchDog)功能，用于避免业务执行期间锁过期导致的问题。

## 主要功能

- **分布式锁获取与释放**：支持基本的锁操作
- **自动重试机制**：可配置尝试获取锁的策略
- **超时控制**：支持设置锁的过期时间和获取锁的最大等待时间
- **看门狗机制**：自动延长锁的有效期，避免长时间任务执行过程中锁过期
- **Try-Lock模式**：支持非阻塞的锁获取方式

## 基本使用

### 配置分布式锁

```go
import (
    "context"
    "fmt"
    "time"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/locker"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/cache/v3"
)

func main() {
    // 创建锁的后端存储实例(例如使用Redis)
    redisConf := &cache.RedisConfig{
        Addrs:         []string{"127.0.0.1:6379"},
        Password:      "密码",
        DB:            0,
        PoolSize:      10,
        MinIdleConn:   5,
        Prefix:        "myapp",
    }
    
    // 使用Redis作为锁的实现
    redis := cache.NewRedisV9(redisConf)
    
    // Redis实现了locker.Locker接口，可直接用作分布式锁
}
```

### 获取并使用分布式锁

```go
// 配置锁参数
param := locker.Param{
    Key: "resource_lock:123",    // 锁的唯一标识
    Val: "random_value_123456",  // 随机值，用于安全释放锁
    Freq: time.Millisecond * 50, // 轮询间隔
    TTL: time.Second * 30,       // 锁的过期时间
    Func: func() error {
        // 获取锁后执行的业务逻辑
        fmt.Println("执行受保护的业务代码")
        time.Sleep(time.Second * 2) // 模拟业务处理时间
        return nil
    },
}

// 使用锁执行业务逻辑
ctx := context.Background()
err := locker.Exec(ctx, redis, param)
if err != nil {
    if err == locker.ErrFailToGetLock {
        fmt.Println("无法获取锁，资源可能被占用")
    } else {
        fmt.Println("执行出错:", err)
    }
}
```

## 高级用法

### 尝试加锁（非阻塞）

非阻塞方式尝试加锁，如果获取不到锁则立即返回失败：

```go
param := locker.Param{
    Key: "resource_lock:123",
    Val: "random_value_123456",
    TTL: time.Second * 30,
    TryLock: true,  // 设置为尝试模式，获取不到锁立即返回
    Func: func() error {
        // 业务代码
        return nil
    },
}

err := locker.Exec(ctx, redis, param)
if err == locker.ErrFailToGetLock {
    fmt.Println("资源已被锁定，稍后再试")
    return
}
```

### 设置获取锁的超时时间

尝试在指定时间内获取锁，超时则返回失败：

```go
param := locker.Param{
    Key: "resource_lock:123",
    Val: "random_value_123456",
    Freq: time.Millisecond * 100, // 每100ms尝试一次
    TTL: time.Second * 30,
    Duration: time.Second * 5,    // 最多尝试5秒
    Func: func() error {
        // 业务代码
        return nil
    },
}

err := locker.Exec(ctx, redis, param)
if err == locker.ErrFailToGetLock {
    fmt.Println("5秒内无法获取锁，操作取消")
    return
}
```

### 启用看门狗机制

对于执行时间较长的任务，可以启用看门狗机制，自动延长锁的有效期：

```go
param := locker.Param{
    Key: "resource_lock:123",
    Val: "random_value_123456",
    TTL: time.Second * 30,
    EnableWatchDog: true,  // 启用看门狗
    Func: func() error {
        // 长时间运行的业务逻辑（可能超过TTL）
        fmt.Println("执行长时间任务...")
        time.Sleep(time.Second * 60) // 模拟长时间运行
        return nil
    },
}

err := locker.Exec(ctx, redis, param)
```

### 不自动释放锁

在某些场景下，可能需要手动控制锁的释放时机：

```go
param := locker.Param{
    Key: "resource_lock:123",
    Val: "random_value_123456",
    TTL: time.Second * 30,
    NotReleaseLock: true,  // 不自动释放锁
    Func: func() error {
        // 业务代码
        return nil
    },
}

err := locker.Exec(ctx, redis, param)
// 业务代码执行完成后，锁不会自动释放
// 需要手动释放锁
_, unlockErr := redis.Unlock(ctx, "resource_lock:123", "random_value_123456")
if unlockErr != nil {
    fmt.Println("释放锁失败:", unlockErr)
}
```

## 参数说明

`Param`结构体中的字段说明：

| 参数名 | 类型 | 说明 | 默认值 |
| --- | --- | --- | --- |
| Key | string | 锁的唯一标识 | 必填 |
| Val | string | 随机值，用于安全释放锁 | 必填 |
| Freq | time.Duration | 轮询间隔，获取锁失败后的重试间隔 | 10ms |
| TTL | time.Duration | 锁的过期时间 | 1s |
| TryLock | bool | 是否是尝试加锁，true表示获取失败立即返回 | false |
| Duration | time.Duration | 尝试获取锁的最大时长，0表示一直尝试 | 0 |
| Func | func() error | 获取锁成功后执行的方法 | 必填 |
| EnableWatchDog | bool | 是否启动看门狗，防止锁在业务执行期间过期 | false |
| NotReleaseLock | bool | 是否不自动释放锁 | false |

## 最佳实践

1. **设置合适的TTL**：锁的过期时间应该根据业务执行时间合理设置，一般设置为预期执行时间的2-3倍。

2. **使用随机值**：锁的值应使用唯一的随机值，可以使用UUID等方式生成，防止误解锁。

3. **考虑启用看门狗**：对于可能长时间运行的任务，建议启用看门狗机制，避免锁过期导致的并发问题。

4. **处理获取锁失败的情况**：在高并发场景下，应妥善处理获取锁失败的情况，可以选择稍后重试或者返回友好的错误提示。

5. **合理设置重试间隔**：根据业务需求和系统负载，合理设置获取锁的重试间隔，避免过度频繁的请求。

## 注意事项

1. 分布式锁并不保证绝对的一致性，在网络分区等极端情况下可能出现问题，应结合业务特性选择合适的锁策略。

2. 看门狗机制运行在单独的goroutine中，如果主函数panic，可能导致看门狗无法正常停止，建议在外层使用recover捕获异常。

3. 使用`NotReleaseLock=true`时，必须手动释放锁，否则可能导致资源长时间被锁定。

4. 在高并发环境下，过多的锁竞争可能导致系统性能下降，应考虑使用粒度更小的锁或其他并发控制手段。

5. 锁的Key应该具有明确的命名规范，建议使用`资源类型:资源ID`的格式，如`order:12345`，便于问题排查。
