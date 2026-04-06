package mq

import (
	"context"
	"time"
)

type ProducerMessage struct {
	// Payload for the message
	Payload []byte
	// Key sets the key of the message for routing policy
	ID string
	// OrderingKey sets the ordering key of the message
	OrderingKey string

	// Properties attach application defined properties on the message
	Attributes map[string]string

	// EventTime set the event time for a given message
	// By default, messages don't have an event time associated, while the publish
	// time will be be always present.
	// Set the event time to a non-zero timestamp to explicitly declare the time
	// that the event "happened", as opposed to when the message is being published.
	EventTime time.Time
	// DeliverAfter requests to deliver the message only after the specified relative delay.
	// Note: messages are only delivered with delay when a consumer is consuming
	//     through a `SubscriptionType=Shared` subscription. With other subscription
	//     types, the messages will still be delivered immediately.
	DeliverAfter time.Duration

	// DeliverAt delivers the message only at or after the specified absolute timestamp.
	// Note: messages are only delivered with delay when a consumer is consuming
	//     through a `SubscriptionType=Shared` subscription. With other subscription
	//     types, the messages will still be delivered immediately.
	DeliverAt time.Time
	// DeliveryAttempt is the number of times a message has been delivered.
	DeliveryAttempt *int
}

type ConsumerMessage struct {
	// Payload for the message
	Payload []byte
	// Key sets the key of the message for routing policy
	ID string
	// OrderingKey sets the ordering key of the message
	OrderingKey string

	// Properties attach application defined properties on the message
	Attributes map[string]string

	// EventTime set the event time for a given message
	// By default, messages don't have an event time associated, while the publish
	// time will be be always present.
	// Set the event time to a non-zero timestamp to explicitly declare the time
	// that the event "happened", as opposed to when the message is being published.
	EventTime time.Time
	// DeliverAfter requests to deliver the message only after the specified relative delay.
	// Note: messages are only delivered with delay when a consumer is consuming
	//     through a `SubscriptionType=Shared` subscription. With other subscription
	//     types, the messages will still be delivered immediately.
	DeliverAfter time.Duration

	// DeliverAt delivers the message only at or after the specified absolute timestamp.
	// Note: messages are only delivered with delay when a consumer is consuming
	//     through a `SubscriptionType=Shared` subscription. With other subscription
	//     types, the messages will still be delivered immediately.
	DeliverAt time.Time
	// DeliveryAttempt is the number of times a message has been delivered.
	DeliveryAttempt *int

	Ack  func() error
	Nack func() error
}

type ProducerConf struct {
	Topic                 string
	DelayThresholdSec     int
	CountThreshold        int
	ByteThreshold         int
	TimeoutSec            int
	Name                  string
	EnableMessageOrdering bool
	// 压缩方式, 目前仅服务于 kafka, 支持的压缩方式：none(默认), gzip, snappy, lz4, zstd, 其中 snappy, lz4 两种方式在速度和压缩比例之间平衡的较好
	CompressionType CompressionType
	// Redis Stream 最大长度限制，为0时使用默认值
	MaxStreamLen int
}

type CompressionType string

const (
	CompressionTypeNone   CompressionType = "none"
	CompressionTypeGzip   CompressionType = "gzip"
	CompressionTypeSnappy CompressionType = "snappy"
	CompressionTypeLz4    CompressionType = "lz4"
	CompressionTypeZstd   CompressionType = "zstd"
)

