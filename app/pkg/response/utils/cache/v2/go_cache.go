package cache

import (
	"context"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	gocache "github.com/patrickmn/go-cache"
	"github.com/spf13/cast"
	error2 "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/error"
)

type GoCache struct {
	*Base
	cli *gocache.Cache
}

type GoCacheConfig struct {
	// 单位秒
	DefaultTtlSec int `yaml:"defaultTtlSec"`
	// 单位秒
	CleanIntervalSec int `yaml:"cleanIntervalSec"`
}

func NewGoCache(cnf *GoCacheConfig) *GoCache {
	c := gocache.New(time.Duration(cnf.DefaultTtlSec)*time.Second, time.Duration(cnf.CleanIntervalSec)*time.Second)

	return &GoCache{
		Base: &Base{},
		cli:  c,
	}
}

func (g *GoCache) Set(ctx context.Context, k string, v interface{}, ttl time.Duration) (ok bool, err error) {
	k = g.GetKey(k)
	g.cli.Set(k, v, ttl)

	return true, nil
}

func (g *GoCache) Get(ctx context.Context, k string) (val interface{}, err error) {
	k = g.GetKey(k)
	val, _ = g.cli.Get(k)

	return val, nil
}

func (g *GoCache) Del(ctx context.Context, k string) (ok bool, err error) {
	//TODO implement me
	k = g.GetKey(k)
	g.cli.Delete(k)

	return true, nil
}

func (g *GoCache) Exists(ctx context.Context, k string) (exists bool, err error) {
	k = g.GetKey(k)
	_, exists = g.cli.Get(k)

	return
}

func (g *GoCache) Expire(ctx context.Context, k string, ttl time.Duration) (ok bool, err error) {
	k = g.GetKey(k)
	v, exists := g.cli.Get(k)
	if exists {
		g.cli.Set(k, v, ttl)
	}

	return
}

func (g *GoCache) incrAutoFill(ctx context.Context, k string, n int64, err error) bool {
	k = g.GetKey(k)
	if strings.Contains(err.Error(), "not found") {
		g.cli.Set(k, n, 0)

		return true
	}

	return false
}

func (g *GoCache) Incr(ctx context.Context, k string) (n int64, err error) {
	n, err = g.cli.IncrementInt64(k, 1)
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
	n, err = g.cli.IncrementInt64(k, v)
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
	n, err = g.cli.DecrementInt64(k, 1)
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
	n, err = g.cli.DecrementInt64(k, v)
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
	_, t, exists := g.cli.GetWithExpiration(k)
	if !exists {
		ttl = -2
	}

	interval := int(t.Sub(time.Now()))
	if interval < 0 {
		ttl = -1
	}

	return ttl, nil
}

func (g *GoCache) Lock(ctx context.Context, k string, v string, ttl time.Duration) (ok bool, err error) {
	//TODO implement me
	err = g.cli.Add(k, v, ttl)
	if err == nil {
		ok = true
	}

	return
}

func (g *GoCache) Unlock(ctx context.Context, k string, ranVal string) (ok bool, err error) {
	curV, exists := g.cli.Get(k)
	if exists {
		if ranVal == curV {
			g.cli.Delete(k)
			ok = true
		}
	} else {
		ok = true
	}

	return
}

func (g *GoCache) Client() *gocache.Cache {
	//TODO implement me
	return g.cli
}

func (g *GoCache) HGetAll(ctx context.Context, key string) (data map[string]string, err error) {
	cacheData, exists := g.cli.Get(key)
	if !exists {
		return nil, redis.Nil
	}

	cacheMapData, ok := cacheData.(map[string]interface{})
	if !ok {
		return nil, error2.NewSUError(500402, "类型转换时发生错误, 期望类型 map[string]interface{} 类型")
	}
	data = make(map[string]string, len(cacheMapData))
	for k, v := range cacheMapData {
		data[k] = cast.ToString(v)
	}

	return data, nil
}

func (g *GoCache) HGet(ctx context.Context, key string, field string) (rs string, err error) {
	cacheData, err := g.HGetAll(ctx, key)
	if err != nil {
		return "", err
	}
	data, _ := cacheData[field]

	return data, nil
}

func (g *GoCache) HMGet(ctx context.Context, key string, fields []string) (rs interface{}, err error) {
	cacheData, err := g.HGetAll(ctx, key)
	if err != nil {
		return nil, err
	}

	rsData := make(map[string]string, len(fields))
	for _, field := range fields {
		if _, exists := cacheData[field]; !exists {
			rsData[field] = cacheData[field]
		}
	}

	return rsData, nil
}

func (g *GoCache) HDel(ctx context.Context, key string, fields []string) (err error) {
	cacheData, err := g.HGetAll(ctx, key)
	if err != nil {
		return err
	}

	for _, field := range fields {
		delete(cacheData, field)
	}

	newCacheData := make(map[string]interface{}, len(cacheData))
	for k, v := range cacheData {
		newCacheData[k] = v
	}

	ttl, _ := g.TTL(ctx, key)
	if ttl < 0 {
		ttl = 0
	}

	_, err = g.HMSet(ctx, key, newCacheData, ttl)

	return nil
}

func (g *GoCache) HMSet(ctx context.Context, key string, data map[string]interface{}, ttl time.Duration) (ok bool, err error) {
	g.cli.Set(key, data, ttl)

	return true, nil
}
