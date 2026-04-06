package syncer

import (
	"context"

	"github.com/redis/go-redis/v9"
)

type RedisV9 struct {
	cli redis.UniversalClient
}

func (r *RedisV9) Publish(ctx context.Context, topic string, msg string) error {
	return r.cli.Publish(ctx, topic, msg).Err()
}

func (r *RedisV9) Subscribe(ctx context.Context, channels string) (<-chan string, error) {
	pubsub := r.cli.Subscribe(ctx, channels)
	ch := make(chan string)
	go func() {
		for msg := range pubsub.Channel() {
			ch <- msg.Payload
		}
	}()
	return ch, nil
}

func NewRedisV9(cli redis.UniversalClient) Synchronizer {
	return &RedisV9{
		cli: cli,
	}
}
