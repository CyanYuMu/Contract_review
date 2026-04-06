# NSQ 消息队列封装

这是一个基于 [go-nsq](https://github.com/nsqio/go-nsq) 的 NSQ 消息队列封装，提供了统一的消息队列接口。

## 特性

- 统一的消息队列接口，与 Kafka、Pulsar 等保持一致
- 支持同步/异步消息发送
- 支持批量消息发送
- 支持消息消费和自动/手动 ACK
- 支持 TLS 和认证配置
- 支持连接池和重试机制
- 集成了 trace 和日志功能

## 基本用法

### 创建客户端

```go
import (
    "context"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/mq/nsq"
)

// 基本配置
config := &nsq.Config{
    NSQdAddresses: []string{"127.0.0.1:4150"},
    LookupdAddresses: []string{"127.0.0.1:4161"},
}

// 创建客户端
client := nsq.New(context.Background(), config)
```

### 生产者

```go
// 创建生产者
producer, err := client.NewProducer(ctx, mq.ProducerConf{
    Topic: "test-topic",
    TimeoutSec: 30,
})
if err != nil {
    log.Fatal(err)
}
defer producer.Close()

// 发送消息
msg := &mq.ProducerMessage{
    Payload: []byte("Hello NSQ"),
    ID:      "msg-001",
}

// 同步发送
err = producer.Send(ctx, msg)

// 异步发送
err = producer.AsyncSend(ctx, msg, func(ctx context.Context, msgId string, msg *mq.ProducerMessage, err error) {
    if err != nil {
        log.Printf("发送失败: %v", err)
    } else {
        log.Printf("发送成功: %s", msgId)
    }
})

// 批量发送
msgs := []*mq.ProducerMessage{msg1, msg2, msg3}
err = producer.BatchSend(ctx, msgs, false)
```

### 消费者

```go
// 创建消费者
consumer, err := client.NewConsumer(ctx, mq.ConsumerConf{
    Topic: "test-topic",
    Group: "test-channel", // NSQ 中称为 channel
    NumGoroutines: 4,      // 并发处理数量
    DisableAutoAck: false, // 自动 ACK
})
if err != nil {
    log.Fatal(err)
}
defer consumer.Close()

// 消费消息
err = consumer.Consume(ctx, func(ctx context.Context, msg *mq.ConsumerMessage) error {
    log.Printf("收到消息: %s", string(msg.Payload))
    
    // 处理消息逻辑
    // ...
    
    // 手动 ACK (如果 DisableAutoAck = true)
    // return msg.Ack()
    
    // 手动 NACK (重新排队)
    // return msg.Nack()
    
    return nil
})
```

## 高级配置

### TLS 配置

```go
config := &nsq.Config{
    NSQdAddresses: []string{"127.0.0.1:4150"},
    TLS: &nsq.TLSConfig{
        Enabled: true,
        InsecureSkipVerify: false,
        CertFile: "/path/to/cert.pem",
        KeyFile:  "/path/to/key.pem",
    },
}
```

### 认证配置

```go
config := &nsq.Config{
    NSQdAddresses: []string{"127.0.0.1:4150"},
    Auth: &nsq.Auth{
        Secret: "your-auth-secret",
    },
}
```

### 连接配置

```go
config := &nsq.Config{
    NSQdAddresses: []string{"127.0.0.1:4150"},
    Connection: &nsq.ConnectionConfig{
        DialTimeout:       time.Second * 5,
        ReadTimeout:       time.Second * 60,
        WriteTimeout:      time.Second * 5,
        HeartbeatInterval: time.Second * 30,
        MsgTimeout:        time.Second * 60,
        MaxAttempts:       5,
        ClientID:          "my-client",
    },
}
```

## NSQ 特性说明

### 与其他 MQ 的差异

1. **Channel 概念**: NSQ 使用 channel 作为消费者组的概念，每个 topic 可以有多个 channel
2. **无消息属性**: NSQ 不支持消息属性，如需传递 trace 信息，需要在消息体中包装
3. **直连模式**: 生产者通常直连 NSQd，消费者可以通过 NSQLookupd 发现服务
4. **无 Flush**: NSQ 没有 flush 概念，消息立即发送

### 重试机制

NSQ 内置了消息重试机制，当消息处理失败时会自动重新排队。可以通过以下方式控制：

- `MaxAttempts`: 最大尝试次数
- `DefaultRequeueDelay`: 默认重新排队延迟
- `MaxRequeueDelay`: 最大重新排队延迟

## 注意事项

1. 生产者需要直连 NSQd 地址，不能只使用 NSQLookupd
2. 消费者可以使用 NSQLookupd 进行服务发现
3. NSQ 的消息是无序的，如需保证顺序，请使用单个生产者和消费者
4. Channel 名称在 NSQ 中是持久化的，删除需要通过 NSQ 管理接口
