# Packer 包

`packer` 是一个用于简化Redis缓存操作的Go语言工具包，它提供了简洁的API来实现缓存查询、设置和批量处理功能，支持字符串和哈希表等数据类型，并提供防缓存穿透机制。

## 主要功能

- 字符串缓存获取与设置 (GetSetString/GetSetStringV2)
- 批量字符串缓存操作 (PipeStringGetSet/PipeStringGetSetV2)
- 批量哈希表缓存操作 (PipeHashGetSet/PipeHashGetSetV2)
- 防缓存穿透机制
- 分布式锁支持

## 安装

```bash
go get gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/packer
```

## 使用示例

### 批量获取哈希表数据 (PipeHashGetSetV2)

```go
import (
    "context"
    "time"
    
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/packer"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/cache/v3"
)

func GetStatistics(ctx context.Context, ids []string) (*packer.HashRs, error) {
    rs, err := packer.PipeHashGetSetV2(ctx, a.redis, ids, func(ctx context.Context, keys []string) (items map[string]packer.HashItem, err error) {
        // 从数据源查询获取数据
        // 这里应实现从数据库或其他来源获取数据的逻辑
        // 返回的数据需要实现 packer.HashItem 接口
        
        items = make(map[string]packer.HashItem)
        // 填充 items...
        
        return items, nil
    }, &packer.PipeHashGetSetOption{
        GetCacheKey: func(k string) string {
            return GetStatCacheKey(0, k)
        },
        Ttl:        time.Hour * 24,
        Fields:     nil,
        EmptyTtl:   time.Minute * 5,
        Decoder:    model.DecodeStatCacheMap,
        LockOption: nil,
    })
    
    return rs, err
}

#### 示例详解

在上面的示例中，`PipeHashGetSetV2` 函数用于批量获取哈希表数据，具体参数说明：

1. **ctx (context.Context)**: 上下文对象，用于控制请求的生命周期和携带请求相关信息。

2. **a.redis (cache.Rds)**: Redis客户端实例，实现了 `cache.Rds` 接口。

3. **ids ([]string)**: 需要查询的键列表，可以是用户ID、商品ID等唯一标识符。

4. **回调函数**: 当缓存未命中时，用于从数据源（如数据库）获取数据的函数。
   ```go
   func(ctx context.Context, keys []string) (items map[string]packer.HashItem, err error)
   ```
   - 该函数接收未命中缓存的键列表
   - 需要返回一个映射，键为原始键，值为实现了 `packer.HashItem` 接口的数据对象
   - 返回的数据会被自动写入缓存
   
5. **PipeHashGetSetOption**: 缓存操作的配置选项
   - `GetCacheKey`: 自定义缓存键生成函数，将原始键转换为Redis中实际使用的键
   - `Ttl`: 缓存有效期设置为24小时，过期后将从数据源重新获取
   - `Fields`: 为nil表示获取哈希表的所有字段；若指定字段列表，则只获取这些字段
   - `EmptyTtl`: 空值缓存5分钟，防止缓存穿透
   - `Decoder`: 将字符串哈希表转换为结构化对象的函数
   - `LockOption`: 为nil表示不使用分布式锁

### 批量获取字符串数据 (PipeStringGetSetV2)

```go
import (
    "context"
    "time"
    
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/packer"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/cache/v3"
)

func GetUserProfiles(ctx context.Context, userIds []string) (*packer.StringRs, error) {
    rs, err := packer.PipeStringGetSetV2(ctx, a.redis, userIds, func(ctx context.Context, keys []string) (items map[string]packer.Item, err error) {
        // 从数据源查询获取数据
        // 这里应实现从数据库或其他来源获取数据的逻辑
        // 返回的数据需要实现 packer.Item 接口
        
        items = make(map[string]packer.Item)
        // 填充 items...
        
        return items, nil
    }, &packer.PipeStringGetSetOption{
        GetCacheKey: func(k string) string {
            return "user:profile:" + k
        },
        Ttl:      time.Hour * 6,
        EmptyTtl: time.Minute * 5,
    })
    
    return rs, err
}

#### 示例详解

在上面的示例中，`PipeStringGetSetV2` 函数用于批量获取字符串类型的缓存数据，具体参数说明：

1. **ctx (context.Context)**: 上下文对象，用于控制请求的生命周期和携带请求相关信息。

2. **a.redis (cache.Rds)**: Redis客户端实例，实现了 `cache.Rds` 接口。

3. **userIds ([]string)**: 需要查询的用户ID列表。

4. **回调函数**: 当缓存未命中时，用于从数据源获取数据的函数。
   ```go
   func(ctx context.Context, keys []string) (items map[string]packer.Item, err error)
   ```
   - 该函数接收未命中缓存的键列表
   - 需要返回一个映射，键为原始键，值为实现了 `packer.Item` 接口的数据对象
   - 所有返回的数据都必须实现 `CacheString()` 方法，将对象序列化为字符串
   - 返回的数据会被自动写入缓存
   
5. **PipeStringGetSetOption**: 缓存操作的配置选项
   - `GetCacheKey`: 自定义缓存键生成函数，这里为每个用户ID添加前缀 "user:profile:"
   - `Ttl`: 缓存有效期设置为6小时
   - `EmptyTtl`: 空值缓存5分钟，防止缓存穿透
   - 未设置 `LockOption`，表示不使用分布式锁

## 工作原理

Packer 包基于 Redis 实现了高效的缓存操作，主要工作流程如下：

1. **批量查询缓存**：首先使用 Redis 的 Pipeline 机制批量查询所有请求的键。

