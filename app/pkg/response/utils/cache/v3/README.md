# Cache 缓存中间件使用手册

## 概述

`cache/v3`包提供了一套完整的缓存解决方案，支持Redis V8/V9版本、内存缓存(GoCache)以及二级缓存实现。该包封装了常用的缓存操作，提供了统一接口使用不同版本的Redis客户端和内存缓存，并支持批量操作、管道操作和分布式锁等高级功能。

## 主要功能

1. **Redis客户端封装**
   - 支持Redis V8和V9版本
   - 支持单机模式和集群模式
   - 提供统一的接口

2. **内存缓存(GoCache)**
   - 轻量级内存缓存实现
   - 支持过期时间设置
   - 适合单机应用的高速缓存

3. **二级缓存(L2Cache)**
   - 一级缓存(内存)与二级缓存(Redis)联动
   - 自动数据同步与过期策略
   - 支持自定义数据转换

4. **常用Redis数据类型操作**
   - 字符串(String)：Set、Get、Del等
   - 哈希(Hash)：HSet、HGet、HMSet等
   - 列表(List)：LPush、RPush、LPop等
   - 集合(Set)：SAdd、SRem、SMembers等
   - 有序集合(ZSet)：ZAdd、ZRange等

5. **高级功能**
   - 管道操作(Pipeline)
   - 分布式锁(Lock/Unlock)
   - 发布订阅(Publish/Subscribe)

## 使用示例

### 基本配置与初始化

#### Redis缓存

```go
import (
    "context"
    "time"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/cache/v3"
)

func main() {
    // Redis配置
    redisConf := &cache.RedisConfig{
        Addrs:         []string{"127.0.0.1:6379"},
        Password:      "密码",
        DB:            0,
        PoolSize:      10,
        MinIdleConn:   5,
        Prefix:        "app",
        DialTimeoutSec: 5,
        ReadTimeoutSec: 3,
        WriteTimeoutSec: 3,
    }
    
    // 初始化Redis客户端(V9版本)
    redisClient := cache.NewRedisV9(redisConf)
    defer redisClient.Close()
    
    // 使用Redis客户端
    ctx := context.Background()
    redisClient.Set(ctx, "key", "value", time.Minute)
}
```

#### 内存缓存(GoCache)

```go
import (
    "context"
    "time"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/cache/v3"
)

func main() {
    // GoCache配置
    goCacheConf := &cache.GoCacheConfig{
        DefaultTtlSec:     300,  // 默认过期时间为300秒
        CleanIntervalSec:  60,   // 每60秒清理一次过期键
        ShardNum:          16,   // 使用16个分片减少锁竞争
    }
    
    // 初始化内存缓存
    memCache := cache.NewGoCache(goCacheConf)
    
    // 使用内存缓存
    ctx := context.Background()
    memCache.Set(ctx, "key", "value", time.Minute)
}
```

### 字符串操作

```go
// 设置字符串，带过期时间
ok, err := redisClient.Set(ctx, "user:name", "张三", time.Hour)

// 获取字符串
name, err := redisClient.GetString(ctx, "user:name")

// 设置并获取旧值
oldValue, err := redisClient.GetSet(ctx, "counter", 100)

// 删除键
success, err := redisClient.Del(ctx, "user:name")

// 检查键是否存在
exists, err := redisClient.Exists(ctx, "user:name")

// 设置过期时间
success, err := redisClient.Expire(ctx, "user:name", time.Hour)
```

### 内存缓存操作

```go
// 设置值
ok, err := memCache.Set(ctx, "counter", 100, time.Minute)

// 获取值
val, err := memCache.Get(ctx, "counter")

// 删除键
ok, err := memCache.Del(ctx, "counter")

// 键是否存在
exists, err := memCache.Exists(ctx, "counter")

// 设置过期时间
ok, err := memCache.Expire(ctx, "counter", time.Minute*5)

// 递增操作
newVal, err := memCache.Incr(ctx, "counter")

// 增加指定值
newVal, err := memCache.IncrBy(ctx, "counter", 10)

// 递减操作
newVal, err := memCache.Decr(ctx, "counter")

// 减少指定值
newVal, err := memCache.DecrBy(ctx, "counter", 5)

// 获取过期时间
ttl, err := memCache.TTL(ctx, "counter")
```

