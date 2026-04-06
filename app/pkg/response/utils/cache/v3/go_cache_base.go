package cache

import (
	"context"
	"time"
)

type GoCacheCmd interface {
	Set(ctx context.Context, k string, v interface{}, ttl time.Duration, opt ...*StringSetOption) (ok bool, err error)
	Get(ctx context.Context, k string) (val interface{}, err error)
	Del(ctx context.Context, k string) (ok bool, err error)
	Exists(ctx context.Context, k string) (exists bool, err error)
	Expire(ctx context.Context, k string, ttl time.Duration) (ok bool, err error)
	Incr(ctx context.Context, k string) (n int64, err error)
	IncrBy(ctx context.Context, k string, v int64) (n int64, err error)
	Decr(ctx context.Context, k string) (n int64, err error)
	DecrBy(ctx context.Context, k string, v int64) (n int64, err error)
	TTL(ctx context.Context, k string) (ttl time.Duration, err error)
}

type GoCacheConfig struct {
	// 单位秒
	DefaultTtlSec int `yaml:"defaultTtlSec"`
	// 单位秒
	CleanIntervalSec int `yaml:"cleanIntervalSec"`
	// 分片数, 默认是1
	ShardNum int `yaml:"shardNum"`
}
