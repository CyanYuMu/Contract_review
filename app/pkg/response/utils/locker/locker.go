package locker

import (
	"context"
	"errors"
	"time"
)

type Param struct {
	// 锁定的key
	Key string
	// 随机值
	Val string
	// 轮询间隔
	Freq time.Duration
	// 加锁成功后, key的过期时间, 保险机制
	TTL time.Duration
	// 是否是尝试加锁, true => 获取锁失败直接放弃执行
	TryLock bool
	// 尝试获取锁的时间, 0 => 直到获取锁成功, 大于0 => 尝试获取锁的最大时长
	Duration time.Duration
	// 获取锁成功后, 调用的方法
	Func func() error
	// 是否启动看门狗, 防止锁在业务执行期间被释放掉
	EnableWatchDog bool
	// 不释放锁, 默认false 即自动释放
	NotReleaseLock bool
}

type Locker interface {
	Lock(ctx context.Context, k string, v string, ttl time.Duration) (ok bool, err error)
	Unlock(ctx context.Context, k string, ranVal string) (ok bool, err error)
	Expire(ctx context.Context, k string, ttl time.Duration) (ok bool, err error)
}

const minFreq = time.Millisecond * 10
const minTTL = time.Second

var ErrFailToGetLock = errors.New("failed to get lock")

func Exec(ctx context.Context, inst Locker, param Param) (err error) {
	// 参数校验
	if param.Freq < minFreq {
		param.Freq = minFreq
	}

	if param.TTL < minTTL {
		param.TTL = minTTL
	}

	var ok bool

	if param.TryLock {
		ok, err = inst.Lock(ctx, param.Key, param.Val, param.TTL)
		if err != nil {
			return err
		}

	} else {
		var timeTo time.Time
		if param.Duration > 0 {
			timeTo = time.Now().Add(param.Duration)
		}

		for param.Duration <= 0 || time.Now().Before(timeTo) {
			ok, err = inst.Lock(ctx, param.Key, param.Val, param.TTL)
			if err != nil {
				return err
			}
			if !ok {
				// 获取锁失败, 等待一段时间后重试
				time.Sleep(param.Freq)
				continue
			} else {
				break
			}
		}
	}

	if ok {
		defer func() {
			if !param.NotReleaseLock {
				_, err1 := inst.Unlock(ctx, param.Key, param.Val)
				if err == nil {
					err = err1
				}
			}
		}()
		// 获取锁成功
		if param.EnableWatchDog {
			// 创建一个子context，用于在Func执行完成后取消watchDog
			watchDogCtx, cancel := context.WithCancel(ctx)
			defer cancel()

			watchDog(watchDogCtx, inst, param.Key, param.TTL)
		}
		return param.Func()
	}

	return ErrFailToGetLock
}

func watchDog(ctx context.Context, l Locker, lockKey string, ttl time.Duration) {
	// 提前2秒去设置过期时间
	var freq = time.Second * 5
	if ttl > time.Minute {
		freq = ttl - time.Second*5
	} else if ttl > time.Second*10 {
		freq = ttl - time.Second*2
	}

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