2. **区分命中与未命中**：将查询结果分为命中缓存和未命中缓存两类。

3. **数据源查询**：对于未命中的键，调用用户提供的回调函数从数据源（如数据库）批量查询数据。

4. **写入缓存**：将从数据源获取的数据写入缓存，并根据配置设置过期时间。

5. **空值处理**：对于数据源中不存在的数据，根据 `EmptyTtl` 配置缓存特殊的空值标记，防止缓存穿透。

6. **并发控制**：提供分布式锁机制，防止在高并发情况下对数据源造成过大压力。

7. **结果返回**：合并缓存命中和数据源查询的结果，返回给调用方。

### Pipeline 机制

Packer 使用 Redis 的 Pipeline 机制，将多个命令打包一次性发送到 Redis 服务器，大大减少了网络通信次数，提高了查询效率。

### 缓存穿透保护

当查询不存在的数据时，Packer 会在缓存中标记一个短期有效的空值（`_empty_`），防止后续相同的查询直接穿透到数据源。

### 分布式锁

在高并发场景下，如果大量相同的查询同时命中数据源，可能会对数据库造成冲击。Packer 提供了基于 Redis 的分布式锁机制，确保对同一组键的查询在同一时间只有一个请求能够访问数据源。

## 最佳实践

### 合理设置缓存时间

- **热点数据**：频繁访问但很少变化的数据可以设置较长的缓存时间，例如 24 小时。
- **频繁变化的数据**：经常更新的数据应该设置较短的缓存时间，例如几分钟。
- **空值缓存**：一般设置较短时间，例如 5-10 分钟，防止长时间缓存不存在的数据造成数据不一致。

### 键名设计

- 使用有意义的前缀，例如 `user:profile:`、`product:detail:`。
- 避免键名过长，浪费内存。
- 考虑使用 GetCacheKey 函数对原始键进行转换，增加灵活性。

### 实现接口

确保数据模型正确实现了 Packer 要求的接口：

```go
// 字符串缓存
type Item interface {
    CacheString() string  // 返回字符串表示，通常是 JSON 序列化的结果
}

// 哈希表缓存
type HashItem interface {
    CacheMapStringString() map[string]string  // 返回字段名到值的映射
}
```

### 处理并发场景

在高并发场景下，建议配置 LockOption 参数：

```go
LockOption: &packer.LockOption{
    LockKey: "lock:your_operation", // 锁的键名
    LockTtl: time.Second * 5,        // 锁的有效期
    LockFailedRetryInterval: time.Millisecond * 100, // 重试间隔
    MaxRetryTimes: 3,               // 最大重试次数
}
```

### 批量查询优化

- 合理控制批量查询的键的数量，避免一次查询过多键导致响应时间过长。
- 确保数据源查询函数能够高效地批量获取数据。

## 参数说明

### PipeHashGetSetOption

```go
type PipeHashGetSetOption struct {
    GetCacheKey func(k string) string
    Ttl         time.Duration
    EmptyTtl    time.Duration
    Fields      []string
    Decoder     func(map[string]string) interface{}
    LockOption  *LockOption
}
```

| 参数 | 类型 | 描述 |
|------|------|------|
| GetCacheKey | func(k string) string | 用于生成Redis缓存键名的函数，接收原始键名并返回最终使用的缓存键名 |
| Ttl | time.Duration | 缓存的有效期，表示数据在Redis中保存的时间 |
| EmptyTtl | time.Duration | 空值缓存的有效期，用于防止缓存穿透，当值大于0时会缓存空结果 |
| Fields | []string | 需要获取的哈希表字段列表，为nil时获取全部字段 |
| Decoder | func(map[string]string) interface{} | 将哈希表数据解码为结构化对象的函数 |
| LockOption | *LockOption | 分布式锁配置，用于控制并发访问 |

### LockOption

```go
type LockOption struct {
    LockKey                 string
    LockTtl                 time.Duration
    LockFailedRetryInterval time.Duration
    MaxRetryTimes           int
}
```

| 参数 | 类型 | 描述 |
|------|------|------|
| LockKey | string | 分布式锁使用的键名 |
| LockTtl | time.Duration | 锁的有效期，防止死锁 |
| LockFailedRetryInterval | time.Duration | 获取锁失败后的重试间隔，默认200ms |
| MaxRetryTimes | int | 最大重试次数，0表示一直重试 |

### PipeStringGetSetOption

```go
type PipeStringGetSetOption struct {
    GetCacheKey func(k string) string
    Ttl         time.Duration
    EmptyTtl    time.Duration
    LockOption  *LockOption
}
```

| 参数 | 类型 | 描述 |
|------|------|------|
| GetCacheKey | func(k string) string | 用于生成Redis缓存键名的函数 |
| Ttl | time.Duration | 缓存的有效期 |
| EmptyTtl | time.Duration | 空值缓存的有效期，防止缓存穿透 |
| LockOption | *LockOption | 分布式锁配置 |

## 接口实现

要使用packer包，您需要实现以下接口：

### 字符串缓存

```go
type Item interface {
    CacheString() string
}
```

### 哈希表缓存

```go
type HashItem interface {
    CacheMapStringString() map[string]string
}
```

## 缓存穿透保护

当查询不存在的数据时，packer会根据EmptyTtl参数缓存一个空值标记（"_empty_"），防止频繁查询导致的缓存穿透问题。

## 分布式锁

通过配置LockOption，可以在高并发场景下保护数据源不被大量相同请求击穿。当获取锁失败时，会根据配置进行重试。 