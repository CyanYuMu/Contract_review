package pulsar

import (
	"context"
	"io/ioutil"
	"strings"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/apache/pulsar-client-go/pulsar/log"
	"github.com/sirupsen/logrus"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/enum"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/su_math"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/mq"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/trace"
)

type Client struct {
	pulsar.Client
}

func (c *Consumer) doCreateSub(ctx context.Context, opt pulsar.ConsumerOptions) (inst pulsar.Consumer, err error) {
	inst, err = c.cli.Subscribe(opt)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			err = nil
		} else {
			return nil, err
		}
	}

	return inst, nil
}

func New(cfg *Config) (cli *Client, err error) {
	config := newDftPulsarConfig()
	config.Apply(cfg)
	var authentication pulsar.Authentication
	if config.Authentication != "" {
		authentication = pulsar.NewAuthenticationToken(cfg.Authentication)
	}

	cliCfg := pulsar.ClientOptions{
		// 服务接入地址
		URL: config.Addr,
		// 授权角色密钥
		Authentication:    authentication,
		OperationTimeout:  config.OperationTimeout,
		ConnectionTimeout: config.ConnectionTimeout,
	}

	if !cfg.EnableLog {
		//cliCfg.Logger = &NoLogger{}
		//cliCfg.Logger.SetOutput()
		logger := logrus.StandardLogger()
		logger.SetOutput(ioutil.Discard)
		cliCfg.Logger = log.NewLoggerWithLogrus(logger)
	}

	client, err := pulsar.NewClient(cliCfg)

	if err != nil {
		return nil, err
	}

	cli = &Client{
		Client: client,
	}

	return cli, nil
}

type Consumer struct {
	inst pulsar.Consumer
	// 自动ack
	autoAck     bool
	retryPolicy *mq.RetryPolicy
	subConf     *pulsar.ConsumerOptions
	group       string
	// 是否立即ack
	immediatelyAck bool
	cli            pulsar.Client
}

func (c *Client) NewConsumer(ctx context.Context, cfg mq.ConsumerConf) (consumer mq.Consumer, err error) {

	keySharedPolicy := &pulsar.KeySharedPolicy{
		Mode:                    pulsar.KeySharedPolicyModeSticky,
		HashRanges:              nil,
		AllowOutOfOrderDelivery: true,
	}

	if cfg.EnableMessageOrdering {
		keySharedPolicy.AllowOutOfOrderDelivery = false
	}

	var cType pulsar.SubscriptionType
	switch cfg.ConsumeType {
	// 1->独占 2=>分享(默认) 3=>备用 4=>共享
	case 1:
		cType = pulsar.Exclusive
	case 2:
		cType = pulsar.Shared
	case 3:
		cType = pulsar.Failover
	case 4:
		cType = pulsar.KeyShared
	default:
		cType = pulsar.Shared
	}
	var initPos = pulsar.SubscriptionPositionLatest
	if cfg.InitialPosition == mq.SubscriptionPositionEarliest {
		initPos = pulsar.SubscriptionPositionEarliest
	}

	var dlq *pulsar.DLQPolicy
	if cfg.DLQPolicy != nil {
		dlq = &pulsar.DLQPolicy{}
		dlq.DeadLetterTopic = cfg.DLQPolicy.DeadLetterTopic
		dlq.RetryLetterTopic = cfg.DLQPolicy.RetryLetterTopic
		dlq.MaxDeliveries = cfg.DLQPolicy.MaxDeliveries
	}

	retryEnable := cfg.RetryPolicy != nil

	option := &pulsar.ConsumerOptions{
		Topic:                             cfg.Topic,
		AutoDiscoveryPeriod:               0,
		SubscriptionName:                  cfg.Group,
		Properties:                        nil,
		SubscriptionProperties:            nil,
		Type:                              cType,
		SubscriptionInitialPosition:       initPos,
		EventListener:                     nil,
		DLQ:                               dlq,
		KeySharedPolicy:                   keySharedPolicy,
		RetryEnable:                       retryEnable,
		MessageChannel:                    nil,
		ReceiverQueueSize:                 cfg.ReceiverQueueSize,
		EnableAutoScaledReceiverQueueSize: false,
		NackRedeliveryDelay:               0,
		Name:                              "",
		ReadCompacted:                     false,
		ReplicateSubscriptionState:        false,
		Interceptors:                      nil,
		Schema:                            nil,
		MaxReconnectToBroker:              nil,
		BackoffPolicy:                     nil,
		Decryption:                        nil,
		EnableDefaultNackBackoffPolicy:    false,
		NackBackoffPolicy:                 nil,
		AckWithResponse:                   false,
		MaxPendingChunkedMessage:          0,
		ExpireTimeOfIncompleteChunk:       0,
		AutoAckIncompleteChunk:            false,
		EnableBatchIndexAcknowledgment:    false,
		AckGroupingOptions:                nil,
		SubscriptionMode:                  0,
	}

	var cstRetryPolicy = &mq.RetryPolicy{}
	if cfg.RetryPolicy != nil {
		cstRetryPolicy = cfg.RetryPolicy
	}
	cstRetryPolicy.MaxConsecutiveRetryTimes = su_math.GetMax(cstRetryPolicy.MaxConsecutiveRetryTimes, 1)

	cst := &Consumer{
		autoAck:        !cfg.DisableAutoAck,
		retryPolicy:    cstRetryPolicy,
		subConf:        option,
		group:          cfg.Group,
		immediatelyAck: cfg.ImmediatelyAck,
		cli:            c.Client,
	}
	cst.inst, err = cst.doCreateSub(ctx, *option)

	return cst, err
}

