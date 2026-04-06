# 使用

## Step.1 配置

config/config.yaml 增加配置, 格式如下:

```yaml
kafka:
  # 节点
  broker:
    - "kafka1-svc.kafka.svc.cluster.local:9092"
    - "kafka2-svc.kafka.svc.cluster.local:9092"
    - "kafka3-svc.kafka.svc.cluster.local:9092"
  # 单位秒
  timeout: 2
```

## Step.2 初始化队列

boot/boot.go 追加以下代码进行初始化

```go
// 将此变量设置为全局变量
var MQ *kafka.Kafka
conf := config.GetConf().Kafka
kafkaConf := kafka.Config{
    Timeout: time.Duration(conf.Timeout) * time.Second,
    Level:   kafka.MsgLevelHigh,
}
kafka.InitDelayProducer(conf.Broker)
MQ = kafka.New(conf.Broker, kafkaConf)
```

`MQ` 这个变量需要设置为全局变量


## Step.3 消息投递到延迟队列

### 消息投递

1. 实例化生产者, 并同步投递消息

```go
func TestProducer(t *testing.T) {
	p, err := boot.MQ.NewProducer()
	if err != nil {
		t.Error(err)
		return
	}
	p.Send(&sarama.ProducerMessage{
		Topic: "test",
		Value: sarama.StringEncoder("test"),
	})
}
```

2. 实例化生产者, 并异步投递消息

```go
func TestAsyncProducer(t *testing.T) {
	p, err := boot.MQ.NewAsyncProducer(&kafka.AsyncHandlerConfig{
		Success: func(msg sarama.ProducerMessage) {
			fmt.Println("处理成功消息回调")
		},
		Error: func(msg sarama.ProducerError) {
			fmt.Println("处理失败消息回调")
		},
	})
	
	if err != nil {
		t.Error(err)
		return
	}
	p.Send(&sarama.ProducerMessage{
		Topic: "test",
		Value: sarama.StringEncoder("test"),
	})
}
```

3. 实例化生产者, 并投递延迟消息(仅支持同步)

投递参数说明

```go
type DelayProducerMsg struct {
	// 必填, 时间到期后, 将消息投递的目的topic, 这里需业务方确保topic已存在
	Topic string // The Kafka topic for this message.
	// 非必填, 消息id, 保证全局唯一, 为空会使用uuid.v4 生成
	Key string
	// 必填, 消息内容
	Value []byte
	// 非必填, 头部消息
	Headers []sarama.RecordHeader
	// 非必填, 元数据
	Metadata interface{}
	// 非必填,  指定到期后投递到的分区
	Partition int32
}
```

```go
func TestProducer(t *testing.T) {
	producer, err := boot.MQ.NewProducer()
	if err != nil {
		t.Error(err)
		return
	}

    var data []byte
    for i := 0; i < 10; i++ {
		h := map[string]string{"a": "ha", "b": "hb", "c": "hc"}
		data, _ = jsoniter.Marshal(h)
		pData := &kafka.DelayProducerMsg{
			Value: data,
			Topic: "test",
		}
		err = producer.DelaySend(pData, time.Second)
		if err != nil {
			t.Error(err)
			return
		}
	}
}
```

# Kafka 证书配置指南

本文档介绍如何在 Seago Kafka 客户端中配置不同格式的证书。

## 🎯 简化配置 (推荐)

现在支持**自动格式识别**，大大简化了证书配置！

### ✅ 支持的证书格式

- ✅ **PEM 格式** (.pem 文件)
- ✅ **CRT 格式** (.crt, .cer, .der 文件)  
- ✅ **PEM 字符串** (直接传入证书内容)
- ✅ **自动识别** (根据内容和文件后缀自动判断)

## 🚀 简化配置方式

### 1. 使用文件路径 (自动识别格式)

```go
config := &kafka.Config{
    Brokers: []string{"localhost:9093"},
    Cert: &kafka.Cert{
        CACert:      "/path/to/ca.pem",        // 或 ca.crt
        ClientCert:  "/path/to/client.pem",    // 或 client.crt
        ClientKey:   "/path/to/client-key.pem", // 或 client.key
        KeyPassword: "your-key-password",
        Protocol:    "SSL",
    },
}
```

### 2. 使用 PEM 字符串内容

```go
caCertPEM := `-----BEGIN CERTIFICATE-----
MIIDQTCCAimgAwIBAgITBmyfz5m/jAo54vB4ikPmljZbyjANBgkqhkiG9w0BAQsF
...
-----END CERTIFICATE-----`

config := &kafka.Config{
    Brokers: []string{"localhost:9093"},
    Cert: &kafka.Cert{
        CACert:      caCertPEM,               // 自动识别为PEM内容
        ClientCert:  clientCertPEM,           // 自动识别为PEM内容
        ClientKey:   clientKeyPEM,            // 自动识别为PEM内容
        KeyPassword: "your-key-password",
        Protocol:    "SSL",
    },
}
```

### 3. 混合使用

```go
config := &kafka.Config{
    Brokers: []string{"localhost:9093"},
    Cert: &kafka.Cert{
        CACert:     "/path/to/ca.crt",        // 文件路径
        ClientCert: clientCertPEMString,      // PEM字符串
        ClientKey:  "/path/to/client.key",    // 文件路径
        Protocol:   "SSL",
    },
}
```

### 4. SASL + SSL 认证

