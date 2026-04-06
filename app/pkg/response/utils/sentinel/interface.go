package sentinel

import (
	"context"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/sentinel/limiter"
)

type Limiter interface {
	Init(cfg *limiter.Config) error
	Take(ctx context.Context, uri string, source string) (entry *limiter.Entry, err error)
	IsTrustedIp(ip string) bool
}

type LimiterBase interface {
	Taker(input interface{}, domain string, key string) string
}

func NewLocalLimiter() Limiter {
	return limiter.NewLocalLimiter()
}

func NewDistributedLimiter() Limiter {
	return limiter.NewDistributedLimiter()
}

func NewDistributedLimiterV8() Limiter {
	return limiter.NewDistributedSentinelV8()
}

func NewDistributedLimiterV9() *limiter.DistributedSentinelV9 {
	return limiter.NewDistributedSentinelV9()
}

func NewAliLimiter() Limiter {
	return limiter.NewAliLocalLimiter()
}
