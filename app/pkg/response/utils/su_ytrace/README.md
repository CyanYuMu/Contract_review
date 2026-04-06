# ytrace链路追踪组件库

基于 OpenTelemetry 的分布式链路追踪库，为 Seago 项目提供完整的可观测性支持。

## 特性

- ✅ **OpenTelemetry 标准** - 使用 CNCF 标准的 OpenTelemetry
- ✅ **多 Exporter 支持** - Jaeger (本地/测试) + OTLP/Tempo (生产) + Stdout (调试)
- ✅ **零业务代码改动** - 通过结构体嵌入和 Hook 机制
- ✅ **完整组件覆盖** - MongoDB, Redis v8/v9, HTTP, Fiber, Elasticsearch, MQ, DB 接口
- ✅ **自动 Context 传播** - W3C TraceContext 标准
- ✅ **资源泄漏保护** - Finalizer 自动清理未关闭的 span
- ✅ **高性能设计** - 追踪未启用时零开销，启用时 < 5% CPU

## 架构设计

### 核心理念
- **拒绝闭包** - 使用结构体嵌入，避免闭包嵌套
- **代码生成** - 通过 tracegen 工具自动生成追踪代理
- **Hook 优先** - 能用 Hook 就用 Hook（1行代码集成）
- **性能零损失** - 纯函数调用，无额外开销

### 组件列表

| 组件          | 追踪方式         | 业务代码改动 | Pipeline 支持 |
|-------------|--------------|------------|-------------|
| firestore   | 结构体嵌入        | 1 行 | -           |
| MongoDB     | Monitor Hook | 1 行 | ✅ 自动        |
| Redis v8    | Hook         | 1 行 | ✅ 自动        |
| Redis v9    | Hook         | 1 行 | ✅ 自动        |
| ES          | 结构体嵌入        | 1 行 | -           |
| HTTP Client | RoundTripper | 1 行 | -           |
| Fiber HTTP  | Middleware   | 1 行 | -           |
| DB 接口       | 结构体嵌入        | 0 行 | -           |
| Kafka       | 消息注入         | 手动调用 | -           |
| Pub/Sub     | 消息注入         | 手动调用 | -           |

## 快速开始

### 1. 初始化追踪系统

推荐使用 `Setup` 函数，它会自动处理 cleanup：

```go
package main

import (
    "context"
    "log"
    "time"

    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_ytrace"
)

func main() {
    ctx := context.Background()

    // 从配置初始化（推荐用于生产环境）
    cfg := &su_ytrace.Config{
        Enabled:     true,
        ServiceName: "aiart-api",
        Exporter:    su_ytrace.ExporterOTLP,
        OTLP: su_ytrace.OTLPConfig{
            Endpoint: "otel-collector-gateway-collector.olte.svc.cluster.local:4317",
            Insecure: true,
            Timeout:  10 * time.Second,
        },
        Sampling: su_ytrace.SamplingConfig{
            Ratio: 0.5, // 50% 采样率
        },
    }

    // Setup 返回 cleanup 函数，使用 defer 确保资源释放
    cleanup, err := su_ytrace.Setup(ctx, "aiart-api", cfg)
    if err != nil {
        log.Printf("init tracing failed, continue without tracing: %v", err)
    } else {
        defer cleanup()
    }

    // 启动你的服务...
}
```

### 2. 完整项目接入示例（以 service 项目为例）

#### 步骤 1：添加配置结构

在 `config/config.go` 中：

```go
import "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_ytrace"

type Config struct {
    App         config.App       `yaml:"app"`
    // ... 其他配置

    // OpenTelemetry 分布式追踪配置
    OtelTracing su_ytrace.Config `yaml:"otelTracing"`
}
```

#### 步骤 2：添加 YAML 配置

在 `config/develop.yaml` 中：

```yaml
# OpenTelemetry 分布式追踪配置
otelTracing:
  enabled: true
  service_name: "aiart-api"

  # 本地开发：使用 Jaeger（可选）
  #exporter: "jaeger"
  #jaeger:
  #  endpoint: "http://localhost:14268/api/traces"

  # 测试/生产：使用 OTLP
  exporter: "otlp"
  otlp:
    endpoint: "otel-collector-gateway-collector.olte.svc.cluster.local:4317"
    insecure: true
    timeout: 10s

  sampling:
    ratio: 0.5  # 开发环境 50% 采样，生产环境建议 0.1 (10%)
```

#### 步骤 3：在主程序中初始化

在 `cmd/apiServer.go` 中：

```go
import (
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_ytrace"
)

func main() {
    ctx := context.Background()

    // 加载配置
    cfg := config.Get()

    // 初始化追踪系统
    tracingCleanup := func() {}
    if cleaner, err := su_ytrace.Setup(ctx, cfg.App.Name, &cfg.OtelTracing); err != nil {
        su_logger.Error(ctx, err, "init tracing failed, continue without tracing")
    } else {
        tracingCleanup = cleaner
    }
    defer tracingCleanup()

    // 启动服务...
}
```

