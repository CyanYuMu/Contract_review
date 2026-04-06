package driver

import (
	"context"
	"errors"
	"time"
)

var ErrUnlockValNotMatch = errors.New("unlock failed value not match")

const EmptyString = "_empty_"

type WatchDog interface {
	//Lock(ctx context.Context, k string, v string, ttl time.Duration) (ok bool, err error)
	//Unlock(ctx context.Context, k string, ranVal string) (ok bool, err error)
	Expire(ctx context.Context, k string, ttl time.Duration) (ok bool, err error)
}

func watchDog(ctx context.Context, l WatchDog, lockKey string, ttl time.Duration) {
	// 提前2秒去设置过期时间
	freq := ttl - time.Second*2
	if freq <= 0 {
		return
	}

	go func(ctx context.Context) {
		for {
			select {
			case <-time.After(freq):
				_, _ = l.Expire(ctx, lockKey, ttl)
			case <-ctx.Done():
				return
			}
		}
	}(ctx)
}
