package driver

import (
	"errors"
	"strings"
	"time"

	"github.com/go-redis/redis"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/cache/common"
)

type RedisCacheConfig struct {
	Prefix   string
	Addrs    []string
	Password string
	// 单节点模式生效
	DB            int
	PoolSize      int
	IsCluster     bool
	RouteRandomly bool
	ReadOnly      bool
	MinIdleConn   int

	DialTimeoutSec  int
	ReadTimeoutSec  int
	WriteTimeoutSec int

	// PoolFIFO uses FIFO mode for each node connection pool GET/PUT (default LIFO).
	PoolFIFO bool
	// 最大生命周期, 超过释放回收
	MaxConnAgeSec int
	// 连接池最大超时时间, 超过重置
	PoolTimeoutSec int
	// 最大空闲时间, 超过释放
	IdleTimeoutSec int
	// 检测空闲连接频率, 默认一分钟
	IdleCheckFrequencySec int
}

func NewRedisCache(conf RedisCacheConfig) *RedisCache {
	var inst redis.UniversalClient
	dialTimeout := time.Duration(conf.DialTimeoutSec) * time.Second
	ReadTimeout := time.Duration(conf.ReadTimeoutSec) * time.Second
	WriteTimeout := time.Duration(conf.WriteTimeoutSec) * time.Second

	if conf.IsCluster {
		inst = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:         conf.Addrs,
			Password:      conf.Password,
			PoolSize:      conf.PoolSize,
			RouteRandomly: conf.RouteRandomly,
			MinIdleConns:  conf.MinIdleConn,
			DialTimeout:   dialTimeout,
			ReadTimeout:   ReadTimeout,
			WriteTimeout:  WriteTimeout,
		})
	} else {
		inst = redis.NewClient(&redis.Options{
			Addr:         conf.Addrs[0],
			Password:     conf.Password,
			DB:           conf.DB,
			PoolSize:     conf.PoolSize,
			MinIdleConns: conf.MinIdleConn,
			DialTimeout:  dialTimeout,
			ReadTimeout:  ReadTimeout,
			WriteTimeout: WriteTimeout,
		})
	}

	return &RedisCache{
		prefix: conf.Prefix,
		inst:   inst,
	}
}

type RedisCache struct {
	// key前缀, 区分业务域
	prefix string
	inst   redis.UniversalClient
}

func (r *RedisCache) Set(k string, x interface{}, ttl time.Duration) (ok bool, err error) {
	//TODO implement me
	k = common.GetCacheKey(r.prefix, k)
	err = r.inst.Set(k, x, ttl).Err()
	if err == nil {
		ok = true
	}

	return
}

func (r *RedisCache) Get(k string) (val interface{}, err error) {
	//TODO implement me
	k = common.GetCacheKey(r.prefix, k)
	val, err = r.inst.Get(k).Result()

	return val, err
}

func (r *RedisCache) Del(k string) (ok bool, err error) {
	//TODO implement me
	k = common.GetCacheKey(r.prefix, k)
	err = r.inst.Del(k).Err()
	if err == nil {
		ok = true
	}

	return
}

func (r *RedisCache) Expire(k string, ttl time.Duration) (ok bool, err error) {
	//TODO implement me
	k = common.GetCacheKey(r.prefix, k)
	ok, err = r.inst.Expire(k, ttl).Result()

	return
}

func (r *RedisCache) Incr(k string) (n int64, err error) {
	//TODO implement me
	k = common.GetCacheKey(r.prefix, k)
	n, err = r.inst.Incr(k).Result()

	return
}

func (r *RedisCache) Decr(k string) (n int64, err error) {
	//TODO implement me
	k = common.GetCacheKey(r.prefix, k)
	n, err = r.inst.Decr(k).Result()

	return
}

func (r *RedisCache) IncrBy(k string, v int64) (n int64, err error) {
	//TODO implement me
	k = common.GetCacheKey(r.prefix, k)
	n, err = r.inst.IncrBy(k, v).Result()

	return
}

func (r *RedisCache) DecrBy(k string, v int64) (n int64, err error) {
	//TODO implement me
	k = common.GetCacheKey(r.prefix, k)
	n, err = r.inst.DecrBy(k, v).Result()

	return
}

func (r *RedisCache) HGetAll(key string) (data map[string]string, err error) {
	k := common.GetCacheKey(r.prefix, key)
	data, err = r.inst.HGetAll(k).Result()
	if len(data) == 0 {
		err = redis.Nil
	}

	return
}

func (r *RedisCache) HMGet(key string, fields []string) (rs interface{}, err error) {
	k := common.GetCacheKey(r.prefix, key)
	rs, err = r.inst.HMGet(k, fields...).Result()
	if err != nil {
		return
	}

	return
}

func (r *RedisCache) HGet(key string, field string) (rs string, err error) {
	k := common.GetCacheKey(r.prefix, key)
	rs, err = r.inst.HGet(k, field).Result()
	if err != nil {
		return
	}

	return
}

func (r *RedisCache) HMSet(key string, data map[string]interface{}) (err error) {
	k := common.GetCacheKey(r.prefix, key)
	_, err = r.inst.HMSet(k, data).Result()

	return err
}

func (r *RedisCache) HSet(key, field string, val interface{}) (err error) {
	k := common.GetCacheKey(r.prefix, key)
	_, err = r.inst.HSet(k, field, val).Result()

	return
}

func (r *RedisCache) HDel(key string, fields []string) (err error) {
	k := common.GetCacheKey(r.prefix, key)
	_, err = r.inst.HDel(k, fields...).Result()

	return err
}

func (r *RedisCache) TTL(k string) (ttl time.Duration, err error) {
	//TODO implement me
	k = common.GetCacheKey(r.prefix, k)
	ttl, err = r.inst.TTL(k).Result()

	return
}

func (r *RedisCache) Lock(k string, v string, ttl time.Duration) (ok bool, err error) {
	//TODO implement me
	k = common.GetCacheKey(r.prefix, k)
	ok, err = r.inst.SetNX(k, v, ttl).Result()

	return
}

func (r *RedisCache) Unlock(k string, randVal string) (ok bool, err error) {
	//TODO implement me
	k = common.GetCacheKey(r.prefix, k)
	v := r.inst.Get(k).Val()
	if v == "" || v == randVal {
		_, err = r.inst.Del(k).Result()
		if err == nil {
			ok = true
		} else if strings.Contains(err.Error(), "nil") {
			ok = true
			err = nil
		}
	} else {
		err = errors.New("unlock failed value not match current: " + v + " rand: " + randVal)
	}

	return
}

func (r *RedisCache) Exists(k string) (exists bool, err error) {
	//TODO implement me
	k = common.GetCacheKey(r.prefix, k)
	v, err := r.inst.Exists(k).Result()
	if err != nil {
		return
	}

	if v > 0 {
		exists = true
	}

	return
}

func (r *RedisCache) LockExec(param common.LockExecParam) (err error) {
	//TODO implement me
	return common.LockExec(r, param)
}

func (r *RedisCache) GetInst() interface{} {
	//TODO implement me
	return r.inst
}
