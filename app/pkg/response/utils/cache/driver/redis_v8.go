package driver

import (
	"context"
	"errors"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/spf13/cast"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/cache/common"
)

func NewRedisV8(conf RedisCacheConfig) *RedisV8 {
	var inst redis.UniversalClient
	dialTimeout := time.Duration(conf.DialTimeoutSec) * time.Second
	ReadTimeout := time.Duration(conf.ReadTimeoutSec) * time.Second
	WriteTimeout := time.Duration(conf.WriteTimeoutSec) * time.Second

	if conf.IsCluster {
		inst = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:              conf.Addrs,
			Password:           conf.Password,
			PoolSize:           conf.PoolSize,
			ReadOnly:           conf.ReadOnly,
			RouteRandomly:      conf.RouteRandomly,
			MinIdleConns:       conf.MinIdleConn,
			DialTimeout:        dialTimeout,
			ReadTimeout:        ReadTimeout,
			WriteTimeout:       WriteTimeout,
			PoolFIFO:           conf.PoolFIFO,
			MaxConnAge:         time.Duration(conf.MaxConnAgeSec) * time.Second,
			PoolTimeout:        time.Duration(conf.PoolTimeoutSec) * time.Second,
			IdleTimeout:        time.Duration(conf.IdleTimeoutSec) * time.Second,
			IdleCheckFrequency: time.Duration(conf.IdleCheckFrequencySec) * time.Second,
		})
	} else {
		inst = redis.NewClient(&redis.Options{
			Addr:               conf.Addrs[0],
			Password:           conf.Password,
			DB:                 conf.DB,
			PoolSize:           conf.PoolSize,
			MinIdleConns:       conf.MinIdleConn,
			DialTimeout:        dialTimeout,
			ReadTimeout:        ReadTimeout,
			WriteTimeout:       WriteTimeout,
			PoolFIFO:           conf.PoolFIFO,
			MaxConnAge:         time.Duration(conf.MaxConnAgeSec) * time.Second,
			PoolTimeout:        time.Duration(conf.PoolTimeoutSec) * time.Second,
			IdleTimeout:        time.Duration(conf.IdleTimeoutSec) * time.Second,
			IdleCheckFrequency: time.Duration(conf.IdleCheckFrequencySec) * time.Second,
		})
	}

	return &RedisV8{
		prefix: conf.Prefix,
		inst:   inst,
	}
}

// 定长队列设置
var fixLenQueueSetScript = redis.NewScript(`
local key = KEYS[1]
local maxLength = tonumber(ARGV[#ARGV - 2])
local expireTime = tonumber(ARGV[#ARGV])
local updateExpire = ARGV[#ARGV - 1] == "1"

local i = 1
while i <= #ARGV - 3 do
    local member = ARGV[i]
    local score = tonumber(ARGV[i + 1])
    
    if score == nil then
        return redis.error_reply("Score at position " .. (i + 1) .. " is not a valid float: " .. ARGV[i + 1])
    end
	redis.call('zadd', key, score, member)
    i = i + 2
end

local len = redis.call('zcard', key)
if len > maxLength then
    redis.call('zremrangebyrank', key, 0, len - maxLength - 1)
end

if updateExpire then
    redis.call('expire', key, expireTime)
elseif redis.call('exists', key) == 0 then
    redis.call('expire', key, expireTime)
end

return 1
`)

type RedisV8 struct {
	// key前缀, 区分业务域
	prefix string
	inst   redis.UniversalClient
	ctx    context.Context
}

func (r *RedisV8) WithContext(ctx context.Context) *RedisV8 {
	r.ctx = ctx

	return r
}

func (r *RedisV8) getContext() context.Context {
	if r.ctx == nil {
		return context.Background()
	}

	return r.ctx
}

func (r *RedisV8) Set(k string, x interface{}, ttl time.Duration) (ok bool, err error) {
	//TODO implement me
	k = common.GetCacheKey(r.prefix, k)
	err = r.inst.Set(r.getContext(), k, x, ttl).Err()
	if err == nil {
		ok = true
	}

	return
}

func (r *RedisV8) Get(k string) (val interface{}, err error) {
	//TODO implement me
	k = common.GetCacheKey(r.prefix, k)
	val, err = r.inst.Get(r.getContext(), k).Result()

	return val, err
}