type ConsumerConf struct {
	// 单个主题
	Topic string
	//同时消费多个主题
	//Topics []string
	Group string
	// 消费者ID，为空时基于hostname自动生成
	ConsumerID string
	// 自定义协程数量
	NumGoroutines int
	// 默认自动ack
	DisableAutoAck bool
	// 消息异常投递的重试策略
	RetryPolicy           *RetryPolicy
	EnableMessageOrdering bool
	AckDeadline           time.Duration
	// 消息保留时长
	RetentionDuration   time.Duration
	RetainAckedMessages bool
	// 过期策略, 为0时, 默认31天
	ExpirationPolicy time.Duration
	// 消费者异常的重试策略
	//CstRetryPolicy *CstRetryPolicy
	// 是否立即ack
	ImmediatelyAck bool
	// 开启仅投递成功一次
	EnableExactlyOnceDelivery bool
	KeySharedPolicy           KeySharedPolicyMode
	ConsumeType               SubscriptionType
	// 初始化位置, 默认oldest
	InitialPosition SubscriptionInitialPosition
	// 初始化位置时间, 默认7天前, 目前仅pubsub可用
	InitialPositionTime time.Time
	DLQPolicy           *DLQPolicy
	ReceiverQueueSize   int
	// 消费者最大空闲时间, 为0时, 默认24小时
	ConsumerMaxIdleTime time.Duration
	// 消费者读取消息超时时间, 为0时, 默认100ms
	ConsumerReadTimeout time.Duration
	// allow.auto.create.topics
	AllowAutoCreateTopics bool
	// Redis Stream 最大长度限制，为0时使用默认值
	MaxStreamLen int64
	// SubscriptionGroupExistsCheck 是否在创建 Pub/Sub 消费者前对订阅执行 Exists 检测。
	// nil 表示 true（与历史行为一致）；显式设为 false 指针则跳过 Exists，错误延后到 Receive。
	SubscriptionGroupExistsCheck *bool
}

// 死信队列
type DLQPolicy struct {
	// MaxDeliveries specifies the maximum number of times that a message will be delivered before being
	// sent to the dead letter queue.
	MaxDeliveries uint32

	// DeadLetterTopic specifies the name of the topic where the failing messages will be sent.
	DeadLetterTopic string
	// RetryLetterTopic specifies the name of the topic where the retry messages will be sent.
	RetryLetterTopic string
}

type SubscriptionInitialPosition int

const (
	// SubscriptionPositionLatest is the latest position which means the start consuming position
	// will be the last message
	SubscriptionPositionLatest SubscriptionInitialPosition = iota

	// SubscriptionPositionEarliest is the earliest position which means the start consuming position
	// will be the first message
	SubscriptionPositionEarliest
)

type KeySharedPolicyMode int

const (
	// KeySharedPolicyModeAutoSplit Auto split hash range key shared policy.
	KeySharedPolicyModeAutoSplit KeySharedPolicyMode = iota
	// KeySharedPolicyModeSticky is Sticky attach topic with fixed hash range.
	KeySharedPolicyModeSticky
)

//type CstRetryPolicy struct {
//	// 重试间隔, 0 表示不开启(默认)
//	Interval time.Duration
//	// 重试钩子
//	Hook func(ctx context.Context, topic string, group string, err error)
//	// 连续重试n次, 默认1次
//	MaxConsecutiveRetryTimes int
//}

type RetryPolicy struct {
	// MinimumBackoff is the minimum delay between consecutive deliveries of a
	// given message. Value should be between 0 and 600 seconds. Defaults to 10 seconds.
	MinimumBackoff time.Duration
	// MaximumBackoff is the maximum delay between consecutive deliveries of a
	// given message. Value should be between 0 and 600 seconds. Defaults to 600 seconds.
	MaximumBackoff time.Duration
	// 重试间隔, 0 表示不开启(默认)
	Interval time.Duration
	// 重试钩子
	Hook func(ctx context.Context, topic string, group string, err error)
	// 连续重试n次, 默认1次
	MaxConsecutiveRetryTimes int
}

// SubscriptionType of subscription supported by Pulsar
type SubscriptionType int

const (
	// Exclusive there can be only 1 consumer on the same topic with the same subscription name
	Exclusive SubscriptionType = iota

	// Shared subscription mode, multiple consumer will be able to use the same subscription name
	// and the messages will be dispatched according to
	// a round-robin rotation between the connected consumers
	Shared

	// Failover subscription mode, multiple consumer will be able to use the same subscription name
	// but only 1 consumer will receive the messages.
	// If that consumer disconnects, one of the other connected consumers will start receiving messages.
	Failover

	// KeyShared subscription mode, multiple consumer will be able to use the same
	// subscription and all messages with the same key will be dispatched to only one consumer
	KeyShared
)

type MsgHandler func(ctx context.Context, msg *ConsumerMessage) error
type AsyncMsgHandler func(ctx context.Context, msgId string, msg *ProducerMessage, err error)

type Producer interface {
	Send(ctx context.Context, msg *ProducerMessage) error
	BatchSend(ctx context.Context, msgList []*ProducerMessage, forceFlush bool) error
	AsyncSend(ctx context.Context, msg *ProducerMessage, handler AsyncMsgHandler) error
	Flush() error
}

type Consumer interface {
	Consume(ctx context.Context, handler MsgHandler) error
}
