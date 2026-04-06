package limiter

import (
	"context"
	"github.com/patrickmn/go-cache"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
	"math"
	"time"
)

type TokenStat struct {
	// 上一次时间窗口
	LastTimeMs int64
	// 最大令牌数
	Burst float64
	// 速度
	SpeedInterval float64
	// 周期
	PeriodMs float64
	// 当前存储的数量
	Stored float64
}

type LocalLimiter struct {
	baseLimiter
	sourceCache *cache.Cache
}

func NewLocalLimiter() *LocalLimiter {
	return &LocalLimiter{}
}

func (t *LocalLimiter) JustTake(source string, rule *uriGroup) (ok bool, err error) {
	//TODO implement me
	cacheData, exist := t.sourceCache.Get(source)
	var stat *TokenStat
	if !exist {
		t.lock.Lock()
		defer t.lock.Unlock()

		stat = &TokenStat{
			LastTimeMs: 0,
			Burst:      rule.Burst,
			PeriodMs:   rule.PeriodSec * 1000,
			Stored:     0,
		}
		stat.SpeedInterval = stat.Burst / stat.PeriodMs
	} else {
		stat = cacheData.(*TokenStat)
	}

	var timeMs = time.Now().UnixMilli()
	if timeMs > stat.LastTimeMs {
		newToken := float64(timeMs-stat.LastTimeMs) * stat.SpeedInterval
		su_logger.Info(nil, "new token", su_logger.E().Float64("new", newToken))
		stat.Stored = math.Min(stat.Burst, newToken+stat.Stored)
		stat.LastTimeMs = timeMs
	}
	su_logger.Info(nil, "stat", su_logger.E().Any("stat", stat))
	// store 可能是小数, 如 0.01
	if stat.Stored >= 1 {
		ok = true
		stat.Stored -= 1
	} else {
		err = SourceBlocked
	}

	t.sourceCache.SetDefault(source, stat)

	return ok, err
}

func (t *LocalLimiter) Init(cnf *Config) error {
	//TODO implement me
	t.sourceCache = cache.New(time.Second*30, time.Minute*5)
	err := t.constructor(cnf)
	if err != nil {
		return err
	}

	return nil
}

func (t *LocalLimiter) Take(ctx context.Context, uri string, source string) (entry *Entry, err error) {
	group, exists := t.uriMapping[uri]
	if !exists {
		return nil, SourceLimiterNotDefined
	}
	// 判断当前 source 是否已定义限流
	key := t.getKey(uri, source)

	_, err = t.JustTake(key, group)
	if err != nil {
		if group.ErrController != nil {
			group.ErrController(ctx, key, err)
		}
		return nil, err
	} else {
		return &Entry{
			tpe:   TypeLocal,
			entry: nil,
		}, nil
	}
}