func (r *RedisV8) Del(k string) (ok bool, err error) {
	//TODO implement me
	k = common.GetCacheKey(r.prefix, k)
	err = r.inst.Del(r.getContext(), k).Err()
	if err == nil {
		ok = true
	}

	return
}

func (r *RedisV8) Expire(k string, ttl time.Duration) (ok bool, err error) {
	//TODO implement me
	k = common.GetCacheKey(r.prefix, k)
	ok, err = r.inst.Expire(r.getContext(), k, ttl).Result()

	return
}

func (r *RedisV8) Incr(k string) (n int64, err error) {
	//TODO implement me
	k = common.GetCacheKey(r.prefix, k)
	n, err = r.inst.Incr(r.getContext(), k).Result()

	return
}

func (r *RedisV8) Decr(k string) (n int64, err error) {
	//TODO implement me
	k = common.GetCacheKey(r.prefix, k)
	n, err = r.inst.Decr(r.getContext(), k).Result()

	return
}

func (r *RedisV8) IncrBy(k string, v int64) (n int64, err error) {
	//TODO implement me
	k = common.GetCacheKey(r.prefix, k)
	n, err = r.inst.IncrBy(r.getContext(), k, v).Result()

	return
}

func (r *RedisV8) DecrBy(k string, v int64) (n int64, err error) {
	//TODO implement me
	k = common.GetCacheKey(r.prefix, k)
	n, err = r.inst.DecrBy(r.getContext(), k, v).Result()

	return
}

func (r *RedisV8) HGetAll(key string) (data map[string]string, err error) {
	k := common.GetCacheKey(r.prefix, key)
	data, err = r.inst.HGetAll(r.getContext(), k).Result()
	if len(data) == 0 {
		err = redis.Nil
	}

	return
}

func (r *RedisV8) HMGet(key string, fields []string) (rs interface{}, err error) {
	k := common.GetCacheKey(r.prefix, key)
	rs, err = r.inst.HMGet(r.getContext(), k, fields...).Result()
	if err != nil {
		return
	}

	return
}

func (r *RedisV8) HGet(key string, field string) (rs string, err error) {
	k := common.GetCacheKey(r.prefix, key)
	rs, err = r.inst.HGet(r.getContext(), k, field).Result()
	if err != nil {
		return
	}

	return
}

func (r *RedisV8) HMSet(key string, data map[string]interface{}) (err error) {
	k := common.GetCacheKey(r.prefix, key)
	_, err = r.inst.HMSet(r.getContext(), k, data).Result()

	return err
}

func (r *RedisV8) HSet(key, field string, val interface{}) (err error) {
	k := common.GetCacheKey(r.prefix, key)
	_, err = r.inst.HSet(r.getContext(), k, field, val).Result()

	return
}

func (r *RedisV8) HDel(key string, fields []string) (err error) {
	k := common.GetCacheKey(r.prefix, key)
	_, err = r.inst.HDel(r.getContext(), k, fields...).Result()

	return err
}

func (r *RedisV8) TTL(k string) (ttl time.Duration, err error) {
	//TODO implement me
	k = common.GetCacheKey(r.prefix, k)
	ttl, err = r.inst.TTL(r.getContext(), k).Result()

	return
}

func (r *RedisV8) Lock(k string, v string, ttl time.Duration) (ok bool, err error) {
	//TODO implement me
	k = common.GetCacheKey(r.prefix, k)
	ok, err = r.inst.SetNX(r.getContext(), k, v, ttl).Result()

	return
}

func (r *RedisV8) Unlock(k string, randVal string) (ok bool, err error) {
	//TODO implement me
	k = common.GetCacheKey(r.prefix, k)
	v, err := r.inst.Get(r.getContext(), k).Result()
	// 判断当前key是否存在, 不存在直接返回成功
	if errors.Is(err, redis.Nil) {
		ok = true
		err = nil
		return
	}
	if v == "" || v == randVal {
		_, err = r.inst.Del(r.getContext(), k).Result()
		if err == nil {
			ok = true
		}
	} else {
		err = errors.New("unlock failed value not match current: " + v + " rand: " + randVal)
	}

	return
}

