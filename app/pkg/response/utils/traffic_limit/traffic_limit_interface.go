package traffic_limit

import (
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/traffic_limit/driver"
	"time"
)

type TrafficLimitInterface interface {
	// Init
	// @Description: 配置初始化, 对关键参数进行校验
	// @return error
	Init() error
	// Acquire
	// @Description: 获取令牌, timeout = 0 会一直尝试获取[不推荐], timeout > 0 会在超时时间内尝试获取
	// @param timeout
	// @return bool
	// @return error
	Acquire(timeout time.Duration) (bool, error)
	// TryAcquire
	// @Description: 尝试获取令牌
	// @return timeToWait 需要等待的时间
	// @return ok
	// @return err
	TryAcquire() (timeToWait time.Duration, ok bool, err error)
	// 一直等待
	Wait() error
}

// NewTokenBucket
// @description 令牌桶
func NewTokenBucket(conf *driver.TokenBucketConf) (TrafficLimitInterface, error) {
	t := &driver.TokenBucket{
		Conf: conf,
	}

	err := t.Init()

	return t, err
}

// NewTokenBucketV9 令牌桶（v9 版本）
func NewTokenBucketV9(conf *driver.TokenBucketConfV9) (TrafficLimitInterface, error) {
	t := &driver.TokenBucketV9{
		Conf: conf,
	}
	err := t.Init()
	return t, err
}

// NewStickyWindow
// @description 固定窗口
func NewStickyWindow(conf *driver.StickyWindowConf) (TrafficLimitInterface, error) {
	w := &driver.StickyWindow{
		Conf: conf,
	}
	err := w.Init()

	return w, err
}
