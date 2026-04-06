package qwrobot

import (
	"context"
	"time"

	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/cache/v3"
)

type RedisLimiter struct {
	redisCli cache.Rds
}

func NewRedisLimiter(cli cache.Rds) Limiter {
	return &RedisLimiter{redisCli: cli}
}

func (r *RedisLimiter) Count(ctx context.Context, key string, ttlSec int) (count int, err error) {
	cnt, err := r.redisCli.Incr(ctx, key)
	if err != nil {
		return
	}
	if cnt == 1 && ttlSec > 0 {
		r.redisCli.Expire(ctx, key, time.Duration(ttlSec)*time.Second)
	}

	return int(cnt), nil
}

func (r *RedisLimiter) Del(ctx context.Context, key string) {
	r.redisCli.Del(ctx, key)
}
