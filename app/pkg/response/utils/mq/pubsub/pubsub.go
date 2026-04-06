package pubsub

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/panjf2000/ants/v2"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/enum"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/encrypt"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/su_math"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/mq"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/trace"
	"google.golang.org/api/option"
)

type Client struct {
	*pubsub.Client
}

type PubSubConfig struct {
	CredentialsJson string
	ProjectId       string
	Options         []option.ClientOption
}

func New(ctx context.Context, cnf PubSubConfig) (cli *Client, err error) {
	cli = &Client{}
	byteData, err := encrypt.Base64Decode([]byte(cnf.CredentialsJson))
	if err != nil {
		return nil, err
	}
	cnf.Options = append(cnf.Options, option.WithCredentialsJSON(byteData))
	cli.Client, err = pubsub.NewClient(ctx, cnf.ProjectId, cnf.Options...)

	return
}

type Consumer struct {
	inst *pubsub.Subscription
	// 自动ack
	autoAck     bool
	topic       *pubsub.Topic
	retryPolicy *mq.RetryPolicy
	recSettings pubsub.ReceiveSettings
	subConf     pubsub.SubscriptionConfig
	group       string
	// 是否立即ack
	immediatelyAck bool
	cli            *pubsub.Client
}

func toConsumerMessage(msg *pubsub.Message) (context.Context, *mq.ConsumerMessage) {
	if msg == nil {
		return nil, nil
	}
	ctx := context.Background()
	if msg.Attributes != nil {
		for k, v := range msg.Attributes {
			if k == enum.CtxRequestId {
				ctx = context.WithValue(ctx, enum.CtxRequestId, v)
			} else if k == enum.CtxSpanId {
				ctx = context.WithValue(ctx, enum.CtxSpanId, v)
			} else if k == enum.CtxTrackingParams {
				ctx = context.WithValue(ctx, enum.CtxTrackingParams, v)
			} else if k == enum.CtxDebug {
				ctx = context.WithValue(ctx, enum.CtxDebug, v)
			}
		}
	}

	return ctx, &mq.ConsumerMessage{
		Payload:         msg.Data,
		ID:              msg.ID,
		OrderingKey:     msg.OrderingKey,
		Attributes:      msg.Attributes,
		EventTime:       msg.PublishTime,
		DeliveryAttempt: msg.DeliveryAttempt,
		Ack: func() error {
			msg.Ack()
			return nil
		},
		Nack: func() error {
			msg.Nack()
			return nil
		},
	}
}

func toProducerMessage(ctx context.Context, msg *mq.ProducerMessage) *pubsub.Message {
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

	return &pubsub.Message{
		ID:              msg.ID,
		Data:            msg.Payload,
		Attributes:      msg.Attributes,
		PublishTime:     msg.EventTime,
		DeliveryAttempt: msg.DeliveryAttempt,
		OrderingKey:     msg.OrderingKey,
	}
}

