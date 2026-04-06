package qwrobot

import "context"

type Limiter interface {
	Count(ctx context.Context, key string, ttlSec int) (count int, err error)
	Del(ctx context.Context, key string)
}
