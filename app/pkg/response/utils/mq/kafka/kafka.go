package kafka

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/panjf2000/ants/v2"
	"github.com/spf13/cast"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/enum"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/su_math"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/mq"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/trace"
)

type Client struct {
	ctx    context.Context
	Config *Config
}

func New(ctx context.Context, cnf *Config) (cli *Client) {

	cli = &Client{
		ctx:    ctx,
		Config: cnf,
	}

	return
}

// Consumer Kafka消费者实现
type Consumer struct {
	inst           *kafka.Consumer
	topic          string
	topics         []string
	group          string
	autoAck        bool
	retryPolicy    *mq.RetryPolicy
	immediatelyAck bool
	running        bool
	stopChan       chan struct{}
	config         *kafka.ConfigMap
	readTimeout    time.Duration
}

// Close 关闭消费者
func (c *Consumer) Close() error {
	if c.running {
		close(c.stopChan)
		c.running = false
	}
	if c.inst != nil {
		return c.inst.Close()
	}
	return nil
}

// 将kafka.Message转换为mq.ConsumerMessage
func toConsumerMessage(consumer *kafka.Consumer, msg *kafka.Message) (context.Context, *mq.ConsumerMessage) {
	if msg == nil {
		return nil, nil
	}

	// 提取消息属性
	attributes := make(map[string]string)
	ctx := context.Background()
	if msg.Headers != nil {
		for _, header := range msg.Headers {
			attributes[header.Key] = string(header.Value)
			if header.Key == enum.CtxRequestId {
				ctx = context.WithValue(ctx, enum.CtxRequestId, cast.ToString(header.Value))
			} else if header.Key == enum.CtxSpanId {
				ctx = context.WithValue(ctx, enum.CtxSpanId, cast.ToString(header.Value))
			} else if header.Key == enum.CtxTrackingParams {
				ctx = context.WithValue(ctx, enum.CtxTrackingParams, cast.ToString(header.Value))
			} else if header.Key == enum.CtxDebug {
				ctx = context.WithValue(ctx, enum.CtxDebug, cast.ToString(header.Value))
			}
		}
	}

	// 提取消息ID
	id := ""
	if msg.Key != nil {
		id = string(msg.Key)
	}

	return ctx, &mq.ConsumerMessage{
		Payload:    msg.Value,
		ID:         id,
		Attributes: attributes,
		EventTime:  msg.Timestamp,
		// Kafka消息没有DeliveryAttempt字段，设为nil
		DeliveryAttempt: nil,
		// 实现Ack和Nack函数
		Ack: func() error {
			// Kafka的CommitMessage方法用于手动提交消息
			// 注意：如果启用了自动提交，这个操作可能是多余的
			if consumer == nil {
				return nil
			}
			_, err := consumer.CommitMessage(msg)
			return err
		},
		Nack: func() error {
			// Kafka没有原生的Nack操作，这里不做任何处理
			return nil
		},
	}
}

