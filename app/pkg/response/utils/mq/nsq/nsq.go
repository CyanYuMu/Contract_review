package nsq

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"github.com/nsqio/go-nsq"
	"github.com/panjf2000/ants/v2"
	"github.com/spf13/cast"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/su_math"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/mq"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/trace"
)

// Client NSQ客户端
type Client struct {
	ctx    context.Context
	Config *Config
}

// New 创建NSQ客户端
func New(ctx context.Context, cnf *Config) (cli *Client) {
	cli = &Client{
		ctx:    ctx,
		Config: cnf,
	}
	return
}

// 创建NSQ配置
func (c *Client) createNSQConfig() *nsq.Config {
	config := nsq.NewConfig()

	if c.Config.Connection != nil {
		conn := c.Config.Connection
		if conn.DialTimeout > 0 {
			config.DialTimeout = conn.DialTimeout
		}
		if conn.ReadTimeout > 0 {
			config.ReadTimeout = conn.ReadTimeout
		}
		if conn.WriteTimeout > 0 {
			config.WriteTimeout = conn.WriteTimeout
		}
		if conn.HeartbeatInterval > 0 {
			config.HeartbeatInterval = conn.HeartbeatInterval
		}
		if conn.MsgTimeout > 0 {
			config.MsgTimeout = conn.MsgTimeout
		}
		if conn.MaxRequeueDelay > 0 {
			config.MaxRequeueDelay = conn.MaxRequeueDelay
		}
		if conn.DefaultRequeueDelay > 0 {
			config.DefaultRequeueDelay = conn.DefaultRequeueDelay
		}
		if conn.MaxAttempts > 0 {
			config.MaxAttempts = conn.MaxAttempts
		}
		if conn.LowRdyIdleTimeout > 0 {
			config.LowRdyIdleTimeout = conn.LowRdyIdleTimeout
		}
		if conn.RDYRedistributeInterval > 0 {
			config.RDYRedistributeInterval = conn.RDYRedistributeInterval
		}
		if conn.ClientID != "" {
			config.ClientID = conn.ClientID
		}
		if conn.Hostname != "" {
			config.Hostname = conn.Hostname
		}
		if conn.UserAgent != "" {
			config.UserAgent = conn.UserAgent
		}
	}

	// 设置认证
	if c.Config.Auth != nil && c.Config.Auth.Secret != "" {
		config.AuthSecret = c.Config.Auth.Secret
	}

	// 设置TLS
	if c.Config.TLS != nil && c.Config.TLS.Enabled {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: c.Config.TLS.InsecureSkipVerify,
		}

		if c.Config.TLS.CertFile != "" && c.Config.TLS.KeyFile != "" {
			cert, err := tls.LoadX509KeyPair(c.Config.TLS.CertFile, c.Config.TLS.KeyFile)
			if err == nil {
				tlsConfig.Certificates = []tls.Certificate{cert}
			}
		}

		config.TlsV1 = true
		config.TlsConfig = tlsConfig
	}

	return config
}

// Consumer NSQ消费者实现
type Consumer struct {
	inst           *nsq.Consumer
	topic          string
	channel        string
	autoAck        bool
	retryPolicy    *mq.RetryPolicy
	immediatelyAck bool
	running        bool
	stopChan       chan struct{}
	config         *nsq.Config
	handler        mq.MsgHandler
	ctx            context.Context
	lookupdAddrs   []string
	nsqdAddrs      []string
}

// Close 关闭消费者
func (c *Consumer) Close() error {
	if c.running {
		if c.stopChan != nil {
			close(c.stopChan)
		}
		c.running = false
	}
	if c.inst != nil {
		c.inst.Stop()
		// 使用 select 避免无限等待
		select {
		case <-c.inst.StopChan:
			// 正常关闭
		case <-time.After(5 * time.Second):
			// 超时，强制退出
		}
	}
	return nil
}