func (c *Consumer) Consume(ctx context.Context, handler mq.MsgHandler) error {
	var err error
	for {
		err = c.inst.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
			if c.autoAck && c.immediatelyAck {
				msg.Ack()
			} else if c.autoAck {
				defer msg.Ack()
			}

			uCtx, oneMsg := toConsumerMessage(msg)
			if uCtx == nil {
				uCtx = ctx
			}
			err1 := handler(uCtx, oneMsg)
			if err1 != nil {
				su_logger.Error(uCtx, err1, "pubSub consume", su_logger.E().String("id", msg.ID).String("data", string(msg.Data)))
			}
		})
		if c.retryPolicy.Hook != nil {
			c.retryPolicy.Hook(ctx, c.topic.String(), c.group, err)
		}
		// 当ctx.Done时, 返回的 err == nil, 此时不应该重试, 重试场景仅针对
		if err != nil && c.retryPolicy.Interval > 0 {
			var err2 error
			for i := 1; i <= c.retryPolicy.MaxConsecutiveRetryTimes; i++ {
				sleep := pubsubConsumeRetryBackoff(i, c.retryPolicy)
				time.Sleep(sleep)
				c.inst, err2 = c.doCreateSub(ctx, c.group, c.subConf, c.recSettings)
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

// pubsubConsumeRetryBackoff 指数退避：第 1 次重试为 Interval（或 MinimumBackoff），之后每次翻倍，上限 MaximumBackoff（默认 60s）。
func pubsubConsumeRetryBackoff(attempt int, rp *mq.RetryPolicy) time.Duration {
	if rp == nil {
		return time.Second
	}
	base := rp.Interval
	if base <= 0 && rp.MinimumBackoff > 0 {
		base = rp.MinimumBackoff
	}
	if base <= 0 {
		base = time.Second
	}
	max := rp.MaximumBackoff
	if max <= 0 {
		max = 60 * time.Second
	}
	if max < base {
		max = base
	}
	d := base
	for j := 1; j < attempt; j++ {
		next := d * 2
		if next > max {
			return max
		}
		d = next
	}
	if d > max {
		return max
	}
	return d
}

//type ConsumerConf struct {
//	Topic string
//	Group string
//	// 自定义协程数量
//	NumGoroutines int
//	// 默认自动ack
//	DisableAutoAck bool
//	// 消息异常投递的重试策略
//	RetryPolicy           *pubsub.RetryPolicy
//	EnableMessageOrdering bool
//	AckDeadline           time.Duration
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
//}

//type CstRetryPolicy struct {
//	// 重试间隔, 0 表示不开启(默认)
//	Interval time.Duration
//	// 重试钩子
//	Hook func(ctx context.Context, topic string, group string, err error)
//	// 连续重试n次, 默认1次
//	MaxConsecutiveRetryTimes int
//}

func (c *Client) NewConsumer(ctx context.Context, cfg mq.ConsumerConf) (cl mq.Consumer, err error) {
	topic := c.Client.Topic(cfg.Topic)

	var retryPolicy *pubsub.RetryPolicy
	if cfg.RetryPolicy != nil {
		retryPolicy = &pubsub.RetryPolicy{
			MinimumBackoff: cfg.RetryPolicy.MinimumBackoff,
			MaximumBackoff: cfg.RetryPolicy.MaximumBackoff,
		}
	}

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
		RetryPolicy:                   retryPolicy,
		Detached:                      false,
		TopicMessageRetentionDuration: 0,
		EnableExactlyOnceDelivery:     cfg.EnableExactlyOnceDelivery,
		State:                         0,
	}

	var cstRetryPolicy = &mq.RetryPolicy{}
	if cfg.RetryPolicy != nil {
		cstRetryPolicy = cfg.RetryPolicy
	}
	if cstRetryPolicy.Interval <= 0 {
		// 如果重试间隔为0, 则随机生成1-11s的间隔
		cstRetryPolicy.Interval = time.Second * (1 + time.Duration(rand.Intn(10))) // 1-11s
	}
	cstRetryPolicy.MaxConsecutiveRetryTimes = su_math.GetMax(cstRetryPolicy.MaxConsecutiveRetryTimes, 1)

	cst := &Consumer{
		autoAck:     !cfg.DisableAutoAck,
		topic:       topic,
		retryPolicy: cstRetryPolicy,
		subConf:     subConf,
		group:       cfg.Group,
		recSettings: pubsub.ReceiveSettings{
			NumGoroutines: cfg.NumGoroutines,
		},
		immediatelyAck: cfg.ImmediatelyAck,
		cli:            c.Client,
	}
	inst := c.Client.Subscription(cst.group)
	existsCheck := true
	if cfg.SubscriptionGroupExistsCheck != nil {
		existsCheck = *cfg.SubscriptionGroupExistsCheck
	}
	if existsCheck {
		exists, errExists := inst.Exists(ctx)
		if errExists != nil {
			return nil, errExists
		}
		if !exists {
			e := fmt.Errorf("subscription %s doesn't exist", cst.group)
			su_logger.Error(ctx, e, "subscription doesn't exist", su_logger.E().String("group", cst.group))
			return nil, e
		}
	}
	inst.ReceiveSettings = cst.recSettings
	cst.inst = inst

	// 判断当前的起始位置
	if cfg.InitialPosition == mq.SubscriptionPositionEarliest {
		timeBefore := time.Now().Add(-7 * 24 * time.Hour)
		// 自定义初始化的时间必须要小于当前时间
		if cfg.InitialPositionTime.Before(time.Now()) {
			timeBefore = cfg.InitialPositionTime
		}
		// 将当前消费位置设置为最早, 7天前的消息NewConsumer
		cst.inst.SeekToTime(ctx, timeBefore)
	}

	return cst, nil
}

func (c *Consumer) doCreateSub(ctx context.Context, group string, conf pubsub.SubscriptionConfig, receiveConf pubsub.ReceiveSettings) (inst *pubsub.Subscription, err error) {
	inst, err = c.cli.CreateSubscription(ctx, group, conf)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			err = nil
		} else {
			return nil, err
		}
	}

	inst = c.cli.Subscription(group)
	inst.ReceiveSettings = receiveConf

	return inst, nil
}

