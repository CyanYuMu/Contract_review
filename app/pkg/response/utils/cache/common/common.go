package common

import (
	"context"
	"errors"
	"time"
)

type LockExecParam struct {
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

type locker interface {
	Lock(k string, v string, ttl time.Duration) (ok bool, err error)
	Unlock(k string, randVal string) (ok bool, err error)
	Expire(k string, ttl time.Duration) (ok bool, err error)
}

const minFreq = time.Millisecond * 10
const minTTL = time.Second

var FailToGetLock = errors.New("failed to get lock")

func LockExec(inst interface{}, param LockExecParam) (err error) {
	// 参数校验
	if param.Freq < minFreq {
		param.Freq = minFreq
	}

	if param.TTL < minTTL {
		param.TTL = minTTL
	}

	var ok bool
	var l locker

	if l, ok = inst.(locker); !ok {
		return errors.New("unsupported inst")
	}

	if param.TryLock {
		ok, err = l.Lock(param.Key, param.Val, param.TTL)
		if err != nil {
			return err
		}

		if ok {
			defer func() {
				if !param.NotReleaseLock {
					_, err1 := l.Unlock(param.Key, param.Val)
					if err == nil {
						err = err1
					}
				}
			}()
			// 获取锁成功
			if param.EnableWatchDog {
				watchDogCtx, cancel := context.WithCancel(context.Background())
				defer cancel()
				watchDog(watchDogCtx, l, param.Key, param.TTL)
			}
			return param.Func()
		}
	} else {
		var timeTo time.Time
		if param.Duration > 0 {
			timeTo = time.Now().Add(param.Duration)
		}

		for param.Duration <= 0 || time.Now().Before(timeTo) {
			ok, err = l.Lock(param.Key, param.Val, param.TTL)
			if err != nil {
				return err
			}
			if !ok {
				time.Sleep(param.Freq)
				continue
			}
			// 上锁成功, 跳出循环
			break
		}

		defer func() {
			if !param.NotReleaseLock {
				_, err1 := l.Unlock(param.Key, param.Val)
				if err == nil {
					err = err1
				}
			}
		}()
		// 获取锁成功
		if param.EnableWatchDog {
			// 创建一个子context，用于在Func执行完成后取消watchDog
			watchDogCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			watchDog(watchDogCtx, l, param.Key, param.TTL)
		}

		return param.Func()
	}

	return FailToGetLock
}

const maxWatchDogInterval = time.Second * 60

func watchDog(ctx context.Context, l locker, lockKey string, ttl time.Duration) {
	freq := ttl - time.Second*2
	if freq <= 0 {
		return
	}

	if freq > maxWatchDogInterval {
		freq = maxWatchDogInterval
	}

	go func(ctx context.Context) {
		for {
			select {
			case <-time.After(freq):
				_, _ = l.Expire(lockKey, ttl)
			case <-ctx.Done():
				return
			}
		}
	}(ctx)
}

func GetCacheKey(prefix, k string) string {
	if prefix != "" {
		k = prefix + ":" + k
	}

	//if len(k) > 64 {
	//	t := sha1.New()
	//	_, _ = io.WriteString(t, k)
	//	k = fmt.Sprintf("%x", t.Sum(nil))
	//}

	return k
}
