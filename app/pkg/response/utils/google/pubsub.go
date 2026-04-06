package google

import (
	"cloud.google.com/go/pubsub"
	"context"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/encrypt"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
	"google.golang.org/api/option"
	"strings"
	"time"
)

type PubSub struct {
	*pubsub.Client
}

type PubSubConfig struct {
	CredentialsJson string
	ProjectId       string
	Options         []option.ClientOption
}

func NewPubSub(ctx context.Context, cnf PubSubConfig) (cli *PubSub, err error) {
	cli = &PubSub{}
	byteData, err := encrypt.Base64Decode([]byte(cnf.CredentialsJson))
	if err != nil {
		return nil, err
	}
	cnf.Options = append(cnf.Options, option.WithCredentialsJSON(byteData))
	cli.Client, err = pubsub.NewClient(ctx, cnf.ProjectId, cnf.Options...)

	return
}

type Consumer struct {
	p    *PubSub
	inst *pubsub.Subscription
	// 自动ack
	autoAck        bool
	topic          *pubsub.Topic
	cstRetryPolicy *CstRetryPolicy
	recSettings    pubsub.ReceiveSettings
	subConf        pubsub.SubscriptionConfig
	group          string
	// 是否立即ack
	immediatelyAck bool
}

func (c *Consumer) Consume(ctx context.Context, handler func(ctx context.Context, msg *pubsub.Message) error) error {
	var err error
	for {
		err = c.inst.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
			if c.autoAck && c.immediatelyAck {
				msg.Ack()
			} else if c.autoAck {
				defer msg.Ack()
			}

			err1 := handler(ctx, msg)
			if err1 != nil {
				su_logger.Error(ctx, err1, "pubSub consume", su_logger.E().String("id", msg.ID).String("data", string(msg.Data)))
			}
		})
		if c.cstRetryPolicy.Hook != nil {
			c.cstRetryPolicy.Hook(ctx, c.topic.String(), c.group, err)
		}
		// 当ctx.Done时, 返回的 err == nil, 此时不应该重试, 重试场景仅针对
		if err != nil && c.cstRetryPolicy.Interval > 0 {
			var err2 error
			for i := 1; i <= c.cstRetryPolicy.MaxConsecutiveRetryTimes; i++ {
				time.Sleep(c.cstRetryPolicy.Interval)
				c.inst, err2 = c.p.doCreateSub(ctx, c.group, c.subConf, c.recSettings)
				if err2 != nil {
					su_logger.Error(ctx, err2, "reSub try again", su_logger.E().String("group", c.group).String("topic", c.subConf.Topic.String()))
				} else {
					su_logger.Debug(ctx, "pubSub consume retry success", su_logger.E().String("group", c.group).String("topic", c.subConf.Topic.String()).Int("errCnt", i).Error(err2))
					break
				}
			}
			if err2 != nil {
				su_logger.Error(ctx, err2, "reSub exceed MaxConsecutiveRetryTimes, stopped!!!", su_logger.E().String("group", c.group).String("topic", c.subConf.Topic.String()))
				return err2
			}
		} else {
			return err
		}
	}
}

type ConsumerConf struct {
	Topic string
	Group string
	// 自定义协程数量
	NumGoroutines int
	// 默认自动ack
	DisableAutoAck bool
	// 消息异常投递的重试策略
	RetryPolicy           *pubsub.RetryPolicy
	EnableMessageOrdering bool
	AckDeadline           time.Duration
	// 消息保留时长
	RetentionDuration   time.Duration
	RetainAckedMessages bool
	// 过期策略, 为0时, 默认31天
	ExpirationPolicy time.Duration
	// 消费者异常的重试策略
	CstRetryPolicy *CstRetryPolicy
	// 是否立即ack
	ImmediatelyAck bool
	// 开启仅投递成功一次
	EnableExactlyOnceDelivery bool
}

type CstRetryPolicy struct {
	// 重试间隔, 0 表示不开启(默认)
	Interval time.Duration
	// 重试钩子
	Hook func(ctx context.Context, topic string, group string, err error)
	// 连续重试n次, 默认1次
	MaxConsecutiveRetryTimes int
}

