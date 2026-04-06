package redis_mq_v9

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/redis/go-redis/v9"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/su_json"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/mq"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
)

// Go 1.22中随机数生成器已经自动使用全局熵源，不再需要手动设置种子

// Config Redis MQ 配置
type Config struct {
	Cli redis.UniversalClient
}

// Client Redis MQ 客户端
type Client struct {
	cli redis.UniversalClient
}

// Producer Redis 生产者
type Producer struct {
	cli    redis.UniversalClient
	topic  string
	config mq.ProducerConf
	maxLen int64 // Stream最大长度，在创建时设置一次
}

// Consumer Redis 消费者
type Consumer struct {
	cli        redis.UniversalClient
	topic      string
	maxLen     int64
	group      string
	config     mq.ConsumerConf
	consumerID string // 稳定的消费者ID
}

// New 创建 Redis MQ 客户端
func New(cfg *Config) (*Client, error) {
	return &Client{cli: cfg.Cli}, nil
}

// NewProducer 创建生产者
func (c *Client) NewProducer(ctx context.Context, conf mq.ProducerConf) (mq.Producer, error) {
	// 设置Stream最大长度
	var maxLen int64 = MaxStreamLen
	if conf.MaxStreamLen > 0 {
		maxLen = int64(conf.MaxStreamLen)
	}

	return &Producer{
		cli:    c.cli,
		topic:  conf.Topic,
		config: conf,
		maxLen: maxLen,
	}, nil
}

func getDelayKey(topic string) string {
	return fmt.Sprintf("sys_delay:%s", topic)
}

// generateConsumerID 生成消费者ID
// 如果手动指定ConsumerID则使用，否则基于hostname生成
func generateConsumerID(topic, group, manualID string) string {
	// 如果手动指定了ConsumerID，直接使用
	if manualID != "" {
		return manualID
	}

	// 基于hostname生成稳定ID
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	identifier := fmt.Sprintf("%s-%s-%s", hostname, topic, group)
	hash := md5.Sum([]byte(identifier))
	return hex.EncodeToString(hash[:])[:12]
}

// NewConsumer 创建消费者
func (c *Client) NewConsumer(ctx context.Context, conf mq.ConsumerConf) (mq.Consumer, error) {
	if conf.ConsumerMaxIdleTime == 0 {
		conf.ConsumerMaxIdleTime = 7 * 24 * time.Hour
	}

	// 生成消费者ID
	consumerID := generateConsumerID(conf.Topic, conf.Group, conf.ConsumerID)

	consumer := &Consumer{
		cli:        c.cli,
		topic:      conf.Topic,
		group:      conf.Group,
		config:     conf,
		consumerID: consumerID,
		maxLen:     conf.MaxStreamLen,
	}

	// 创建消费组
	err := consumer.createConsumerGroup(ctx)
	if err != nil {
		return nil, err
	}

	return consumer, nil
}

// createConsumerGroup 创建消费组
func (c *Consumer) createConsumerGroup(ctx context.Context) error {
	// 检查消费组是否存在
	groups, err := c.cli.XInfoGroups(ctx, c.topic).Result()
	if err != nil && err != redis.Nil && !strings.Contains(err.Error(), "ERR no such key") {
		return err
	}

	// 检查组是否已存在
	for _, group := range groups {
		if group.Name == c.group {
			return nil
		}
	}

	// 创建消费组，从最新消息开始消费
	startID := "$"
	if c.config.InitialPosition == mq.SubscriptionPositionEarliest {
		startID = "0"
	}

	err = c.cli.XGroupCreateMkStream(ctx, c.topic, c.group, startID).Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return err
	}

	return nil
}

const MaxStreamLen = 10000