// 将nsq.Message转换为mq.ConsumerMessage
func toConsumerMessage(ctx context.Context, msg *nsq.Message, autoAck bool) (context.Context, *mq.ConsumerMessage) {
	if msg == nil {
		return nil, nil
	}

	// 提取消息属性 - NSQ消息没有原生的属性支持，我们可以通过消息体的JSON格式来实现
	attributes := make(map[string]string)
	msgCtx := ctx

	// 尝试从消息ID中提取trace信息（如果有的话）
	if len(msg.ID) > 0 {
		// NSQ的消息ID是16字节，我们可以将其转换为字符串作为消息ID
		msgId := fmt.Sprintf("%x", msg.ID)
		attributes["nsq_msg_id"] = msgId
	}

	// 添加NSQ特有的属性
	attributes["nsq_attempts"] = cast.ToString(msg.Attempts)
	attributes["nsq_timestamp"] = cast.ToString(msg.Timestamp)

	return msgCtx, &mq.ConsumerMessage{
		Payload:    msg.Body,
		ID:         fmt.Sprintf("%x", msg.ID),
		Attributes: attributes,
		EventTime:  time.Unix(0, msg.Timestamp),
		DeliveryAttempt: func() *int {
			attempts := int(msg.Attempts)
			return &attempts
		}(),
		// 实现Ack和Nack函数
		Ack: func() error {
			if autoAck {
				return nil // 自动ACK模式下不需要手动ACK
			}
			msg.Finish()
			return nil
		},
		Nack: func() error {
			if autoAck {
				return nil // 自动ACK模式下不需要手动NACK
			}
			msg.Requeue(-1) // -1表示使用默认的重新排队延迟
			return nil
		},
	}
}

// NSQ消息处理器适配器
type nsqHandlerAdapter struct {
	handler        mq.MsgHandler
	ctx            context.Context
	autoAck        bool
	immediatelyAck bool
}

func (h *nsqHandlerAdapter) HandleMessage(message *nsq.Message) error {
	// 转换消息格式
	msgCtx, cstMsg := toConsumerMessage(h.ctx, message, h.autoAck)
	if msgCtx == nil {
		msgCtx = h.ctx
	}

	// 如果设置了立即确认，则立即确认消息
	if h.autoAck && h.immediatelyAck {
		_ = cstMsg.Ack()
	}

	// 处理消息
	err := h.handler(msgCtx, cstMsg)
	if err != nil {
		su_logger.Error(msgCtx, err, "nsq message handler error",
			su_logger.E().String("msg_id", cstMsg.ID).
				String("attempts", cast.ToString(message.Attempts)))

		// 如果处理失败且不是自动ACK模式，则重新排队
		if !h.autoAck {
			message.Requeue(-1)
		}
		return err
	}

	// 如果设置了自动确认但不是立即确认，则在处理完后确认
	if h.autoAck && !h.immediatelyAck {
		_ = cstMsg.Ack()
	}

	return nil
}

// Consume 实现消费消息的方法
func (c *Consumer) Consume(ctx context.Context, handler mq.MsgHandler) error {
	if c.running {
		return fmt.Errorf("consumer is already running")
	}

	c.running = true
	c.stopChan = make(chan struct{})
	c.handler = handler
	c.ctx = ctx

	defer func() {
		c.running = false
		if c.stopChan != nil {
			close(c.stopChan)
		}
	}()

	// 创建消息处理器适配器
	handlerAdapter := &nsqHandlerAdapter{
		handler:        handler,
		ctx:            ctx,
		autoAck:        c.autoAck,
		immediatelyAck: c.immediatelyAck,
	}

	// 添加处理器
	c.inst.AddHandler(handlerAdapter)

	// 连接到NSQLookupd或NSQd
	var err error
	if len(c.lookupdAddrs) > 0 {
		err = c.inst.ConnectToNSQLookupds(c.lookupdAddrs)
	} else if len(c.nsqdAddrs) > 0 {
		for _, addr := range c.nsqdAddrs {
			if connErr := c.inst.ConnectToNSQD(addr); connErr != nil {
				err = connErr
				break
			}
		}
	} else {
		err = fmt.Errorf("no nsqlookupd or nsqd addresses provided")
	}

	if err != nil {
		su_logger.Error(ctx, err, "nsq connect failed",
			su_logger.E().String("topic", c.topic).String("channel", c.channel))
		return err
	}

	// 等待停止信号或上下文取消
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.stopChan:
		return nil
	}
}