type Producer struct {
	inst          *pubsub.Topic
	conf          *mq.ProducerConf
	pool          *ants.Pool
	poolOnce      sync.Once
	asyncPoolSize int
	cli           *pubsub.Client
}

func (p *Producer) getAsyncPool() *ants.Pool {
	if p.pool == nil {
		poolSize := su_math.GetMax(p.asyncPoolSize, 16)
		p.poolOnce.Do(func() {
			p.pool, _ = ants.NewPool(poolSize, ants.WithPreAlloc(true), ants.WithExpiryDuration(time.Hour))
		})
	}

	return p.pool
}

func (p *Producer) Send(ctx context.Context, msg *mq.ProducerMessage) error {
	defer func() {
		if err := recover(); err != nil {
			su_logger.Panic(ctx, "pubSub BatchSend", su_logger.E().String("topic", p.conf.Topic).Any("err", err))
		}
	}()
	sendMsg := toProducerMessage(ctx, msg)
	_ = p.inst.Publish(ctx, sendMsg)

	return nil
}

/*AsyncSend
* @Description: 不支持异步发送
* @param ctx
* @param msg
* @param handler
* @return error
 */
func (p *Producer) AsyncSend(ctx context.Context, msg *mq.ProducerMessage, handler mq.AsyncMsgHandler) error {
	defer func() {
		if err := recover(); err != nil {
			su_logger.Panic(ctx, "pubSub BatchSend", su_logger.E().String("topic", p.conf.Topic).Any("err", err))
		}
	}()

	return p.getAsyncPool().Submit(func() {
		err := p.Send(ctx, msg)

		handler(ctx, msg.ID, msg, err)
	})
}

func (p *Producer) Flush() error {
	p.inst.Flush()

	return nil
}

func (p *Producer) BatchSend(ctx context.Context, msgList []*mq.ProducerMessage, forceFlush bool) error {
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

		err := p.Send(ctx, msgList[i])
		if err != nil {
			return err
		}
	}

	if forceFlush {
		_ = p.Flush()
	}

	return nil
}

//type ProducerConf struct {
//	Topic                 string
//	DelayThresholdSec     int
//	CountThreshold        int
//	ByteThreshold         int
//	TimeoutSec            int
//	EnableMessageOrdering bool
//	// 异步投递时, 使用的协程池数量
//	AsyncPoolSize int
//}

func (c *Client) CreateTopic(ctx context.Context, topic string) (id string, err error) {
	t, err := c.Client.CreateTopic(ctx, topic)
	if err != nil {
		return "", err
	}

	return t.ID(), nil
}

func (c *Client) newProducer(cfg mq.ProducerConf) *pubsub.Topic {
	t := c.Client.Topic(cfg.Topic)
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

func (c *Client) NewProducer(ctx context.Context, cfg mq.ProducerConf) (pdc mq.Producer, err error) {
	t := c.newProducer(cfg)

	producer := &Producer{
		inst: t,
		conf: &cfg,
		cli:  c.Client,
	}

	return producer, err
}