// Send 发送消息
func (p *Producer) Send(ctx context.Context, msg *mq.ProducerMessage) error {
	// 将消息转换为 map
	values := make(map[string]interface{})
	values["payload"] = string(msg.Payload)
	values["id"] = msg.ID
	if msg.EventTime.IsZero() {
		values["event_time"] = time.Now().UnixMilli()
	} else {
		values["event_time"] = msg.EventTime.UnixMilli()
	}

	if len(msg.Attributes) > 0 {
		attrBytes, err := json.Marshal(msg.Attributes)
		if err != nil {
			return err
		}
		values["attributes"] = attrBytes
	}

	if msg.DeliverAfter > 0 {
		msg.DeliverAt = time.Now().Add(msg.DeliverAfter)
	}

	// 处理延迟消息
	if !msg.DeliverAt.IsZero() {
		values["deliver_at"] = msg.DeliverAt.UnixMilli()
		delayKey := getDelayKey(p.topic)
		data := su_json.MustMarshalToString(values)
		z := redis.Z{
			Score:  float64(msg.DeliverAt.UnixMilli()),
			Member: data,
		}
		return p.cli.ZAdd(ctx, delayKey, z).Err()
	}

	// 使用 XADD 添加消息到 Stream，使用Producer级别的maxLen配置
	return p.cli.XAdd(ctx, &redis.XAddArgs{
		Stream: p.topic,
		Values: values,
		MaxLen: p.maxLen,
	}).Err()
}

// BatchSend 批量发送消息
func (p *Producer) BatchSend(ctx context.Context, msgList []*mq.ProducerMessage, forceFlush bool) error {
	pipe := p.cli.Pipeline()

	for _, msg := range msgList {
		values := make(map[string]interface{})
		values["payload"] = msg.Payload
		values["id"] = msg.ID
		values["ordering_key"] = msg.OrderingKey
		values["event_time"] = msg.EventTime.UnixNano()

		if len(msg.Attributes) > 0 {
			attrBytes, err := json.Marshal(msg.Attributes)
			if err != nil {
				return err
			}
			values["attributes"] = attrBytes
		}

		if msg.DeliverAfter > 0 || !msg.DeliverAt.IsZero() {
			var deliverAt time.Time
			if msg.DeliverAfter > 0 {
				deliverAt = time.Now().Add(msg.DeliverAfter)
			} else {
				deliverAt = msg.DeliverAt
			}

			values["deliver_at"] = deliverAt.UnixMilli()
			delayKey := getDelayKey(p.topic)

			z := redis.Z{
				Score:  float64(deliverAt.UnixMilli()),
				Member: su_json.MustMarshalToString(values),
			}
			pipe.ZAdd(ctx, delayKey, z)
			continue
		}

		pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: p.topic,
			Values: values,
			MaxLen: p.maxLen,
		})
	}

	_, err := pipe.Exec(ctx)
	return err
}

// AsyncSend 异步发送消息
func (p *Producer) AsyncSend(ctx context.Context, msg *mq.ProducerMessage, handler mq.AsyncMsgHandler) error {
	go func() {
		err := p.Send(ctx, msg)
		if handler != nil {
			handler(ctx, msg.ID, msg, err)
		}
	}()
	return nil
}

// Flush 刷新缓冲区
func (p *Producer) Flush() error {
	return nil // Redis Stream 不需要实现 flush
}

func (c *Consumer) cleanIdleConsumers(ctx context.Context, topic, group string) error {
	// 创建分布式锁key
	lockKey := fmt.Sprintf("clean_idle_consumers_lock:%s:%s", topic, group)
	// 设置锁过期时间为1分钟
	lockExpire := time.Minute

	// 尝试获取分布式锁
	ok, err := c.cli.SetNX(ctx, lockKey, "1", lockExpire).Result()
	if err != nil {
		return fmt.Errorf("acquire lock failed: %w", err)
	}
	if !ok {
		// 其他进程正在清理,直接返回
		return nil
	}

	// 函数结束时释放锁
	defer c.cli.Del(ctx, lockKey)

	consumers, err := c.cli.XInfoConsumers(ctx, topic, group).Result()
	if err != nil {
		return err
	}

	for _, consumer := range consumers {
		if consumer.Idle > c.config.ConsumerMaxIdleTime && consumer.Pending == 0 {
			c.cli.XGroupDelConsumer(ctx, topic, group, consumer.Name)
		}
	}
	return nil
}