// NewConsumer 创建一个新的NSQ消费者
func (c *Client) NewConsumer(ctx context.Context, cfg mq.ConsumerConf) (cl mq.Consumer, err error) {
	config := c.createNSQConfig()

	// 设置消费者特定配置
	if cfg.AckDeadline > 0 {
		config.MsgTimeout = cfg.AckDeadline
	}

	if cfg.ConsumerReadTimeout > 0 {
		config.ReadTimeout = cfg.ConsumerReadTimeout
	}

	// NSQ使用channel作为消费者组的概念
	channel := cfg.Group
	if channel == "" {
		channel = "default"
	}

	// 创建NSQ消费者
	consumer, err := nsq.NewConsumer(cfg.Topic, channel, config)
	if err != nil {
		su_logger.Error(ctx, err, "create nsq consumer failed",
			su_logger.E().String("topic", cfg.Topic).String("channel", channel))
		return nil, err
	}

	// 设置并发处理数量
	if cfg.NumGoroutines > 0 {
		consumer.ChangeMaxInFlight(cfg.NumGoroutines)
	}

	// 设置重试策略
	retryPolicy := cfg.RetryPolicy
	if retryPolicy != nil {
		retryPolicy.MaxConsecutiveRetryTimes = su_math.GetMax(retryPolicy.MaxConsecutiveRetryTimes, 1)
	}

	return &Consumer{
		inst:           consumer,
		topic:          cfg.Topic,
		channel:        channel,
		autoAck:        !cfg.DisableAutoAck,
		retryPolicy:    retryPolicy,
		immediatelyAck: cfg.ImmediatelyAck,
		config:         config,
		lookupdAddrs:   c.Config.LookupdAddresses,
		nsqdAddrs:      c.Config.NSQdAddresses,
	}, nil
}

// Producer NSQ生产者实现
type Producer struct {
	inst          *nsq.Producer
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
		p.inst.Stop()
	}
	return nil
}

// 将mq.ProducerMessage转换为NSQ消息格式
func toNSQMessage(ctx context.Context, msg *mq.ProducerMessage) []byte {
	// NSQ不支持消息属性，我们可以将trace信息嵌入到消息体中
	// 这里简化处理，直接返回消息体
	// 如果需要传递trace信息，可以将消息包装成JSON格式

	traceId, spanId, traceParam, debug := trace.ParseCore(ctx)

	// NSQ不支持消息属性，如果需要传递trace信息，可以将消息包装成JSON格式
	// 这里简化处理，直接返回原始消息体
	_ = traceId
	_ = spanId
	_ = traceParam
	_ = debug

	return msg.Payload
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
			su_logger.Panic(ctx, "nsq Send", su_logger.E().String("topic", p.conf.Topic).Any("err", err))
		}
	}()

	nsqMsg := toNSQMessage(ctx, msg)

	// 使用NSQ的生产者发送消息
	err := p.inst.Publish(p.conf.Topic, nsqMsg)
	if err != nil {
		su_logger.Error(ctx, err, "nsq Send failed", su_logger.E().String("topic", p.conf.Topic).String("msgId", msg.ID))
		return err
	}

	return nil
}

