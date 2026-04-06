package syncer

import (
	"context"
	"strings"
	"time"

	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/su_id"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
	"go.uber.org/zap/zapcore"
)

// 消息分隔符，用于分隔实例 ID 和消息内容
const msgSeparator = "|"

type Syncer struct {
	// 实例唯一标识，用于过滤自己发送的消息
	instanceID string
	opt        *Options
	cli        Synchronizer
}

type Options struct {
	// ? 消息主题, 默认是 "__l2_cache_syncer"
	Topic string
	// ? 批量处理消息的条数, 默认是 100
	BatchSize int
	// ? 批量处理消息的时间间隔, 默认是 100ms
	BatchInterval time.Duration
	// ? 日志级别, 默认是 zapcore.InfoLevel
	LogLevel zapcore.Level
}

func (o *Options) Apply(opt *Options) {
	if opt == nil {
		return
	}
	if opt.BatchSize > 0 {
		o.BatchSize = opt.BatchSize
	}
	if opt.BatchInterval > 0 {
		o.BatchInterval = opt.BatchInterval
	}
	if opt.Topic != "" {
		o.Topic = opt.Topic
	}
}

var DftSyncerTopic = "__l2_cache_syncer"

var getDftSyncerOption = func() *Options {
	return &Options{
		Topic:         DftSyncerTopic,
		BatchSize:     100,
		BatchInterval: 100 * time.Millisecond,
	}
}

func New(mq Synchronizer, opt *Options) *Syncer {
	option := getDftSyncerOption()
	option.Apply(opt)

	inst := &Syncer{
		instanceID: su_id.GetWithUUIDV7(),
		cli:        mq,
		opt:        option,
	}

	return inst
}

// InstanceID 返回当前实例的唯一标识
func (r *Syncer) InstanceID() string {
	return r.instanceID
}

func (r *Syncer) TopicName() string {
	return r.opt.Topic
}

func (r *Syncer) Publish(ctx context.Context, data string) error {
	// 消息格式: {instanceID}|{data}
	wrappedMsg := r.instanceID + msgSeparator + data
	return r.cli.Publish(ctx, r.opt.Topic, wrappedMsg)
}

// SubscriberHandler 单条消息处理函数
type SubscriberHandler func(ctx context.Context, data string) error

// BatchSubscriberHandler 批量消息处理函数
type BatchSubscriberHandler func(ctx context.Context, data []string) error

func (r *Syncer) doHandler(ctx context.Context, items *[]string, handler SubscriberHandler) {
	for _, item := range *items {
		err := handler(ctx, item)
		if err != nil {
			su_logger.Error(ctx, err, "failed to handle message "+item)
		} else if r.opt.LogLevel < zapcore.ErrorLevel {
			su_logger.Info(ctx, "l2sync "+item)
		}
	}

	*items = (*items)[:0]
}

func (r *Syncer) doBatchHandler(ctx context.Context, items *[]string, handler BatchSubscriberHandler) {
	if len(*items) == 0 {
		return
	}

	// 传递副本，避免外部修改影响内部状态
	batch := make([]string, len(*items))
	copy(batch, *items)

	err := handler(ctx, batch)
	if err != nil {
		su_logger.Error(ctx, err, "failed to handle batch messages")
	} else if r.opt.LogLevel < zapcore.ErrorLevel {
		su_logger.Info(ctx, "l2sync batch", su_logger.E().Int("count", len(batch)))
	}

	*items = (*items)[:0]
}

// @deprecated 使用 SubscribeBatch 替代
func (r *Syncer) Subscribe(ctx context.Context, handler SubscriberHandler) error {
	messages := make([]string, 0, r.opt.BatchSize)
	ticker := time.NewTicker(r.opt.BatchInterval)
	defer ticker.Stop()

	ch, err := r.cli.Subscribe(ctx, r.opt.Topic)
	if err != nil {
		su_logger.Error(ctx, err, "failed to subscribe")
		return err
	}
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				// Channel closed, process remaining messages
				su_logger.Error(ctx, err, "channel closed")
				r.doHandler(ctx, &messages, handler)
				return nil
			}
			if msg == "" {
				continue
			}

			// 解析消息，提取发送者 ID 和实际数据
			// 消息格式: {instanceID}|{data}
			senderID, data := r.parseMessage(msg)

			// 过滤自己发送的消息
			if senderID != "" && senderID == r.instanceID {
				continue
			}

			// 去重：仅跳过连续重复的消息，保留时序
			// vA -> vB -> vA => [vA, vB, vA] ✓
			// vA -> vA -> vA => [vA] ✓
			if len(messages) == 0 || messages[len(messages)-1] != data {
				messages = append(messages, data)
			}
			if len(messages) >= r.opt.BatchSize {
				r.doHandler(ctx, &messages, handler)
			}
		case <-ticker.C:
			r.doHandler(ctx, &messages, handler)
		case <-ctx.Done():
			return nil
		}
	}
}

// SubscribeBatch 订阅消息（批量处理模式）
// 在 BatchInterval 时间窗口内收集消息，然后批量传递给 handler
// 去重策略：仅跳过连续重复的消息，保留时序
func (r *Syncer) SubscribeBatch(ctx context.Context, handler BatchSubscriberHandler) error {
	messages := make([]string, 0, r.opt.BatchSize)
	ticker := time.NewTicker(r.opt.BatchInterval)
	defer ticker.Stop()

	ch, err := r.cli.Subscribe(ctx, r.opt.Topic)
	if err != nil {
		su_logger.Error(ctx, err, "failed to subscribe")
		return err
	}
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				// Channel closed, process remaining messages
				su_logger.Error(ctx, err, "channel closed")
				r.doBatchHandler(ctx, &messages, handler)
				return nil
			}
			if msg == "" {
				continue
			}

			senderID, data := r.parseMessage(msg)
			if senderID != "" && senderID == r.instanceID {
				continue
			}

			// 去重：仅跳过连续重复的消息，保留时序
			if len(messages) == 0 || messages[len(messages)-1] != data {
				messages = append(messages, data)
			}
			if len(messages) >= r.opt.BatchSize {
				r.doBatchHandler(ctx, &messages, handler)
			}
		case <-ticker.C:
			r.doBatchHandler(ctx, &messages, handler)
		case <-ctx.Done():
			return nil
		}
	}
}

// parseMessage 解析消息，返回发送者 ID 和实际数据
func (r *Syncer) parseMessage(msg string) (senderID, data string) {
	idx := strings.Index(msg, msgSeparator)
	if idx == -1 {
		// 兼容旧格式消息（无发送者 ID）
		return "", msg
	}
	return msg[:idx], msg[idx+1:]
}