func (c *Consumer) Consume(ctx context.Context, handler mq.MsgHandler) error {
	var err error
	for {
		for msg := range c.inst.Chan() {
			if c.autoAck && c.immediatelyAck {
				_ = msg.Ack(msg)
			}

			var uCtx, oneMsg = toConsumerMsg(&msg)
			if uCtx == nil {
				uCtx = ctx
			}
			err1 := handler(uCtx, oneMsg)

			if err1 != nil {
				su_logger.Error(uCtx, err1, "pubSub consume", su_logger.E().String("id", msg.ID().String()).String("data", string(msg.Payload())))
			}

			if c.autoAck && !c.immediatelyAck {
				_ = msg.Ack(msg)
			}
		}

		c.inst.Close()
		if c.retryPolicy.Hook != nil {
			c.retryPolicy.Hook(ctx, c.subConf.Topic, c.group, err)
		}

		if c.retryPolicy.Interval > 0 {
			for i := 1; i <= c.retryPolicy.MaxConsecutiveRetryTimes; i++ {
				time.Sleep(c.retryPolicy.Interval)
				inst, err1 := c.doCreateSub(ctx, *c.subConf)
				if err1 != nil {
					su_logger.Error(ctx, err, "reSub exceed MaxConsecutiveRetryTimes, stopped!!!", su_logger.E().String("group", c.group).String("topic", c.subConf.Topic))
				} else {
					su_logger.Debug(ctx, "pulsar consume retry success", su_logger.E().String("group", c.group).String("topic", c.subConf.Topic).Int("errCnt", i).Error(err))
					c.inst = inst
					break
				}
			}
			return err
		} else {
			return err
		}
	}
}

type Producer struct {
	cli  pulsar.Client
	inst pulsar.Producer
	opt  *pulsar.ProducerOptions
}

func (p *Producer) Send(ctx context.Context, msg *mq.ProducerMessage) error {
	defer func() {
		if err := recover(); err != nil {
			su_logger.Panic(ctx, "pubSub BatchSend", su_logger.E().String("topic", p.opt.Topic).Any("err", err))
		}
	}()
	sendMsg := toSendMsg(ctx, msg)
	_, err := p.inst.Send(ctx, sendMsg)
	return err
}

func toSendMsg(ctx context.Context, msg *mq.ProducerMessage) *pulsar.ProducerMessage {
	if msg == nil {
		return nil
	}

	if msg.EventTime.IsZero() {
		msg.EventTime = time.Now()
	}
	traceId, spanId, traceParam, debug := trace.ParseCore(ctx)
	attrs := msg.Attributes
	if attrs == nil {
		attrs = make(map[string]string)
	}
	if traceId != "" {
		attrs[enum.CtxRequestId] = traceId
	}
	if spanId != "" {
		attrs[enum.CtxSpanId] = spanId
	}
	if traceParam != "" {
		attrs[enum.CtxTrackingParams] = traceParam
	}
	if debug != "" {
		attrs[enum.CtxDebug] = debug
	}

	return &pulsar.ProducerMessage{
		Payload:      msg.Payload,
		Key:          msg.ID,
		OrderingKey:  msg.OrderingKey,
		Properties:   attrs,
		EventTime:    msg.EventTime,
		SequenceID:   nil,
		DeliverAfter: msg.DeliverAfter,
		DeliverAt:    msg.DeliverAt,
	}
}