### 哈希表操作

```go
// 设置单个哈希字段
ok, err := redisClient.HSet(ctx, "user:1", "name", "张三")

// 获取单个字段
name, err := redisClient.HGet(ctx, "user:1", "name")

// 批量设置哈希字段
userInfo := map[string]string{
    "name": "张三",
    "age": "25",
    "city": "北京",
}
ok, err := redisClient.HMSet(ctx, "user:1", userInfo)

// 获取所有字段
allFields, err := redisClient.HGetAll(ctx, "user:1")

// 获取多个字段
fields := []string{"name", "age"}
values, err := redisClient.HMGet(ctx, "user:1", fields)

// 删除字段
count, err := redisClient.HDel(ctx, "user:1", "city")

// 字段自增
newVal, err := redisClient.HIncrBy(ctx, "user:stats", "visits", 1)
```

### 列表操作

```go
// 左侧推入列表
values := []interface{}{"item1", "item2"}
length, err := redisClient.LPush(ctx, "queue", values)

// 右侧推入列表
length, err = redisClient.RPush(ctx, "queue", values)

// 左侧弹出元素
item, err := redisClient.LPop(ctx, "queue")

// 右侧弹出元素
item, err = redisClient.RPop(ctx, "queue")

// 批量左侧弹出
items, err := redisClient.LPopCount(ctx, "queue", 5)

// 获取列表长度
length, err = redisClient.LLen(ctx, "queue")

// 获取列表范围
items, err = redisClient.LRange(ctx, "queue", 0, -1) // 获取所有元素
```

### 集合操作

```go
// 添加集合成员
members := []interface{}{"member1", "member2"}
added, err := redisClient.SAdd(ctx, "myset", members)

// 移除集合成员
removed, err := redisClient.SRem(ctx, "myset", members)

// 获取所有成员
allMembers, err := redisClient.SMembers(ctx, "myset")

// 获取集合大小
size, err := redisClient.SCard(ctx, "myset")
```

### 有序集合操作

```go
// 添加一个成员
added, err := redisClient.ZAdd(ctx, "leaderboard", cache.Z{Score: 100, Member: "user1"})

// 添加多个成员
members := []cache.Z{
    {Score: 90, Member: "user2"},
    {Score: 85, Member: "user3"},
}
added, err = redisClient.ZMAdd(ctx, "leaderboard", members)

// 按分数范围获取成员
users, err := redisClient.ZRangeByScore(ctx, "leaderboard", 80, 100)

// 获取排名前N的成员
topUsers, err := redisClient.ZRevRange(ctx, "leaderboard", 0, 9) // 前10名

// 带分数获取成员
usersWithScores, err := redisClient.ZRevRangeWithScores(ctx, "leaderboard", 0, 9)

// 获取成员分数
score, err := redisClient.ZScore(ctx, "leaderboard", "user1")

// 移除成员
removed, err := redisClient.ZRem(ctx, "leaderboard", "user1")

// 获取集合大小
size, err := redisClient.ZCard(ctx, "leaderboard")
```

### 管道操作

```go
// 创建管道
pipe := redisClient.Pipeline()

// 添加命令到管道
stringCmd := pipe.Get(ctx, "key1")
intCmd := pipe.ZCard(ctx, "zset1")
boolCmd := pipe.Exists(ctx, "key1")

// 执行管道命令
cmds, err := pipe.Exec(ctx)
if err != nil {
    // 处理错误
}

// 获取结果
val1, err := stringCmd.Result()
count, err := intCmd.Result()
exists, err := boolCmd.Result()
```

### 分布式锁