#### 步骤 4：集成 Fiber 中间件

在 `internal/route/api/api.go` 中：

```go
import (
    mid "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_fiber/middleware"
)

func InitApp() *fiber.App {
    app := fiber.New(/* ... */)

    app.Use(
        mid.Cors(),
        mid.PanicRecover(),
        middleware.TokenPreprocess,
        http.Tracer(/* ... */),
        mid.OtelTracing(), // ✅ ytrace 分布式追踪-必须在 http.Tracer 之后
        // ... 其他中间件
    )

    return app
}
```

#### 步骤 5：集成各组件（在 `boot/boot.go` 中）

**MongoDB 集成**（自动追踪）：

```go
import "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/db"

// MongoDB 客户端会自动注入追踪 Monitor
mongoCli, err := db.NewMongoClient(context.Background(), mongoConf)
// ✅ 无需额外代码，自动追踪所有 MongoDB 操作
```

**Redis 集成**（1 行代码）：

```go
import "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_ytrace/instrumentation"

// Redis v8
RedisV8 = cacheV2.NewRedisV8(redisConf.ToRedisConf())
if len(redisConf.Addrs) > 0 {
    redisEndpoint := "redis://" + redisConf.Addrs[0]
    RedisV8.Client().AddHook(instrumentation.NewRedisV8Hook(redisEndpoint))
    su_logger.Info(ctx, "redis v8 tracing enabled", su_logger.E().String("endpoint", redisEndpoint))
}

// Redis v9
MemRedisTtl := cacheV3.NewRedis(memRedisConf.ToRedisConf())
if len(memRedisConf.Addrs) > 0 {
    redisEndpoint := "redis://" + memRedisConf.Addrs[0]
    MemRedisTtl.Client().AddHook(instrumentation.NewRedisV9Hook(redisEndpoint))
    su_logger.Info(ctx, "redis v9 tracing enabled", su_logger.E().String("endpoint", redisEndpoint))
}
```

**Elasticsearch 集成**（在 `lib/es/v8.go` 中）：

```go
import "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_ytrace/instrumentation"

func New(cnf Config) (es *V8, err error) {
    // 创建底层 Transport（带 TLS 配置）
    baseTransport := &http.Transport{
        TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
    }

    // ✅ 用追踪 Transport 包装底层 Transport
    tracedTransport := instrumentation.NewElasticsearchTransport(baseTransport, cnf.Addresses)

    // 创建 ES 客户端
    client, err := elasticsearch.NewClient(elasticsearch.Config{
        Addresses: cnf.Addresses,
        Transport: tracedTransport,
        // ...
    })
    // ...
}
```

**Firestore 集成**（0 行业务代码）：

```go
import "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/db"

// 在 dao 层包装一次
func FirestoreDB() db.DB {
    return singleFac.Get("firestoreDB", func() interface{} {
        rawDB := boot.Firestore
        // ✅ 包装为追踪代理
        return db.NewTracedDB(rawDB, "firestore")
    }).(db.DB)
}

// 业务代码完全不变
doc, err := dao.FirestoreDB().Get(ctx, "users", "123")
```

**MQ 集成**（在 `boot/boot.go` 中）：

```go
import "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_ytrace/instrumentation"

func MQ() mq.MQ {
    return singleFac.Get("pubsub", func() interface{} {
        su_logger.Info(glbCtx, "init pubsub mq")

        cnf := config.Get().PubSub
        mq, err := pubsub.New(glbCtx, pubsub.PubSubConfig{
            CredentialsJson: cnf.CredentialsJson,
            ProjectId:       cnf.ProjectId,
            Options:         cnf.Options,
        })

        if err != nil {
            panic(err)
        }

        // ✅ 包装为追踪代理
        return instrumentation.WrapMQ(mq, "pubsub")
    }).(mq.MQ)
}
```

**HTTP Client 集成**（自动追踪）：

```go
import "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/http/req"

// ✅ req.New() 已自动启用追踪，无需额外代码
client := req.New(ctx)
resp, err := client.Get(url)

// ✅ req.GetHostClient()已自动启用追踪，无需额外代码
req.GetHostClient()
```

### 3. 环境配置对比

| 环境 | Exporter | 采样率 | 用途 |
|------|---------|--------|------|
| **本地开发** | `stdout` 或 `jaeger` | 100% | 调试追踪数据 |
| **测试环境** | `otlp` | 50-100% | 功能验证 |
| **生产环境** | `otlp` | 10-20% | 性能监控 |

**本地开发配置示例**：