// Consume 实现消费消息的方法
func (c *Consumer) Consume(ctx context.Context, handler mq.MsgHandler) error {
	if c.running {
		return fmt.Errorf("consumer is already running")
	}

	c.running = true
	c.stopChan = make(chan struct{})

	// 同步消费消息，不再使用goroutine，由调用者决定是否使用协程
	defer func() {
		c.running = false
		close(c.stopChan)
	}()

	// 设置默认超时时间
	timeout := c.readTimeout
	if timeout <= 0 {
		timeout = 100 * time.Millisecond
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.stopChan:
			return nil
		default:
			// 读取消息
			msg, err := c.inst.ReadMessage(timeout)
			if err != nil {
				// 忽略超时错误
				if err.(kafka.Error).Code() == kafka.ErrTimedOut {
					// time.Sleep(100 * time.Millisecond)
					// su_logger.Debug(ctx, "kafka read message timeout", su_logger.E().String("group", c.group).String("topic", c.topic))
					continue
				}

				// 记录错误日志
				su_logger.Error(ctx, err, "kafka consume error",
					su_logger.E().String("group", c.group).String("topic", c.topic))

				// 调用重试钩子
				if c.retryPolicy != nil && c.retryPolicy.Hook != nil {
					c.retryPolicy.Hook(ctx, c.topic, c.group, err)
				}

				// 如果设置了重试策略，则进行重试
				if c.retryPolicy != nil && c.retryPolicy.Interval > 0 {
					var retryErr error
					for i := 1; i <= c.retryPolicy.MaxConsecutiveRetryTimes; i++ {
						time.Sleep(c.retryPolicy.Interval)
						// 尝试重新订阅
						retryErr = c.inst.SubscribeTopics(c.topics, nil)
						if retryErr != nil {
							su_logger.Error(ctx, retryErr, "kafka resubscribe failed",
								su_logger.E().String("group", c.group).String("topic", c.topic).Int("attempt", i))
						} else {
							su_logger.Debug(ctx, "kafka resubscribe success",
								su_logger.E().String("group", c.group).String("topic", c.topic).Int("attempt", i))
							break
						}
					}

					if retryErr != nil {
						return retryErr
					}
					continue
				}

				return err
			}

			// 处理消息
			var cstMsg *mq.ConsumerMessage
			var uCtx context.Context
			if c.autoAck {
				uCtx, cstMsg = toConsumerMessage(nil, msg)
			} else {
				uCtx, cstMsg = toConsumerMessage(c.inst, msg)
			}
			if uCtx == nil {
				uCtx = ctx
			}

			// 如果设置了立即确认，则立即确认消息
			if c.autoAck && c.immediatelyAck {
				_ = cstMsg.Ack()
			}

			// 处理消息
			err = handler(uCtx, cstMsg)
			if err != nil {
				su_logger.Error(uCtx, err, "kafka message handler error",
					su_logger.E().String("topic", *msg.TopicPartition.Topic).
						String("key", string(msg.Key)).
						String("value", string(msg.Value)))
			}

			// 如果设置了自动确认但不是立即确认，则在处理完后确认
			if c.autoAck {
				_ = cstMsg.Ack()
			}
		}
	}
}

// NewConsumer 创建一个新的Kafka消费者
func (c *Client) NewConsumer(ctx context.Context, cfg mq.ConsumerConf) (cl mq.Consumer, err error) {
	conf := c.Config.ToConfigMap()

	// 设置消费者组ID
	conf.SetKey("group.id", cfg.Group)

	// 设置初始位置 - 修复逻辑错误
	initialPosition := "latest"
	if cfg.InitialPosition == mq.SubscriptionPositionEarliest {
		initialPosition = "earliest"
	}
	conf.SetKey("auto.offset.reset", initialPosition)

	// 设置是否自动提交
	conf.SetKey("enable.auto.commit", !cfg.DisableAutoAck)

	// 设置消费者特定配置
	if cfg.EnableMessageOrdering {
		conf.SetKey("enable.partition.eof", true)
	}

	if cfg.AllowAutoCreateTopics {
		conf.SetKey("allow.auto.create.topics", true)
	}

	// 创建Kafka消费者
	consumer, err := kafka.NewConsumer(conf)
	if err != nil {
		su_logger.Error(ctx, err, "create kafka consumer failed",
			su_logger.E().String("topic", cfg.Topic).String("group", cfg.Group))
		return nil, err
	}

	// 订阅主题
	topics := []string{cfg.Topic}
	err = consumer.SubscribeTopics(topics, nil)
	if err != nil {
		su_logger.Error(ctx, err, "kafka subscribe topic failed",
			su_logger.E().String("topic", cfg.Topic).String("group", cfg.Group))
		consumer.Close()
		return nil, err
	}

	// 设置重试策略
	retryPolicy := cfg.RetryPolicy
	if retryPolicy != nil {
		retryPolicy.MaxConsecutiveRetryTimes = su_math.GetMax(retryPolicy.MaxConsecutiveRetryTimes, 1)
	}

	var readTimeout time.Duration
	if cfg.ConsumerReadTimeout > 0 {
		readTimeout = cfg.ConsumerReadTimeout
	} else {
		readTimeout = 100 * time.Millisecond
	}

	return &Consumer{
		inst:           consumer,
		topic:          cfg.Topic,
		topics:         topics,
		group:          cfg.Group,
		autoAck:        !cfg.DisableAutoAck,
		retryPolicy:    retryPolicy,
		immediatelyAck: cfg.ImmediatelyAck,
		config:         conf,
		readTimeout:    readTimeout,
	}, nil
}