```go
// 获取锁
lockKey := "resource_lock"
randomValue := "随机值-1234567890" // 用于解锁验证
locked, err := redisClient.Lock(ctx, lockKey, randomValue, time.Second*30)
if locked {
    // 执行受保护的代码
    // ...
    
    // 释放锁
    unlocked, err := redisClient.Unlock(ctx, lockKey, randomValue)
}
```

### 二级缓存(L2Cache)使用

```go
import (
    "context"
    "time"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/cache/v3"
)

func main() {
    // 初始化Redis客户端作为二级缓存
    redisClient := cache.NewRedisV9(redisConf)
    
    // 初始化内存缓存作为一级缓存
    goCacheConf := &cache.GoCacheConfig{
        DefaultTtlSec:     300,
        CleanIntervalSec:  60,
    }
    memoryCache := cache.NewGoCache(goCacheConf)
    
    // 创建二级缓存
    l2Cache := cache.NewL2(memoryCache, redisClient, nil)
    
    ctx := context.Background()
    
    // 使用L2Cache.GetSet获取数据，自动处理缓存
    result := l2Cache.GetSet(ctx, "user:profile:123", 
        // 源数据获取函数(当缓存不存在时调用)
        func(ctx context.Context) (interface{}, error) {
            // 从数据库或其他源获取数据
            return getUserFromDB(ctx, 123)
        }, 
        // 可选配置
        &cache.L2GetSetOptions{
            L1Ttl: time.Minute * 5,  // 一级缓存过期时间
            L2Ttl: time.Hour,        // 二级缓存过期时间
        })
    
    // 处理结果
    if result.Err != nil {
        // 处理错误
    }
    
    // 解码结果到用户结构体
    var user User
    err := result.Decode(&user)
}
```

## 配置参数说明

### RedisConfig 参数

```go
type RedisConfig struct {
    Addrs              []string   // Redis服务器地址列表
    Password           string     // Redis密码
    DB                 int        // 数据库索引，默认为0
    PoolSize           int        // 连接池大小
    MinIdleConn        int        // 最小空闲连接数
    DialTimeoutSec     int        // 连接超时秒数
    ReadTimeoutSec     int        // 读取超时秒数
    WriteTimeoutSec    int        // 写入超时秒数
    PoolTimeoutSec     int        // 池超时秒数
    IdleTimeoutSec     int        // 空闲连接超时秒数
    IdleCheckFrequencySec int     // 空闲检查频率秒数
    MaxConnAgeSec      int        // 连接最大存活时间秒数
    Prefix             string     // 键前缀
    IsCluster          bool       // 是否集群模式
    ReadOnly           bool       // 是否只读模式(集群模式下有效)
    RouteRandomly      bool       // 是否随机路由(集群模式下有效)
    PoolFIFO           bool       // 连接池FIFO模式
    Cli                *Client    // 预设客户端(可选)
}
```

### GoCacheConfig 参数

```go
type GoCacheConfig struct {
    DefaultTtlSec     int   `yaml:"defaultTtlSec"`  // 默认的缓存过期时间(秒)
    CleanIntervalSec  int   `yaml:"cleanIntervalSec"` // 清理过期项目的时间间隔(秒)
    ShardNum          int   `yaml:"shardNum"` // 分片数量，用于减少并发锁竞争
}
```

### L2GetSetOptions 参数

```go
type L2GetSetOptions struct {
    L1Ttl          time.Duration  // 一级缓存TTL，默认5分钟
    L2Ttl          time.Duration  // 二级缓存TTL，默认10分钟
    L1EmptyTtl     time.Duration  // 一级缓存空值TTL，默认1分钟
    L2EmptyTtl     time.Duration  // 二级缓存空值TTL，默认5分钟
    L1Setter       func(...)      // 自定义一级缓存设置函数
    L1Getter       func(...)      // 自定义一级缓存获取函数
    L2Getter       func(...)      // 自定义二级缓存获取函数
    L2Setter       func(...)      // 自定义二级缓存设置函数
    L2Transformer  func(...)      // 二级缓存数据到一级缓存的转换函数
}
```

