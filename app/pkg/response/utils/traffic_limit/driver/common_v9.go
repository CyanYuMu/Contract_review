package driver

import (
	redis "github.com/redis/go-redis/v9"
)

// TokenBucketConfV9 令牌桶配置（v9 版本）
type TokenBucketConfV9 struct {
	Key          string
	Capacity     uint64
	NumPerPeriod uint64
	Period       uint32
	Redis        redis.UniversalClient
}

// ToV8 转换为 v8 版本的配置（内部使用 v9 客户端的适配）
// 注意：这个方法不可用，因为 v8 和 v9 的 UniversalClient 接口不兼容。
// 使用 NewTokenBucketV9 代替。

// StickyWindowConfV9 固定窗口配置（v9 版本）
type StickyWindowConfV9 struct {
	Key          string
	NumPerPeriod uint64
	Period       uint32
	Redis        redis.UniversalClient
}