func toConsumerMsg(msg *pulsar.ConsumerMessage) (context.Context, *mq.ConsumerMessage) {
	if msg == nil {
		return nil, nil
	}
	ctx := context.Background()
	attrs := msg.Properties()
	if attrs != nil {
		for k, v := range attrs {
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

	deliveryCnt := int(msg.RedeliveryCount())

	return ctx, &mq.ConsumerMessage{
		Payload:         msg.Payload(),
		ID:              msg.Key(),
		OrderingKey:     msg.OrderingKey(),
		Attributes:      msg.Properties(),
		EventTime:       msg.EventTime(),
		DeliverAt:       msg.PublishTime(),
		DeliveryAttempt: &deliveryCnt,
		Ack: func() error {
			return msg.Ack(msg)
		},
		Nack: func() error {
			msg.Nack(msg)
			return nil
		},
	}
}

/*AsyncSend
* @Description: 异步投递
* @param ctx
* @param msg
* @param handler
 */
func (p *Producer) AsyncSend(ctx context.Context, msg *mq.ProducerMessage, handler mq.AsyncMsgHandler) error {
	defer func() {
		if err := recover(); err != nil {
			su_logger.Panic(ctx, "pulsar BatchSend", su_logger.E().String("topic", p.opt.Topic).Any("err", err))
		}
	}()
	sendMsg := toSendMsg(ctx, msg)
	p.inst.SendAsync(ctx, sendMsg, func(id pulsar.MessageID, message *pulsar.ProducerMessage, err error) {
		handler(ctx, id.String(), msg, err)
	})

	return nil
}

func (p *Producer) Flush() error {
	return p.inst.Flush()
}

func (p *Producer) BatchSend(ctx context.Context, msgList []*mq.ProducerMessage, forceFlush bool) error {
	defer func() {
		if err := recover(); err != nil {
			su_logger.Panic(ctx, "pulsar BatchSend", su_logger.E().String("topic", p.opt.Topic).Any("err", err))
		}
	}()
	if len(msgList) == 0 {
		return nil
	}
	for i := range msgList {
		if msgList[i] == nil {
			continue
		}
		//_, err := p.inst.Send(ctx, msg)
		err := p.Send(ctx, msgList[i])
		if err != nil {
			return err
		}
	}

	if forceFlush {
		return p.inst.Flush()
	}

	return nil
}

type ProducerConf struct {
	Topic             string
	DelayThresholdSec int
	CountThreshold    int
	ByteThreshold     int
	Timeout           time.Duration
	// ?
	Name string
}

func (c *Client) NewProducer(ctx context.Context, cfg mq.ProducerConf) (pdc mq.Producer, err error) {
	timeOut := time.Duration(su_math.GetMax(cfg.TimeoutSec, 10)) * time.Second
	opt := pulsar.ProducerOptions{
		Topic:                           cfg.Topic,
		Name:                            cfg.Name,
		Properties:                      nil,
		SendTimeout:                     timeOut,
		DisableBlockIfQueueFull:         false,
		MaxPendingMessages:              0,
		HashingScheme:                   0,
		CompressionType:                 0,
		CompressionLevel:                0,
		MessageRouter:                   nil,
		DisableBatching:                 false,
		BatchingMaxPublishDelay:         0,
		BatchingMaxMessages:             0,
		BatchingMaxSize:                 0,
		Interceptors:                    nil,
		Schema:                          nil,
		MaxReconnectToBroker:            nil,
		BackoffPolicy:                   nil,
		BatcherBuilderType:              0,
		PartitionsAutoDiscoveryInterval: 0,
		DisableMultiSchema:              false,
		Encryption:                      nil,
		EnableChunking:                  false,
		ChunkMaxMessageSize:             0,
		ProducerAccessMode:              pulsar.ProducerAccessModeShared,
	}
	inst, err := c.Client.CreateProducer(opt)
	if err != nil {
		return nil, err
	}
	producer := &Producer{
		inst: inst,
		opt:  &opt,
		cli:  c.Client,
	}

	return producer, err
}
