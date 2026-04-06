package driver

import (
	"github.com/go-redis/redis/v8"
	error2 "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/error"
)

var ErrorTimeout = error2.NewSUError(500500, "get token timeout")

type TokenBucketConf struct {
	// 必填, redis key
	Key string
	// 必填, 令牌桶的容量
	Capacity uint64
	// 必填, 每个周期产生的令牌数
	NumPerPeriod uint64
	// 可选,周期, 单位秒, 默认1秒
	Period uint32
	// 必填, redis cli
	Redis redis.UniversalClient
}

// StickyWindowConf
// @Description: 固定窗口配置
type StickyWindowConf struct {
	// 必填, redis key
	Key string
	// 必填, 每个周期产生的令牌数
	NumPerPeriod uint64
	// 可选,周期, 单位秒, 默认1秒
	Period uint32
	// 必填, redis cli
	Redis redis.UniversalClient
}