```go
config := &kafka.Config{
    Brokers: []string{"localhost:9093"},
    Cert: &kafka.Cert{
        CACert:    "/path/to/ca.crt",
        Username:  "kafka-user",
        Password:  "kafka-password", 
        Protocol:  "SASL_SSL",
        Mechanism: "PLAIN",
    },
}
```

## 🔍 自动识别规则

系统会按以下规则自动识别证书格式：

### 1. **PEM 内容识别**
```go
// 包含 PEM 标识符 -> 识别为 PEM 内容
content := `-----BEGIN CERTIFICATE-----
...
-----END CERTIFICATE-----`
```

### 2. **文件路径识别**
```go
// 根据文件扩展名识别
"/path/to/ca.pem"     -> PEM 文件
"/path/to/ca.crt"     -> CRT 文件  
"/path/to/ca.cer"     -> CER 文件
"/path/to/ca.der"     -> DER 文件
"/path/to/client.key" -> KEY 文件
```

### 3. **配置优先级**
1. **xxxPEM 字段** (最高优先级，明确指定PEM内容)
2. **简化字段** (CACert, ClientCert, ClientKey - 自动识别)
3. **兼容字段** (CertFile - 向后兼容)

## 📋 完整配置示例

```go
package main

import (
    "context"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/mq/kafka"
    "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/mq"
)

func main() {
    // 🎯 简化配置 - 自动识别格式
    config := &kafka.Config{
        Brokers: []string{"broker1:9093", "broker2:9093"},
        Cert: &kafka.Cert{
            // 支持 .pem/.crt/.cer/.der 等格式，自动识别
            CACert:     "/etc/ssl/certs/ca.pem",
            ClientCert: "/etc/ssl/certs/client.crt", 
            ClientKey:  "/etc/ssl/private/client.key",
            KeyPassword: "your-private-key-password",
            
            // SSL 配置
            Protocol:  "SSL",
            Algorithm: "https", // 启用主机名验证
        },
    }
    
    // 创建客户端
    client := kafka.New(context.Background(), config)
    
    // 创建生产者
    producer, err := client.NewProducer(context.Background(), mq.ProducerConf{
        Topic: "my-topic",
    })
    if err != nil {
        panic(err)
    }
    defer producer.Close()
    
    // 创建消费者
    consumer, err := client.NewConsumer(context.Background(), mq.ConsumerConf{
        Topic: "my-topic",
        Group: "my-group",
    })
    if err != nil {
        panic(err)
    }
    defer consumer.Close()
}
```

## 🔧 高级配置 (如需精确控制)

如果需要精确控制证书格式，仍可使用专门的PEM字段：

```go
config := &kafka.Config{
    Cert: &kafka.Cert{
        // 明确指定为PEM内容 (最高优先级)
        CACertPEM:     pemCACertString,
        ClientCertPEM: pemClientCertString,
        ClientKeyPEM:  pemClientKeyString,
        
        // 简化配置 (自动识别，次优先级)
        CACert:     "/backup/ca.crt",
        ClientCert: "/backup/client.crt",
        ClientKey:  "/backup/client.key",
    },
}
```

## ⚡ 配置对比

### 🔴 旧版本配置 (复杂)
```go
Cert: &kafka.Cert{
    CAFile:         "/path/to/ca.pem",
    ClientCertFile: "/path/to/client.pem", 
    ClientKeyFile:  "/path/to/client-key.pem",
    CACertPEM:      pemString1,
    ClientCertPEM:  pemString2,
    ClientKeyPEM:   pemString3,
}
```

### 🟢 新版本配置 (简化)
```go
Cert: &kafka.Cert{
    CACert:     "/path/to/ca.pem",     // 自动识别
    ClientCert: "/path/to/client.crt", // 自动识别
    ClientKey:  pemKeyString,          // 自动识别
}
```

## 🛠️ 证书格式转换

如果需要在不同格式之间转换：

### CRT 转 PEM
```bash
# 证书转换
openssl x509 -in certificate.crt -out certificate.pem -outform PEM

# 私钥转换  
openssl rsa -in private.key -out private.pem -outform PEM
```

### PEM 转 CRT
```bash
# 证书转换
openssl x509 -in certificate.pem -out certificate.crt -outform DER

# 私钥转换
openssl rsa -in private.pem -out private.key -outform DER
```

## ⚠️ 注意事项

1. **文件权限**：确保证书文件具有适当的权限 (通常是 600 或 400)
2. **密码保护**：建议对私钥使用密码保护
3. **证书链**：如果使用中间CA，确保证书链完整
4. **过期时间**：定期检查证书过期时间
5. **主机名验证**：生产环境建议启用主机名验证 (`Algorithm: "https"`)
6. **自动识别**：系统会自动识别格式，但如有疑问可使用专门的PEM字段

## 🔍 故障排除

### 常见错误

1. **格式识别错误**
   ```
   Error: unable to load certificate
   ```
   解决：检查文件路径是否正确，或使用专门的 `xxxPEM` 字段明确指定

2. **私钥密码错误**
   ```
   Error: bad decrypt
   ```
   解决：检查 `KeyPassword` 是否正确

3. **证书链不完整**
   ```
   Error: certificate verify failed
   ```
   解决：确保提供完整的证书链，包括中间CA证书

### 调试方法

启用 SSL 调试日志：
```bash
export KAFKA_OPTS=-Djavax.net.debug=ssl
```

测试连接：
```bash
openssl s_client -connect broker:9093 -cert client.pem -key client-key.pem
```



