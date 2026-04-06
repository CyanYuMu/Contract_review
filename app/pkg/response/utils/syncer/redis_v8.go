package syncer

import (
	"context"

	"github.com/go-redis/redis/v8"
)

type RedisV8 struct {
	cli redis.UniversalClient
}

func (r *RedisV8) Publish(ctx context.Context, topic string, msg string) error {
	return r.cli.Publish(ctx, topic, msg).Err()
}

func (r *RedisV8) Subscribe(ctx context.Context, channel string) (<-chan string, error) {
	pubsub := r.cli.Subscribe(ctx, channel)
	ch := make(chan string)
	go func() {
		for msg := range pubsub.Channel() {
			ch <- msg.Payload
		}
	}()
	return ch, nil
}

func NewRedisV8(cli redis.UniversalClient) Synchronizer {
	return &RedisV8{
		cli: cli,
	}
}