func (r *RedisV8) Exists(k string) (exists bool, err error) {
	//TODO implement me
	k = common.GetCacheKey(r.prefix, k)
	v, err := r.inst.Exists(r.getContext(), k).Result()
	if err != nil {
		return
	}

	if v > 0 {
		exists = true
	}

	return
}

func (r *RedisV8) LockExec(param common.LockExecParam) (err error) {
	//TODO implement me
	return common.LockExec(r, param)
}

func (r *RedisV8) GetInst() interface{} {
	//TODO implement me
	return r.inst
}

type HashOption struct {
	ExtendTtl bool
}

func (t *HashOption) Apply(opt *HashOption) {
	if opt == nil {
		return
	}

	t.ExtendTtl = opt.ExtendTtl
}

func dftTryOption() *HashOption {
	return &HashOption{}
}

/*TryHashIncrBy
* @Description:
* @param ctx
* @param k
* @param data
* @param ttl
* @param opt
* @return rs 当err == nil && rs == nil 是表示记录不存在
* @return err
 */
func (r *RedisV8) TryHashIncrBy(ctx context.Context, k string, data map[string]int64, ttl time.Duration, opt *HashOption) (rs map[string]int64, err error) {
	option := dftTryOption()
	option.Apply(opt)
	items := make([]interface{}, 0, len(data)*2)
	if option.ExtendTtl {
		items = append(items, ttl.Seconds())
	}
	for field, val := range data {
		items = append(items, field, val)
	}
	var result interface{}
	if option.ExtendTtl {
		result, err = tryHashIncrByWithExtendScript.Run(ctx, r.inst, []string{k}, items...).Result()
	} else {
		result, err = tryHashIncrByScript.Run(ctx, r.inst, []string{k}, items...).Result()
	}
	if err != nil {
		return
	}

	// 不存在时返回 -1
	if _, ok := (result.(int64)); ok {
		return nil, nil
	}
	rs = make(map[string]int64, len(data))
	results := result.([]interface{})

	for i := 0; i < len(results); i += 2 {
		rs[results[i].(string)] = cast.ToInt64(results[i+1])
	}

	return rs, nil
}

/*TryHashSet
* @Description:
* @param ctx
* @param k
* @param data
* @param ttl
* @param opt
* @return status 1=>成功 -1 => 记录不存在
* @return err
 */
func (r *RedisV8) TryHashSet(ctx context.Context, k string, data map[string]interface{}, ttl time.Duration, opt *HashOption) (status int8, err error) {
	//TODO implement me
	option := dftTryOption()
	option.Apply(opt)
	items := make([]interface{}, 0, len(data)*2)
	if option.ExtendTtl {
		items = append(items, ttl.Seconds())
	}
	for field := range data {
		items = append(items, field, data[field])
	}
	var result interface{}
	if option.ExtendTtl {
		result, err = tryHashSetWithExtendScript.Run(ctx, r.inst, []string{k}, items...).Result()
	} else {
		result, err = tryHashSetScript.Run(ctx, r.inst, []string{k}, items...).Result()
	}
	if err != nil {
		return
	}

	status = cast.ToInt8(result)

	return status, nil
}

func (r *RedisV8) HashIncrBy(ctx context.Context, k string, data map[string]int64, ttl time.Duration, opt *HashOption) (rs map[string]int64, err error) {
	option := dftTryOption()
	option.Apply(opt)
	items := make([]interface{}, 0, len(data)*2)
	if option.ExtendTtl {
		items = append(items, ttl.Seconds())
	}
	for field, val := range data {
		items = append(items, field, val)
	}
	var result interface{}
	if option.ExtendTtl {
		result, err = hashIncrByWithExtendScript.Run(ctx, r.inst, []string{k}, items...).Result()
	} else {
		result, err = hashIncrByScript.Run(ctx, r.inst, []string{k}, items...).Result()
	}
	if err != nil {
		return
	}

	// 不存在时返回 -1
	if _, ok := (result.(int64)); ok {
		return nil, nil
	}
	rs = make(map[string]int64, len(data))
	results := result.([]interface{})

	for i := 0; i < len(results); i += 2 {
		rs[results[i].(string)] = cast.ToInt64(results[i+1])
	}

	return rs, nil
}