## 缓存选择指南

### 何时使用内存缓存(GoCache)
- 单机应用，不需要跨服务器共享缓存数据
- 对性能要求极高，需要极低的访问延迟
- 缓存数据量不大，内存资源充足
- 适用场景：频繁读取的配置信息、会话数据、临时计算结果等

### 何时使用Redis缓存
- 分布式应用，需要在多个服务器间共享数据
- 需要使用高级数据结构(Hash、List、Set、ZSet等)
- 需要持久化缓存数据
- 适用场景：用户会话、排行榜、计数器、分布式锁等

### 何时使用二级缓存(L2Cache)
- 既需要高性能又需要数据共享的场景
- 读多写少的数据
- 需要在性能和一致性之间取得平衡
- 适用场景：用户信息、商品信息、配置数据等

## 最佳实践

1. **使用前缀隔离不同应用**
   ```go
   redisConf.Prefix = "myapp"  // 所有键将自动添加"myapp:"前缀
   ```

2. **合理设置过期时间**
   ```go
   // 热点数据短期缓存
   redisClient.Set(ctx, "hot:data", value, time.Minute*5)
   
   // 稳定数据长期缓存
   redisClient.Set(ctx, "stable:data", value, time.Hour*24)
   ```

3. **针对不同场景选择合适的缓存类型**
   ```go
   // 本地缓存：高频访问的小型数据
   memCache.Set(ctx, "local:config", configData, time.Hour)
   
   // Redis缓存：需要共享的数据
   redisClient.Set(ctx, "shared:config", configData, time.Hour)
   ```

4. **L2Cache中配置不同级别的过期时间**
   ```go
   options := &cache.L2GetSetOptions{
       L1Ttl: time.Minute*5,    // 一级缓存(内存)短期保存
       L2Ttl: time.Hour,        // 二级缓存(Redis)长期保存
   }
   ```

5. **使用管道提高批量操作性能**
   ```go
   pipe := redisClient.Pipeline()
   for i := 0; i < 100; i++ {
       pipe.Set(ctx, fmt.Sprintf("key:%d", i), i, time.Hour)
   }
   _, err := pipe.Exec(ctx)
   ```

6. **正确处理分布式锁**
   ```go
   // 使用唯一标识解锁，避免误解他人的锁
   randomValue := generateUniqueID()
   locked, _ := redisClient.Lock(ctx, "lock:key", randomValue, time.Minute)
   // 操作完成后释放锁
   redisClient.Unlock(ctx, "lock:key", randomValue)
   ```

7. **内存缓存的分片配置**
   ```go
   // 高并发环境，增加分片数减少锁争用
   goCacheConf := &cache.GoCacheConfig{
       DefaultTtlSec: 300,
       ShardNum: 32,  // 32个分片减少锁竞争
   }
   ```

## 错误处理

正确检查每个缓存操作的错误返回：

```go
val, err := redisClient.Get(ctx, "key")
if err != nil {
    if cache.IsNotFoundError(err) {
        // 键不存在处理
    } else {
        // 其他错误处理
    }
}
```

## 注意事项

1. 在高并发场景下，合理设置连接池大小(`PoolSize`)和最小空闲连接数(`MinIdleConn`)
2. 使用内存缓存时，注意控制缓存大小，避免内存溢出
3. 使用二级缓存时，合理设置一级和二级缓存的过期时间，避免缓存雪崩
4. 使用分布式锁时，设置合理的锁超时时间，并确保操作完成后释放锁
5. 对于频繁修改的数据，考虑较短的缓存时间或使用更新模式而非失效模式
6. 对于可能不存在的键，处理缓存穿透问题(如使用空值缓存)
7. 在多服务器环境中，使用GoCache需要意识到数据一致性问题，每个服务实例的缓存是独立的
