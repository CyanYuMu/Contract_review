package driver

import (
	"hash/fnv"
	"strings"
	"time"

	"github.com/go-redis/redis"
	goCache "github.com/patrickmn/go-cache"
	"github.com/spf13/cast"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/cache/common"
	error2 "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/error"
)

type GoCacheConfig struct {
	// 单位秒
	DefaultTtlSec int `yaml:"defaultTtlSec"`
	// 单位秒
	CleanIntervalSec int `yaml:"cleanIntervalSec"`
	// 分片数量，默认为1
	ShardNum int `yaml:"shardNum"`
}

type GoCache struct {
	shards []*goCache.Cache
}

func NewGoCache(conf GoCacheConfig) *GoCache {
	if conf.ShardNum <= 0 {
		conf.ShardNum = 1
	}

	shards := make([]*goCache.Cache, conf.ShardNum)
	for i := 0; i < conf.ShardNum; i++ {
		shards[i] = goCache.New(time.Duration(conf.DefaultTtlSec)*time.Second, time.Duration(conf.CleanIntervalSec)*time.Second)
	}

	return &GoCache{
		shards: shards,
	}
}

// getShard returns the appropriate shard for a given key
func (g *GoCache) getShard(k string) *goCache.Cache {
	if len(g.shards) == 1 {
		return g.shards[0]
	}

	h := fnv.New32a()
	h.Write([]byte(k))
	return g.shards[h.Sum32()%uint32(len(g.shards))]
}

func (g *GoCache) Set(k string, v interface{}, ttl time.Duration) (ok bool, err error) {
	g.getShard(k).Set(k, v, ttl)

	return true, nil
}

func (g *GoCache) Get(k string) (val interface{}, err error) {
	val, _ = g.getShard(k).Get(k)

	return val, nil
}

func (g *GoCache) Del(k string) (ok bool, err error) {
	g.getShard(k).Delete(k)

	return true, nil
}

func (g *GoCache) Exists(k string) (exists bool, err error) {
	_, exists = g.getShard(k).Get(k)

	return
}

func (g *GoCache) Expire(k string, ttl time.Duration) (ok bool, err error) {
	v, exists := g.getShard(k).Get(k)
	if exists {
		g.getShard(k).Set(k, v, ttl)
	}

	return
}

func (g *GoCache) incrAutoFill(k string, n int64, err error) bool {
	if strings.Contains(err.Error(), "not found") {
		g.Set(k, n, 0)

		return true
	}

	return false
}

func (g *GoCache) Incr(k string) (n int64, err error) {
	n, err = g.getShard(k).IncrementInt64(k, 1)
	if err != nil {
		ok := g.incrAutoFill(k, 1, err)
		if ok {
			n = 1
			err = nil
		}
	}
	return
}

func (g *GoCache) IncrBy(k string, v int64) (n int64, err error) {
	n, err = g.getShard(k).IncrementInt64(k, v)
	if err != nil {
		ok := g.incrAutoFill(k, v, err)
		if ok {
			n = v
			err = nil
		}
	}
	return
}

func (g *GoCache) Decr(k string) (n int64, err error) {
	n, err = g.getShard(k).DecrementInt64(k, 1)
	if err != nil {
		ok := g.incrAutoFill(k, -1, err)
		if ok {
			n = -1
			err = nil
		}
	}

	return
}

func (g *GoCache) DecrBy(k string, v int64) (n int64, err error) {
	n, err = g.getShard(k).DecrementInt64(k, v)
	if err != nil {
		ok := g.incrAutoFill(k, -v, err)
		if ok {
			n = -v
			err = nil
		}
	}

	return
}

func (g *GoCache) TTL(k string) (ttl time.Duration, err error) {
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

func (g *GoCache) Lock(k string, v string, ttl time.Duration) (ok bool, err error) {
	//TODO implement me
	err = g.getShard(k).Add(k, v, ttl)
	if err == nil {
		ok = true
	}

	return
}

func (g *GoCache) Unlock(k string, randVal string) (ok bool, err error) {
	curV, exists := g.getShard(k).Get(k)
	if exists {
		if randVal == curV {
			g.getShard(k).Delete(k)
			ok = true
		}
	} else {
		ok = true
	}

	return
}

func (g *GoCache) LockExec(param common.LockExecParam) (err error) {
	return common.LockExec(g, param)
}

func (g *GoCache) GetInst() interface{} {
	//TODO implement me
	return g.shards
}

func (g *GoCache) HGetAll(key string) (data map[string]string, err error) {
	val, exists := g.getShard(key).Get(key)
	if !exists {
		return nil, redis.Nil
	}

	if m, ok := val.(map[string]string); ok {
		return m, nil
	}

	if m, ok := val.(map[string]interface{}); ok {
		data = make(map[string]string, len(m))
		for k, v := range m {
			data[k] = cast.ToString(v)
		}
		return data, nil
	}

	return nil, error2.NewSUError(500401, "不支持的数据类型")
}

func (g *GoCache) HGet(key string, field string) (rs string, err error) {
	cacheData, err := g.HGetAll(key)
	if err != nil {
		return "", err
	}
	data, _ := cacheData[field]

	return data, nil
}

func (g *GoCache) HMGet(key string, fields []string) (rs interface{}, err error) {
	cacheData, err := g.HGetAll(key)
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

func (g *GoCache) HDel(key string, fields []string) (err error) {
	cacheData, err := g.HGetAll(key)
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

	err = g.HMSet(key, newCacheData)

	return nil
}

func (g *GoCache) HMSet(key string, data map[string]interface{}) (err error) {
	g.getShard(key).Set(key, data, time.Hour*4)

	return nil
}