```yaml
otelTracing:
  enabled: true
  service_name: "aiart-api"
  exporter: "stdout"  # 输出到控制台，便于调试
  sampling:
    ratio: 1.0  # 100% 采样
```

**生产环境配置示例**：

```yaml
otelTracing:
  enabled: true
  service_name: "aiart-api"
  exporter: "otlp"
  otlp:
    endpoint: "otel-collector-gateway-collector.olte.svc.cluster.local:4317"
    insecure: true  # K8s 内部通信
    timeout: 10s
  sampling:
    ratio: 0.1  # 10% 采样，降低性能开销
```

## Worker 服务接入

Worker 服务（消息消费者、定时任务）需要手动创建 root span：

```go
import (
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_ytrace"
    "go.opentelemetry.io/otel/attribute"
)

// 消息处理入口
func (w *Worker) ProcessMessage(ctx context.Context, msg *kafka.Message) error {
    // ✅ 为每个消息创建独立的 root span
    ctx, cleanup := su_ytrace.NewRequestContext(ctx, "worker.process_message",
        attribute.String("message_id", msg.ID),
        attribute.String("topic", msg.Topic),
    )
    defer cleanup()  // ⚠️ 必须 defer，否则会资源泄漏

    // 处理消息...
    // 所有下游操作（DB、Redis、HTTP）会自动成为子 span

    return nil
}

// 定时任务入口
func (c *Cron) SyncData(ctx context.Context) error {
    ctx, cleanup := su_ytrace.NewRequestContext(ctx, "cron.sync_data",
        attribute.String("job_name", "sync_user_data"),
    )
    defer cleanup()

    // 执行任务...

    return nil
}
```

**⚠️ 重要**：
- 必须使用 `defer cleanup()`，否则 span 不会结束
- 如果忘记调用 cleanup，Finalizer 会在 GC 时自动清理并记录警告日志

## 本地开发调试

### 方式 1：使用 Stdout（最简单）

```yaml
otelTracing:
  enabled: true
  service_name: "aiart-api"
  exporter: "stdout"
  sampling:
    ratio: 1.0
```

运行程序后，追踪数据会直接输出到控制台（JSON 格式）。

### 方式 2：使用本地 Jaeger

1. **启动 Jaeger**：

```bash
docker run -d --name jaeger \
  -p 16686:16686 \
  -p 14268:14268 \
  jaegertracing/all-in-one:latest
```

2. **配置**：

```yaml
otelTracing:
  enabled: true
  service_name: "aiart-api"
  exporter: "jaeger"
  jaeger:
    endpoint: "http://localhost:14268/api/traces"
  sampling:
    ratio: 1.0
```

3. **访问 UI**：http://localhost:16686

## 故障排查

### 检查追踪是否启用

```go
import "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_ytrace"

if su_ytrace.IsEnabled() {
    fmt.Println("✅ 追踪已启用")
} else {
    fmt.Println("❌ 追踪未启用")
}
```

### 查看 Span 泄漏日志

如果忘记调用 `cleanup()`，会在日志中看到：

```
[WARN] ytrace: span leaked and cleaned by finalizer | operation=worker.process_message request_id=xxx hint=forgot to call defer cleanup()?
```

在 Grafana Tempo 中搜索 `span.leaked=true` 可以找到所有泄漏的 span。

### 验证 Context 传播

```go
import "go.opentelemetry.io/otel/trace"

// 在 Handler 中检查 span
span := trace.SpanFromContext(c.UserContext())
if span.IsRecording() {
    fmt.Println("✅ Span 正在记录")
    fmt.Printf("TraceID: %s\n", span.SpanContext().TraceID())
}
```

## FAQ

### Q: 如何选择初始化方式？

- **生产环境**：使用 `Setup(ctx, serviceName, &cfg.OtelTracing)`，集成到业务配置
- **开发测试**：使用 `SetupWithEnvironment(ctx, serviceName, "local")`，快速启动

### Q: 多个服务如何使用不同的 service_name？

```go
// API Server
cleanup, _ := su_ytrace.Setup(ctx, "aiart-api", cfg)

// Worker Server
cleanup, _ := su_ytrace.Setup(ctx, "aiart-worker", cfg)

// Event Bus Server
cleanup, _ := su_ytrace.Setup(ctx, "aiart-eventbus", cfg)
```

### Q: Redis Pipeline 需要额外配置吗？

不需要！Redis v8/v9 的 Hook 会自动追踪 Pipeline 操作。

### Q: 忘记调用 cleanup() 会怎样？

Finalizer 会在 GC 时自动清理 span，并记录警告日志。但建议始终使用 `defer cleanup()`。

### Q: 性能影响有多大？

- **追踪未启用**：零开销
- **追踪已启用**：< 5% CPU
- **采样率 10%**：对 QPS 影响 < 1%

### 其他任何问题找 也书