// Consume 消费消息
func (c *Consumer) Consume(ctx context.Context, handler mq.MsgHandler) error {
	// 使用稳定的消费者ID
	consumerID := c.consumerID

	// 清理空闲消费者
	if c.config.ConsumerMaxIdleTime > 0 {
		go c.cleanIdleConsumers(ctx, c.topic, c.group)
	}

	// 获取重试策略配置
	var maxRetries int = 10                         // 默认最大重试次数
	var baseDelay time.Duration = time.Second       // 默认基础延迟1秒
	var maxBackoff time.Duration = 60 * time.Second // 默认最大退避时间60秒

	if c.config.RetryPolicy != nil {
		if c.config.RetryPolicy.MaxConsecutiveRetryTimes > 0 {
			maxRetries = c.config.RetryPolicy.MaxConsecutiveRetryTimes
		}
		if c.config.RetryPolicy.MinimumBackoff > 0 {
			baseDelay = c.config.RetryPolicy.MinimumBackoff
		}
		if c.config.RetryPolicy.MaximumBackoff > 0 {
			maxBackoff = c.config.RetryPolicy.MaximumBackoff
		}
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Minute):
				c.checkPendingMessages(ctx, consumerID)
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// 处理延迟队列中的消息
			err := c.processDelayedMessages(ctx)
			if err != nil {
				su_logger.Error(ctx, err, "process redis delayed messages failed")
			}

			// 使用指数退避算法从Stream读取消息
			var streams []redis.XStream
			readErr := retryWithExponentialBackoff(
				ctx,
				func() error {
					var err error
					streams, err = c.cli.XReadGroup(ctx, &redis.XReadGroupArgs{
						Group:    c.group,
						Consumer: consumerID,
						Streams:  []string{c.topic, ">"},
						Count:    100,
						Block:    time.Millisecond * 100,
					}).Result()

					if err == redis.Nil {
						// redis.Nil不是真正的错误，只是表示没有消息
						return nil
					}
					return err
				},
				maxRetries,
				baseDelay,
				maxBackoff,
			)

			if readErr != nil {
				su_logger.Error(ctx, readErr, "persistent error reading messages from Redis Stream",
					su_logger.E().
						String("topic", c.topic).
						String("group", c.group))
				return readErr
			}

			// 如果没有消息或者出现redis.Nil，继续下一轮循环
			if len(streams) == 0 {
				continue
			}

			// 处理读取到的消息
			for _, stream := range streams {
				for _, message := range stream.Messages {
					msg, err := c.convertToConsumerMessage(message)
					if err != nil {
						su_logger.Error(ctx, err, "convert redis message failed")
						continue
					}

					// 设置 Ack/Nack 函数
					messageID := message.ID
					msg.Ack = func() error {
						return c.cli.XAck(ctx, c.topic, c.group, messageID).Err()
					}
					msg.Nack = func() error {
						// XCLAIM 重新认领消息，让其他消费者处理
						return c.cli.XClaim(ctx, &redis.XClaimArgs{
							Stream:   c.topic,
							Group:    c.group,
							Consumer: consumerID,
							MinIdle:  time.Minute,
							Messages: []string{messageID},
						}).Err()
					}

					// 处理消息
					err = handler(ctx, msg)
					if err != nil {
						su_logger.Error(ctx, err, "failed to process message",
							su_logger.E().
								String("message_id", messageID).
								String("topic", c.topic).
								String("group", c.group))

						if !c.config.DisableAutoAck {
							nackErr := msg.Nack()
							if nackErr != nil {
								su_logger.Error(ctx, nackErr, "failed to nack message",
									su_logger.E().
										String("message_id", messageID).
										String("topic", c.topic).
										String("group", c.group))
							}
						}
						continue
					}

					if !c.config.DisableAutoAck {
						ackErr := msg.Ack()
						if ackErr != nil {
							su_logger.Error(ctx, ackErr, "failed to ack message",
								su_logger.E().
									String("message_id", messageID).
									String("topic", c.topic).
									String("group", c.group))
						}
					}
				}
			}
		}
	}
}

