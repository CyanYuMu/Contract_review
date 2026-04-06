package cache

import (
	"context"
	"strings"
	"time"

	gocache "github.com/patrickmn/go-cache"
)

type GoCache struct {
	*Base
	shards []*gocache.Cache
}

// getShard returns the appropriate shard for a given key
func (g *GoCache) getShard(k string) *gocache.Cache {
	if len(g.shards) == 1 {
		return g.shards[0]
	}

	// 直接使用 fnv-1a 算法的核心逻辑计算哈希值，避免每次创建新的哈希对象
	// fnv-1a 的计算方式: hash = offset32, 然后对每个字节: hash = (hash ^ byte) * prime32
	hash := uint32(2166136261) // offset32 常量
	for i := 0; i < len(k); i++ {
		hash ^= uint32(k[i])
		hash *= 16777619 // prime32 常量
	}
	return g.shards[hash%uint32(len(g.shards))]
}

// NewGoCache creates a new sharded GoCache instance
func NewGoCache(cnf *GoCacheConfig) GoCacheCmd {
	if cnf.ShardNum < 1 {
		cnf.ShardNum = 1
	}

	shards := make([]*gocache.Cache, cnf.ShardNum)
	for i := 0; i < cnf.ShardNum; i++ {
		shards[i] = gocache.New(time.Duration(cnf.DefaultTtlSec)*time.Second, time.Duration(cnf.CleanIntervalSec)*time.Second)
	}

	return &GoCache{
		Base:   &Base{},
		shards: shards,
	}
}

func (g *GoCache) Set(ctx context.Context, k string, v interface{}, ttl time.Duration, opt ...*StringSetOption) (ok bool, err error) {
	k = g.GetKey(k)
	g.getShard(k).Set(k, v, ttl)

	return true, nil
}

func (g *GoCache) Get(ctx context.Context, k string) (val interface{}, err error) {
	k = g.GetKey(k)
	val, _ = g.getShard(k).Get(k)

	return val, nil
}

func (g *GoCache) Del(ctx context.Context, k string) (ok bool, err error) {
	k = g.GetKey(k)
	g.getShard(k).Delete(k)

	return true, nil
}

func (g *GoCache) Exists(ctx context.Context, k string) (exists bool, err error) {
	k = g.GetKey(k)
	_, exists = g.getShard(k).Get(k)

	return
}

func (g *GoCache) Expire(ctx context.Context, k string, ttl time.Duration) (ok bool, err error) {
	k = g.GetKey(k)
	v, exists := g.getShard(k).Get(k)
	if exists {
		g.getShard(k).Set(k, v, ttl)
	}

	return
}

func (g *GoCache) incrAutoFill(ctx context.Context, k string, n int64, err error) bool {
	k = g.GetKey(k)
	if strings.Contains(err.Error(), "not found") {
		g.getShard(k).Set(k, n, 0)

		return true
	}

	return false
}

func (g *GoCache) Incr(ctx context.Context, k string) (n int64, err error) {
	k = g.GetKey(k)
	n, err = g.getShard(k).IncrementInt64(k, 1)
	if err != nil {
		ok := g.incrAutoFill(ctx, k, 1, err)
		if ok {
			n = 1
			err = nil
		}
	}
	return
}

func (g *GoCache) IncrBy(ctx context.Context, k string, v int64) (n int64, err error) {
	k = g.GetKey(k)
	n, err = g.getShard(k).IncrementInt64(k, v)
	if err != nil {
		ok := g.incrAutoFill(ctx, k, v, err)
		if ok {
			n = v
			err = nil
		}
	}
	return
}

func (g *GoCache) Decr(ctx context.Context, k string) (n int64, err error) {
	k = g.GetKey(k)
	n, err = g.getShard(k).DecrementInt64(k, 1)
	if err != nil {
		ok := g.incrAutoFill(ctx, k, -1, err)
		if ok {
			n = -1
			err = nil
		}
	}

	return
}

func (g *GoCache) DecrBy(ctx context.Context, k string, v int64) (n int64, err error) {
	k = g.GetKey(k)
	n, err = g.getShard(k).DecrementInt64(k, v)
	if err != nil {
		ok := g.incrAutoFill(ctx, k, -v, err)
		if ok {
			n = -v
			err = nil
		}
	}

	return
}

func (g *GoCache) TTL(ctx context.Context, k string) (ttl time.Duration, err error) {
	k = g.GetKey(k)
	_, t, exists := g.getShard(k).GetWithExpiration(k)
	if !exists {
		ttl = -2
	}

	interval := int(t.Sub(time.Now()))
	if interval < 0 {
		ttl = -1
	}

	return ttl, nil
}
