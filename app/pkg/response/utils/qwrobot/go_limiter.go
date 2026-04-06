package qwrobot

import (
	"context"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/cache/v3"
	"time"
)

type GoCacheLimiter struct {
	cli cache.GoCacheCmd
}

func (g *GoCacheLimiter) Count(ctx context.Context, key string, ttlSec int) (count int, err error) {
	n, _ := g.cli.Incr(ctx, key)
	if n <= 1 {
		// 设置过期时间
		g.cli.Expire(ctx, key, time.Duration(ttlSec)*time.Second)
	}

	return int(n), nil
}

func (g *GoCacheLimiter) Del(ctx context.Context, key string) {
	g.cli.Del(ctx, key)
}

func NewGoLimiter(limiter cache.GoCacheCmd) Limiter {
	return &GoCacheLimiter{cli: limiter}
}
