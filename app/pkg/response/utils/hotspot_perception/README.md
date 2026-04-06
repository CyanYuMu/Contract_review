# TinyLFU 热点探测组件

基于 Ristretto TinyLFU 算法的热点感知器，具有高性能、低内存占用的特点，适用于大规模热词发现和热点检测。

## 特性

- **高性能**: 使用 Count-Min Sketch 和 Bloom Filter 实现高效的热点检测
- **低内存占用**: 通过概率数据结构减少内存使用
- **并发安全**: 支持高并发访问
- **自动重置**: 支持基于时间和访问次数的自动重置机制
- **统计信息**: 提供详细的性能统计信息

## 核心算法

### TinyLFU (Tiny Least Frequently Used)
- 基于频率的缓存淘汰算法
- 使用 Count-Min Sketch 进行频率估计
- 使用 Bloom Filter 进行快速过滤

### Count-Min Sketch
- 概率数据结构，用于估计元素频率
- 使用多个 hash 函数减少冲突
- 提供频率的下界估计

### Bloom Filter
- 快速判断元素是否存在于集合中
- 可能存在假阳性，但不会有假阴性
- 大幅减少不必要的 Sketch 查询

## 使用方法

### 基本使用

```go
package main

import (
    "context"
    "time"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/hotspot_perception"
)

func main() {
    // 创建热点感知器
    // numCounters: 1000 (建议设为预期key数量的10倍)
    // threshold: 5 (热点阈值)
    // windowSize: 1分钟 (时间窗口)
    hotspot := hotspotperception.NewTinyLFUHotspot(1000, 5, 1*time.Minute)
    ctx := context.Background()

    // 标记访问
    hotspot.Mark(ctx, "user_123", 1)
    hotspot.Mark(ctx, "user_456", 2)

    // 检查是否为热点
    if hotspot.IsHotspot(ctx, "user_123") {
        println("user_123 是热点")
    }

    // 获取访问次数
    count := hotspot.Get(ctx, "user_456")
    println("user_456 的访问次数:", count)
}
```

### 并发使用

```go
func concurrentExample() {
    hotspot := hotspotperception.NewTinyLFUHotspot(10000, 10, 1*time.Minute)
    ctx := context.Background()

    var wg sync.WaitGroup
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            key := fmt.Sprintf("key_%d", id)
            hotspot.Mark(ctx, key, 1)
        }(i)
    }
    wg.Wait()
}
```

### 获取统计信息

```go
func statsExample() {
    hotspot := hotspotperception.NewTinyLFUHotspot(1000, 5, 1*time.Minute)
    ctx := context.Background()

    // 进行一些访问
    hotspot.Mark(ctx, "key1", 1)
    hotspot.Mark(ctx, "key2", 2)
    hotspot.Get(ctx, "key1")

    // 获取统计信息
    stats := hotspot.GetStats()
    println("总访问次数:", stats.TotalAccess)
    println("Sketch命中次数:", stats.SketchHits)
    println("Bloom命中次数:", stats.BloomHits)
}
```

## 参数说明

### NewTinyLFUHotspot 参数

- `numCounters`: sketch 计数器总数，建议设为预期 key 数量的 10 倍
- `threshold`: 热点阈值，访问次数达到此值时认为该 key 是热点
- `windowSize`: 时间窗口大小，超过此时间会触发重置

### 性能建议

- **小规模应用**: numCounters = 1000, 内存占用约 1MB
- **中规模应用**: numCounters = 10000, 内存占用约 10MB  
- **大规模应用**: numCounters = 100000, 内存占用约 100MB

## 内存优化

### 已实现的优化

1. **预分配内存**: 在创建时一次性分配所有需要的内存
2. **减少对象创建**: 避免频繁的小对象分配
3. **紧凑数据结构**: 使用更紧凑的数据结构减少内存碎片
4. **原子操作**: 使用原子操作保证并发安全，避免锁竞争

### 内存使用分析

- **Count-Min Sketch**: 占用主要内存，大小约为 `width * depth * 8` 字节
- **Bloom Filter**: 占用较少内存，大小约为 `bloomSize / 8` 字节
- **其他开销**: 统计信息、锁等，占用内存较少

## 性能特性

### 时间复杂度

- **Mark 操作**: O(depth) - 需要更新多个 hash 位置
- **Get 操作**: O(depth) - 需要查询多个 hash 位置
- **IsHotspot 操作**: O(depth) - 基于 Get 操作

### 空间复杂度

- **总内存**: O(width * depth + bloomSize)
- **每个 key**: O(1) - 固定大小的 hash 计算

### 基准测试结果

```
BenchmarkMark-8                 10020094               100.9 ns/op            55 B/op          3 allocs/op
BenchmarkGet-8                  13317622                88.25 ns/op           55 B/op          3 allocs/op
BenchmarkConcurrentMark-8        7375242               160.2 ns/op            55 B/op          3 allocs/op
```

## 适用场景

### 推荐使用场景

1. **API 热点检测**: 检测频繁访问的 API 端点
2. **用户行为分析**: 分析用户访问模式
3. **缓存优化**: 识别需要优先缓存的数据
4. **负载均衡**: 识别热点服务器或服务
5. **安全监控**: 检测异常访问模式

### 不推荐使用场景

1. **精确计数**: 需要精确访问次数统计
2. **排序需求**: 需要获取访问频率排行榜
3. **小规模应用**: 数据量很小，使用简单计数器即可

## 注意事项

### 算法特性

1. **概率性**: Count-Min Sketch 提供频率的下界估计，可能存在误差
2. **假阳性**: Bloom Filter 可能存在假阳性，但不会有假阴性
3. **重置机制**: 定期重置以保持数据的新鲜度

### 使用建议

1. **合理设置阈值**: 根据实际业务需求设置热点阈值
2. **监控统计信息**: 定期检查统计信息，了解组件运行状态
3. **内存监控**: 监控内存使用情况，避免内存泄漏
4. **性能调优**: 根据实际负载调整参数

## 故障排除

### 常见问题

1. **内存使用过高**: 检查 numCounters 参数是否设置过大
2. **热点检测不准确**: 调整 threshold 参数或检查数据分布
3. **性能下降**: 检查并发访问模式，考虑增加 numCounters

### 调试方法

1. **查看统计信息**: 使用 `GetStats()` 方法查看运行状态
2. **监控重置次数**: 检查重置频率是否合理
3. **性能分析**: 使用 Go 的性能分析工具

## 版本历史

- **v1.0.0**: 初始版本，实现基本的 TinyLFU 热点检测功能
- **v1.1.0**: 优化内存使用，减少频繁分配
- **v1.2.0**: 添加完整的单元测试和示例代码

## 贡献

欢迎提交 Issue 和 Pull Request 来改进这个组件。

## 许可证

本项目采用 MIT 许可证。 