func (p *PubSub) NewConsumer(ctx context.Context, cfg ConsumerConf) (cst *Consumer, err error) {
	topic := p.Client.Topic(cfg.Topic)

	subConf := pubsub.SubscriptionConfig{
		Topic:                         topic,
		PushConfig:                    pubsub.PushConfig{},
		BigQueryConfig:                pubsub.BigQueryConfig{},
		AckDeadline:                   cfg.AckDeadline,
		RetainAckedMessages:           cfg.RetainAckedMessages,
		RetentionDuration:             cfg.RetentionDuration,
		ExpirationPolicy:              cfg.ExpirationPolicy,
		Labels:                        nil,
		EnableMessageOrdering:         cfg.EnableMessageOrdering,
		DeadLetterPolicy:              nil,
		Filter:                        "",
		RetryPolicy:                   cfg.RetryPolicy,
		Detached:                      false,
		TopicMessageRetentionDuration: 0,
		EnableExactlyOnceDelivery:     cfg.EnableExactlyOnceDelivery,
		State:                         0,
	}

	var cstRetryPolicy = &CstRetryPolicy{}
	if cfg.CstRetryPolicy != nil {
		cstRetryPolicy = cfg.CstRetryPolicy
	}
	if cstRetryPolicy.MaxConsecutiveRetryTimes <= 0 {
		cstRetryPolicy.MaxConsecutiveRetryTimes = 1
	}

	cst = &Consumer{
		autoAck:        !cfg.DisableAutoAck,
		topic:          topic,
		cstRetryPolicy: cstRetryPolicy,
		subConf:        subConf,
		group:          cfg.Group,
		recSettings: pubsub.ReceiveSettings{
			NumGoroutines: cfg.NumGoroutines,
		},
		immediatelyAck: cfg.ImmediatelyAck,
	}
	cst.p = p
	cst.inst, err = p.doCreateSub(ctx, cst.group, subConf, cst.recSettings)

	return
}

func (p *PubSub) doCreateSub(ctx context.Context, group string, conf pubsub.SubscriptionConfig, receiveConf pubsub.ReceiveSettings) (inst *pubsub.Subscription, err error) {
	inst, err = p.Client.CreateSubscription(ctx, group, conf)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			err = nil
		} else {
			return nil, err
		}
	}

	inst = p.Client.Subscription(group)
	inst.ReceiveSettings = receiveConf

	return inst, nil
}

type Producer struct {
	inst *pubsub.Topic
	conf *ProducerConf
	p    *PubSub
}

func (p *Producer) Send(ctx context.Context, msg *pubsub.Message) error {
	defer func() {
		if err := recover(); err != nil {
			su_logger.Panic(ctx, "pubSub BatchSend", su_logger.E().String("topic", p.conf.Topic).Any("err", err))
		}
	}()
	_ = p.inst.Publish(ctx, msg)

	return nil
}

func (p *Producer) Flush() {
	p.inst.Flush()
}

func (p *Producer) BatchSend(ctx context.Context, msgList []*pubsub.Message, forceFlush ...bool) error {
	defer func() {
		if err := recover(); err != nil {
			su_logger.Panic(ctx, "pubSub BatchSend", su_logger.E().String("topic", p.conf.Topic).Any("err", err))
		}
	}()
	if len(msgList) == 0 {
		return nil
	}
	for i := range msgList {
		if msgList[i] == nil {
			continue
		}
		p.inst.Publish(ctx, msgList[i])
	}

	if len(forceFlush) > 0 && forceFlush[0] {
		p.Flush()
	}

	return nil
}

type ProducerConf struct {
	Topic                 string
	DelayThresholdSec     int
	CountThreshold        int
	ByteThreshold         int
	TimeoutSec            int
	EnableMessageOrdering bool
}

func (p *PubSub) CreateTopic(ctx context.Context, topic string) (id string, err error) {
	t, err := p.Client.CreateTopic(ctx, topic)
	return t.ID(), err
}

func (p *PubSub) newProducer(cfg ProducerConf) *pubsub.Topic {
	t := p.Client.Topic(cfg.Topic)
	t.PublishSettings = pubsub.DefaultPublishSettings
	if cfg.DelayThresholdSec > 0 {
		t.PublishSettings.DelayThreshold = time.Duration(cfg.DelayThresholdSec) * time.Second
	}
	if cfg.CountThreshold > 0 {
		t.PublishSettings.CountThreshold = cfg.CountThreshold
	}

	if cfg.ByteThreshold > 0 {
		t.PublishSettings.ByteThreshold = cfg.ByteThreshold
	}

	if cfg.TimeoutSec > 0 {
		t.PublishSettings.Timeout = time.Duration(cfg.TimeoutSec) * time.Second
	}

	t.EnableMessageOrdering = cfg.EnableMessageOrdering

	return t
}

func (p *PubSub) NewProducer(ctx context.Context, cfg ProducerConf) (producer *Producer, err error) {
	t := p.newProducer(cfg)

	producer = &Producer{
		inst: t,
		conf: &cfg,
		p:    p,
	}

	return producer, err
}