// Producer Kafka生产者实现
type Producer struct {
	inst          *kafka.Producer
	conf          *mq.ProducerConf
	pool          *ants.Pool
	poolOnce      sync.Once
	asyncPoolSize int
}

// Close 关闭生产者
func (p *Producer) Close() error {
	if p.pool != nil {
		p.pool.Release()
	}
	if p.inst != nil {
		p.inst.Close()
	}
	return nil
}

// 将mq.ProducerMessage转换为kafka.Message
func toKafkaMessage(ctx context.Context, msg *mq.ProducerMessage, topic string) *kafka.Message {
	traceId, spanId, traceParam, debug := trace.ParseCore(ctx)
	if msg.Attributes == nil {
		msg.Attributes = make(map[string]string)
	}
	if traceId != "" {
		msg.Attributes[enum.CtxRequestId] = traceId
	}
	if spanId != "" {
		msg.Attributes[enum.CtxSpanId] = spanId
	}
	if traceParam != "" {
		msg.Attributes[enum.CtxTrackingParams] = traceParam
	}
	if debug != "" {
		msg.Attributes[enum.CtxDebug] = debug
	}

	return &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: kafka.PartitionAny,
		},
		Value:         msg.Payload,
		Key:           []byte(msg.ID),
		Headers:       toKafkaHeaders(msg.Attributes),
		Timestamp:     msg.EventTime,
		TimestampType: kafka.TimestampCreateTime,
	}
}

// 将map[string]string转换为[]kafka.Header
func toKafkaHeaders(attrs map[string]string) []kafka.Header {
	if len(attrs) == 0 {
		return nil
	}

	headers := make([]kafka.Header, 0, len(attrs))
	for k, v := range attrs {
		headers = append(headers, kafka.Header{
			Key:   k,
			Value: []byte(v),
		})
	}
	return headers
}

// 获取异步发送使用的协程池
func (p *Producer) getAsyncPool() *ants.Pool {
	if p.pool == nil {
		poolSize := su_math.GetMax(p.asyncPoolSize, 16)
		p.poolOnce.Do(func() {
			p.pool, _ = ants.NewPool(poolSize, ants.WithPreAlloc(true), ants.WithExpiryDuration(time.Hour))
		})
	}

	return p.pool
}

// Send 同步发送消息
func (p *Producer) Send(ctx context.Context, msg *mq.ProducerMessage) error {
	defer func() {
		if err := recover(); err != nil {
			su_logger.Panic(ctx, "kafka Send", su_logger.E().String("topic", p.conf.Topic).Any("err", err))
		}
	}()

	kafkaMsg := toKafkaMessage(ctx, msg, p.conf.Topic)

	// 使用Kafka的生产者发送消息
	err := p.inst.Produce(kafkaMsg, nil)
	if err != nil {
		su_logger.Error(ctx, err, "kafka Send failed", su_logger.E().String("topic", p.conf.Topic).String("msgId", msg.ID))
		return err
	}

	return nil
}

