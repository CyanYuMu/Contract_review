package mq

import (
	"context"
)

type MQ interface {
	NewProducer(ctx context.Context, conf ProducerConf) (Producer, error)
	NewConsumer(ctx context.Context, conf ConsumerConf) (Consumer, error)
}
