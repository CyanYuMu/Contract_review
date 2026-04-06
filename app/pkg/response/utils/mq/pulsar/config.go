package pulsar

import (
	"regexp"
	"time"
)

type Config struct {
	Addr           string
	Authentication string

	// 默认30s
	OperationTimeout time.Duration
	// 默认30s
	ConnectionTimeout time.Duration
	EnableLog         bool
}

func newDftPulsarConfig() *Config {
	return &Config{
		OperationTimeout:  30 * time.Second,
		ConnectionTimeout: 30 * time.Second,
	}
}

func (p *Config) Apply(cfg *Config) {
	if cfg.Addr != "" {
		p.Addr = cfg.Addr
	}
	if cfg.Authentication != "" {
		p.Authentication = cfg.Authentication
	}
	if cfg.OperationTimeout > 0 {
		p.OperationTimeout = cfg.OperationTimeout
	}
	if cfg.ConnectionTimeout > 0 {
		p.ConnectionTimeout = cfg.ConnectionTimeout
	}
}

var TagPattern = regexp.MustCompile(`^\{(.+?)\}.`)

type SharedKeyTagPolicy struct {
	regex *regexp.Regexp
}

func (c *SharedKeyTagPolicy) GetKeyHash(key string) uint32 {
	// 使用正则表达式提取{}中的内容，并剔除
	submatches := TagPattern.FindStringSubmatch(key)
	if len(submatches) > 1 {
		return uint32(len(submatches[1]))
	}
	return uint32(len(key))
}

//func DftSharedKeyTagPolicy() *pulsar.KeySharedPolicy {
//	return &pulsar.KeySharedPolicy{
//		Mode: pulsar.KeySharedPolicyModeSticky,
//		//HashRanges:             []int{0,31},
//		AllowOutOfOrderDelivery: true,
//	}
//}

//type ConsumerConf struct {
//	Topic string
//	// 支持监听多个topic
//	Topics []string
//	// 消费组名
//	Group string
//	// 自定义协程数量
//	//NumGoroutines int
//	// 默认自动ack
//	DisableAutoAck bool
//	AckDeadline    time.Duration
//	// 消费类型, 1->独占 2=>分享(默认) 3=>备用 4=>共享
//	ConsumeType pulsar.SubscriptionType
//	// 初始化位置, 默认oldest
//	InitialPosition pulsar.SubscriptionInitialPosition
//	// 消息保留时长
//	RetentionDuration   time.Duration
//	RetainAckedMessages bool
//	// 过期策略, 为0时, 默认31天
//	ExpirationPolicy time.Duration
//	// 消费者异常的重试策略
//	CstRetryPolicy *CstRetryPolicy
//	// 是否立即ack
//	ImmediatelyAck bool
//	// 开启仅投递成功一次
//	EnableExactlyOnceDelivery bool
//	// 自动nack的最大时间间隔, 默认1分钟
//	NackRedeliveryDelay time.Duration
//	// 默认黏贴模式
//	KeySharedPolicy *pulsar.KeySharedPolicy
//	// 死信队列配置, 默认关闭
//	DeadLetterPolicy *pulsar.DLQPolicy
//	// 默认1000
//	ReceiverQueueSize int
//	// 消费失败开启重试队列
//	RetryEnable bool
//}

//type CstRetryPolicy struct {
//	// 重试间隔, 0 表示不开启(默认)
//	Interval time.Duration
//	// 重试钩子
//	Hook func(ctx context.Context, topic string, group string, err error)
//	// 连续重试n次, 默认1次
//	MaxConsecutiveRetryTimes int
//	// 对于nack的消息进行重试的间隔, 默认 0
//	MsgReconsumeLater time.Duration
//}