// AsyncSend 异步发送消息
func (p *Producer) AsyncSend(ctx context.Context, msg *mq.ProducerMessage, handler mq.AsyncMsgHandler) error {
	defer func() {
		if err := recover(); err != nil {
			su_logger.Panic(ctx, "kafka AsyncSend", su_logger.E().String("topic", p.conf.Topic).Any("err", err))
		}
	}()

	return p.getAsyncPool().Submit(func() {
		kafkaMsg := toKafkaMessage(ctx, msg, p.conf.Topic)

		// 使用Kafka的生产者发送消息
		deliveryChan := make(chan kafka.Event, 1)
		err := p.inst.Produce(kafkaMsg, deliveryChan)

		if err != nil {
			su_logger.Error(ctx, err, "kafka AsyncSend failed", su_logger.E().String("topic", p.conf.Topic).String("msgId", msg.ID))
			handler(ctx, msg.ID, msg, err)
			close(deliveryChan)
			return
		}

		// 等待投递结果，使用select避免阻塞
		select {
		case e := <-deliveryChan:
			switch ev := e.(type) {
			case *kafka.Message:
				var err error
				if ev.TopicPartition.Error != nil {
					err = ev.TopicPartition.Error
				}
				handler(ctx, msg.ID, msg, err)
			case kafka.Error:
				handler(ctx, msg.ID, msg, ev)
			}
		case <-time.After(30 * time.Second): // 添加超时处理
			handler(ctx, msg.ID, msg, fmt.Errorf("kafka async send timeout"))
		}
		close(deliveryChan)
	})
}

// Flush 刷新所有消息
func (p *Producer) Flush() error {
	remainingMessages := p.inst.Flush(15 * 1000) // 等待最多15秒
	if remainingMessages > 0 {
		return fmt.Errorf("kafka flush timeout, remaining messages: %d", remainingMessages)
	}
	return nil
}

// BatchSend 批量发送消息
func (p *Producer) BatchSend(ctx context.Context, msgList []*mq.ProducerMessage, forceFlush bool) error {
	defer func() {
		if err := recover(); err != nil {
			su_logger.Panic(ctx, "kafka BatchSend", su_logger.E().String("topic", p.conf.Topic).Any("err", err))
		}
	}()

	if len(msgList) == 0 {
		return nil
	}

	for i := range msgList {
		if msgList[i] == nil {
			continue
		}

		err := p.Send(ctx, msgList[i])
		if err != nil {
			return err
		}
	}

	if forceFlush {
		return p.Flush()
	}

	return nil
}

func (c *Client) NewProducer(ctx context.Context, cfg mq.ProducerConf) (pdc mq.Producer, err error) {
	conf := c.Config.ToConfigMap()

	// 设置生产者特定配置
	if cfg.EnableMessageOrdering {
		conf.SetKey("enable.idempotence", true)
	}

	// 设置压缩方式
	if cfg.CompressionType != "" {
		conf.SetKey("compression.type", string(cfg.CompressionType))
	}

	// 设置批量发送相关配置
	if cfg.DelayThresholdSec > 0 {
		conf.SetKey("linger.ms", cfg.DelayThresholdSec*1000)
	}

	if cfg.CountThreshold > 0 {
		conf.SetKey("batch.num.messages", cfg.CountThreshold)
	}

	if cfg.ByteThreshold > 0 {
		conf.SetKey("batch.size", cfg.ByteThreshold)
	}

	if cfg.TimeoutSec > 0 {
		conf.SetKey("delivery.timeout.ms", cfg.TimeoutSec*1000)
	}

	producer, err := kafka.NewProducer(conf)
	if err != nil {
		su_logger.Error(ctx, err, "create kafka producer failed", su_logger.E().String("topic", cfg.Topic))
		return nil, err
	}

	// 启动一个goroutine来处理事件通道
	go func() {
		for e := range producer.Events() {
			switch ev := e.(type) {
			case *kafka.Message:
				if ev.TopicPartition.Error != nil {
					su_logger.Error(ctx, ev.TopicPartition.Error, "kafka delivery failed",
						su_logger.E().String("topic", *ev.TopicPartition.Topic).String("key", string(ev.Key)))
				}
			case kafka.Error:
				su_logger.Error(ctx, ev, "kafka error", su_logger.E().String("topic", cfg.Topic))
			}
		}
	}()

	return &Producer{
		inst:          producer,
		conf:          &cfg,
		asyncPoolSize: 16, // 默认异步池大小
	}, nil
}