// processDelayedMessages 处理延迟队列中的消息
func (c *Consumer) processDelayedMessages(ctx context.Context) error {
	delayKey := getDelayKey(c.topic)
	now := float64(time.Now().UnixMilli())

	// 获取重试策略配置
	var maxRetries int = 5                          // 默认最大重试次数
	var baseDelay time.Duration = time.Second       // 默认基础延迟1秒
	var maxBackoff time.Duration = 30 * time.Second // 默认最大退避时间30秒

	if c.config.RetryPolicy != nil {
		if c.config.RetryPolicy.MaxConsecutiveRetryTimes > 0 {
			maxRetries = c.config.RetryPolicy.MaxConsecutiveRetryTimes / 2 // 对延迟消息处理使用较少的重试次数
			if maxRetries < 3 {
				maxRetries = 3 // 至少重试3次
			}
		}
		if c.config.RetryPolicy.MinimumBackoff > 0 {
			baseDelay = c.config.RetryPolicy.MinimumBackoff
		}
		if c.config.RetryPolicy.MaximumBackoff > 0 {
			maxBackoff = c.config.RetryPolicy.MaximumBackoff / 2 // 最大退避时间减少
		}
	}

	// 使用指数退避算法获取到期的消息
	var msgs []string
	err := retryWithExponentialBackoff(
		ctx,
		func() error {
			var err error
			msgs, err = c.cli.ZRangeByScore(ctx, delayKey, &redis.ZRangeBy{
				Min: "0",
				Max: fmt.Sprintf("%f", now),
			}).Result()

			if err != nil && err != redis.Nil {
				return err
			}
			return nil
		},
		maxRetries,
		baseDelay,
		maxBackoff,
	)

	if err != nil {
		return err
	}

	if len(msgs) > 0 {
		pipe := c.cli.Pipeline()
		for _, msg := range msgs {
			// 将消息添加到 Stream
			var data map[string]interface{}
			err := jsoniter.UnmarshalFromString(msg, &data)
			if err != nil {
				su_logger.Error(ctx, err, "process redis delayed messages failed msg: "+msg)
				continue
			}
			pipe.XAdd(ctx, &redis.XAddArgs{
				Stream: c.topic,
				Values: data,
				MaxLen: c.maxLen,
			})
			// 从延迟队列中删除
			pipe.ZRem(ctx, delayKey, msg)
		}

		// 使用指数退避算法执行管道命令
		execErr := retryWithExponentialBackoff(
			ctx,
			func() error {
				_, err := pipe.Exec(ctx)
				return err
			},
			maxRetries,
			baseDelay,
			maxBackoff,
		)

		if execErr != nil {
			su_logger.Error(ctx, execErr, "process redis delayed messages failed",
				su_logger.E().Int("count", len(msgs)))
			return execErr
		}
	}

	return nil
}

// convertToConsumerMessage 将 Redis Stream 消息转换为 ConsumerMessage
func (c *Consumer) convertToConsumerMessage(msg redis.XMessage) (*mq.ConsumerMessage, error) {
	consumerMsg := &mq.ConsumerMessage{
		Payload: []byte(msg.Values["payload"].(string)),
		ID:      msg.Values["id"].(string),
	}

	// 解析事件时间
	if eventTime, ok := msg.Values["event_time"].(string); ok {
		timestamp, err := strconv.ParseInt(eventTime, 10, 64)
		if err == nil {
			consumerMsg.EventTime = time.Unix(0, timestamp)
		}
	}

	// 解析属性
	if attrs, ok := msg.Values["attributes"].(string); ok {
		var attributes map[string]string
		err := json.Unmarshal([]byte(attrs), &attributes)
		if err == nil {
			consumerMsg.Attributes = attributes
		}
	}

	return consumerMsg, nil
}

// retryWithExponentialBackoff 使用指数退避算法执行函数重试
// fn: 需要重试的函数
// maxRetries: 最大重试次数
// baseDelay: 基础延迟时间（秒）
// maxBackoff: 最大退避时间（秒）
// 返回：最终的错误结果，如果成功则为nil
func retryWithExponentialBackoff(
	ctx context.Context,
	fn func() error,
	maxRetries int,
	baseDelay time.Duration,
	maxBackoff time.Duration,
) error {
	var err error
	retryCount := 0
	delay := baseDelay

	for retryCount < maxRetries {
		// 执行函数
		err = fn()
		if err == nil {
			// 成功执行，返回nil
			return nil
		}
		su_logger.Error(ctx, err, "redis execute failed")

		// 检查上下文是否取消
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// 继续执行
		}

		// 计算随机抖动时间（0-1000毫秒）
		jitter := time.Duration(rand.Intn(1000)) * time.Millisecond
		// 当前延迟时间 = 指数递增的退避时间 + 随机抖动
		currentDelay := delay + jitter

		// 确保不超过最大退避时间
		if currentDelay > maxBackoff {
			currentDelay = maxBackoff
		}

		su_logger.Warn(ctx, "Redis operation failed, will retry",
			su_logger.E().
				Int("retry_count", retryCount+1).
				Int("max_retries", maxRetries).
				Int64("delay_ms", currentDelay.Milliseconds()).
				Error(err))

		// 等待一段时间后重试
		timer := time.NewTimer(currentDelay)
		select {
		case <-timer.C:
			// 继续重试
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}

		// 指数增加延迟时间（下一次延迟时间翻倍）
		delay *= 2
		retryCount++
	}

	// 达到最大重试次数后返回最后一次错误
	return fmt.Errorf("operation still failed after maximum retry attempts (%d): %w", maxRetries, err)
}