// AsyncSend 异步发送消息
func (p *Producer) AsyncSend(ctx context.Context, msg *mq.ProducerMessage, handler mq.AsyncMsgHandler) error {
	defer func() {
		if err := recover(); err != nil {
			su_logger.Panic(ctx, "nsq AsyncSend", su_logger.E().String("topic", p.conf.Topic).Any("err", err))
		}
	}()

	return p.getAsyncPool().Submit(func() {
		nsqMsg := toNSQMessage(ctx, msg)

		// NSQ的异步发送
		responseChan := make(chan *nsq.ProducerTransaction, 1)
		err := p.inst.PublishAsync(p.conf.Topic, nsqMsg, responseChan)

		if err != nil {
			su_logger.Error(ctx, err, "nsq AsyncSend failed", su_logger.E().String("topic", p.conf.Topic).String("msgId", msg.ID))
			handler(ctx, msg.ID, msg, err)
			close(responseChan)
			return
		}

		// 等待发送结果，使用select避免阻塞
		select {
		case trans := <-responseChan:
			var resultErr error
			if trans.Error != nil {
				resultErr = trans.Error
			}
			handler(ctx, msg.ID, msg, resultErr)
		case <-time.After(30 * time.Second): // 添加超时处理
			handler(ctx, msg.ID, msg, fmt.Errorf("nsq async send timeout"))
		}
		close(responseChan)
	})
}

// Flush 刷新所有消息 - NSQ没有flush概念，这里为了接口兼容性
func (p *Producer) Flush() error {
	// NSQ生产者没有flush方法，直接返回nil
	return nil
}

// BatchSend 批量发送消息
func (p *Producer) BatchSend(ctx context.Context, msgList []*mq.ProducerMessage, forceFlush bool) error {
	defer func() {
		if err := recover(); err != nil {
			su_logger.Panic(ctx, "nsq BatchSend", su_logger.E().String("topic", p.conf.Topic).Any("err", err))
		}
	}()

	if len(msgList) == 0 {
		return nil
	}

	// NSQ支持批量发送
	nsqMsgList := make([][]byte, 0, len(msgList))
	for i := range msgList {
		if msgList[i] == nil {
			continue
		}
		nsqMsg := toNSQMessage(ctx, msgList[i])
		nsqMsgList = append(nsqMsgList, nsqMsg)
	}

	if len(nsqMsgList) == 0 {
		return nil
	}

	err := p.inst.MultiPublish(p.conf.Topic, nsqMsgList)
	if err != nil {
		su_logger.Error(ctx, err, "nsq BatchSend failed", su_logger.E().String("topic", p.conf.Topic).Int("count", len(nsqMsgList)))
		return err
	}

	// forceFlush在NSQ中没有意义，忽略
	return nil
}

// NewProducer 创建一个新的NSQ生产者
func (c *Client) NewProducer(ctx context.Context, cfg mq.ProducerConf) (pdc mq.Producer, err error) {
	config := c.createNSQConfig()

	// 设置生产者特定配置
	if cfg.TimeoutSec > 0 {
		config.WriteTimeout = time.Duration(cfg.TimeoutSec) * time.Second
	}

	// 选择连接地址 - NSQ生产者通常直连NSQd
	var addr string
	if len(c.Config.NSQdAddresses) > 0 {
		addr = c.Config.NSQdAddresses[0] // 选择第一个NSQd地址
	} else if len(c.Config.LookupdAddresses) > 0 {
		// 如果没有NSQd地址，可以尝试从lookupd获取，但这里简化处理
		return nil, fmt.Errorf("nsq producer requires direct nsqd address, not lookupd")
	} else {
		return nil, fmt.Errorf("no nsqd addresses provided for producer")
	}

	// 创建NSQ生产者
	producer, err := nsq.NewProducer(addr, config)
	if err != nil {
		su_logger.Error(ctx, err, "create nsq producer failed", su_logger.E().String("topic", cfg.Topic).String("addr", addr))
		return nil, err
	}

	return &Producer{
		inst:          producer,
		conf:          &cfg,
		asyncPoolSize: 16, // 默认异步池大小
	}, nil
}
