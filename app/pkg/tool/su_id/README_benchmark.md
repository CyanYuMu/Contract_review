# SnowflakeM1 ID生成器性能测试

本目录包含了雪花算法M1方法（漂移算法）的性能测试和单元测试。

## 文件说明

- `snowflake_benchmark_test.go` - Benchmark性能测试文件
- `snowflake_test.go` - 单元测试文件
- `snowflake.go` - 原始实现文件

## 运行测试

### 1. 运行单元测试

```bash
# 运行所有单元测试
go test -v

# 运行特定测试
go test -v -run TestSnowflakeM1_BasicFunctionality
go test -v -run TestSnowflakeM1_Concurrent
go test -v -run TestSnowflakeM1_StressTest
```

### 2. 运行Benchmark测试

```bash
# 运行所有benchmark测试
go test -bench=BenchmarkSnowflakeM1 -benchmem

# 运行特定benchmark测试
go test -bench=BenchmarkSnowflakeM1_SingleThread -benchmem
go test -bench=BenchmarkSnowflakeM1_Parallel -benchmem
go test -bench=BenchmarkSnowflakeM1_HighConcurrency -benchmem

# 运行详细测试
go test -bench=BenchmarkSnowflakeM1_SingleThread -benchmem -benchtime=5s
```

### 3. 生成性能报告

```bash
# 生成CPU profile
go test -bench=BenchmarkSnowflakeM1_SingleThread -cpuprofile=cpu_m1.prof

# 生成内存 profile
go test -bench=BenchmarkSnowflakeM1_SingleThread -memprofile=mem_m1.prof

# 查看profile文件
go tool pprof cpu_m1.prof
go tool pprof mem_m1.prof
```

## 测试场景说明

### 单元测试场景

1. **基础功能测试** (`TestSnowflakeM1_BasicFunctionality`)
   - 测试ID生成的基本功能
   - 验证ID的唯一性和正数性

2. **并发测试** (`TestSnowflakeM1_Concurrent`)
   - 测试多goroutine并发生成ID
   - 验证并发环境下的ID唯一性

3. **排序测试** (`TestSnowflakeM1_Ordering`)
   - 测试ID的单调递增性
   - 验证时间戳的正确性

4. **不同Worker测试** (`TestSnowflakeM1_DifferentWorkers`)
   - 测试不同WorkerId生成的ID是否不同
   - 验证WorkerId的正确性

5. **自定义配置测试** (`TestSnowflakeM1_CustomOptions`)
   - 测试自定义配置参数是否生效
   - 验证配置的正确性

6. **性能测试** (`TestSnowflakeM1_Performance`)
   - 测试生成大量ID的性能
   - 输出性能指标

7. **压力测试** (`TestSnowflakeM1_StressTest`)
   - 测试在高负载下的性能表现
   - 验证系统的稳定性

### Benchmark测试场景

1. **单线程测试** (`BenchmarkSnowflakeM1_SingleThread`)
   - 测试单线程环境下的性能
   - 基准性能测试

2. **并发测试** (`BenchmarkSnowflakeM1_Parallel`)
   - 测试并发环境下的性能
   - 使用Go的并行测试功能

3. **高并发测试** (`BenchmarkSnowflakeM1_HighConcurrency`)
   - 测试高并发环境下的性能（100个goroutine）
   - 模拟高负载场景

4. **突发请求测试** (`BenchmarkSnowflakeM1_Burst`)
   - 测试突发请求场景下的性能
   - 连续生成大量ID

5. **不同Worker测试** (`BenchmarkSnowflakeM1_DifferentWorkers`)
   - 测试不同WorkerId的性能表现
   - 验证WorkerId对性能的影响

6. **自定义配置测试** (`BenchmarkSnowflakeM1_CustomOptions`)
   - 测试自定义配置下的性能
   - 验证配置对性能的影响

7. **连续生成测试** (`BenchmarkSnowflakeM1_ContinuousGeneration`)
   - 测试持续生成ID的性能
   - 模拟持续负载场景

8. **混合负载测试** (`BenchmarkSnowflakeM1_MixedLoad`)
   - 测试混合负载场景下的性能
   - 模拟真实业务场景

9. **压力测试** (`BenchmarkSnowflakeM1_StressTest`)
   - 测试极限压力下的性能
   - 每个周期生成10000个ID

10. **并发Worker测试** (`BenchmarkSnowflakeM1_ConcurrentWorkers`)
    - 测试并发不同Worker的性能
    - 验证多Worker并发性能

11. **基于时间测试** (`BenchmarkSnowflakeM1_TimeBasedGeneration`)
    - 测试基于时间的ID生成性能
    - 模拟时间相关的业务场景

12. **内存效率测试** (`BenchmarkSnowflakeM1_MemoryEfficiency`)
    - 测试内存分配效率
    - 验证内存使用情况

## 性能指标

### 测试结果示例

```
BenchmarkSnowflakeM1_SingleThread-8      1000000             16014 ns/op               0 B/op          0 allocs/op
BenchmarkSnowflakeM1_Parallel-8          1000000             16022 ns/op               0 B/op          0 allocs/op
BenchmarkSnowflakeM1_HighConcurrency-8   1000000             16026 ns/op               0 B/op          0 allocs/op
```

### 性能分析

1. **单次操作时间**: 约16微秒/操作
2. **内存分配**: 0字节/操作，无内存分配
3. **并发性能**: 并发环境下性能稳定
4. **扩展性**: 支持高并发场景

## M1方法特点

### 优势
- **时间回拨处理**: 能有效处理时间回拨问题
- **过载保护**: 有过载保护机制
- **稳定性**: 在各种异常情况下保持稳定
- **兼容性**: 兼容老系统的雪花算法

### 适用场景
- 高并发系统
- 时间不稳定的环境
- 需要高稳定性的业务
- 对ID唯一性要求极高的场景

## 注意事项

1. **测试环境**
   - 确保测试环境的时间同步正常
   - 避免在测试过程中调整系统时间

2. **测试结果**
   - Benchmark结果会因硬件配置而异
   - 建议多次运行取平均值

3. **依赖项**
   - 需要安装 `github.com/yitter/idgenerator-go` 包

## 添加依赖

如果缺少依赖项，请运行：

```bash
go get github.com/yitter/idgenerator-go
```