// 定期执行以下逻辑
func (c *Consumer) checkPendingMessages(ctx context.Context, consumerId string) {
	pendingInfo, err := c.cli.XPending(ctx, c.topic, c.group).Result()
	if err != nil || pendingInfo.Count == 0 {
		return
	}

	idle := time.Minute * 1 // default idle time
	if c.config.AckDeadline > 0 {
		idle = c.config.AckDeadline
	}

	// end := "+" // 最大ID, 标识所有消息
	// default 24 hours
	end := time.Now().Add(time.Hour * 24)
	// RetentionDuration
	if c.config.RetentionDuration > 0 {
		end = time.Now().Add(c.config.RetentionDuration)
	}

	// 获取详细的 pending 消息列表
	pendingMsgs, err := c.cli.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: c.topic,
		Group:  c.group,
		Start:  "-",                                // 最小ID
		End:    fmt.Sprintf("%d", end.UnixMilli()), // 最大ID
		Count:  100,                                // 批次大小
		Idle:   idle,                               // 至少空闲5分钟的消息
	}).Result()

	if err != nil || len(pendingMsgs) == 0 {
		return
	}

	// 收集需要重新认领的消息ID
	msgIds := make([]string, 0, len(pendingMsgs))
	for _, msg := range pendingMsgs {
		msgIds = append(msgIds, msg.ID)
	}

	// 重新认领这些消息，XClaim 会返回消息内容
	claimedMsgs, err := c.cli.XClaim(ctx, &redis.XClaimArgs{
		Stream:   c.topic,
		Group:    c.group,
		Consumer: consumerId,
		MinIdle:  idle,
		Messages: msgIds,
	}).Result()
	if err != nil {
		su_logger.Error(ctx, err, "failed to claim messages",
			su_logger.E().String("topic", c.topic).String("group", c.group))
		return
	}

	if len(claimedMsgs) == 0 {
		return
	}

	// 将 pending 消息重新入队，并 ACK 旧消息
	c.requeuePendingMessages(ctx, claimedMsgs)
}

// requeuePendingMessages 将 pending 消息重新入队
func (c *Consumer) requeuePendingMessages(ctx context.Context, messages []redis.XMessage) {
	if len(messages) == 0 {
		return
	}

	pipe := c.cli.Pipeline()
	ackedIds := make([]string, 0, len(messages))

	for _, msg := range messages {
		// 防御性检查：确保消息内容有效
		if msg.Values == nil || len(msg.Values) == 0 {
			su_logger.Warn(ctx, "skip empty pending message",
				su_logger.E().String("message_id", msg.ID).String("topic", c.topic))
			continue
		}

		// 将消息重新添加到 Stream（作为新消息重新入队）
		pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: c.topic,
			Values: msg.Values,
			MaxLen: c.maxLen,
		})

		ackedIds = append(ackedIds, msg.ID)
	}

	// 执行重新入队操作
	_, err := pipe.Exec(ctx)
	if err != nil {
		su_logger.Error(ctx, err, "failed to requeue pending messages",
			su_logger.E().String("topic", c.topic).String("group", c.group).Int("count", len(messages)))
		return
	}

	// 入队成功后，ACK 掉旧的 pending 消息
	if len(ackedIds) > 0 {
		err = c.cli.XAck(ctx, c.topic, c.group, ackedIds...).Err()
		if err != nil {
			su_logger.Error(ctx, err, "failed to ack old pending messages after requeue",
				su_logger.E().String("topic", c.topic).String("group", c.group).Int("count", len(ackedIds)))
			return
		}
		su_logger.Info(ctx, "requeued pending messages successfully",
			su_logger.E().String("topic", c.topic).String("group", c.group).Int("count", len(ackedIds)))
	}
}
