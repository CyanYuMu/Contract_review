package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/spf13/cast"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/cache/dualwrite"
)

type RedisV8 struct {
	*Base
	cli redis.UniversalClient
}

type RedisConfig struct {
	Prefix   string   `yaml:"prefix"`
	Addrs    []string `yaml:"addrs"`
	Password string   `yaml:"password"`
	// 单节点模式生效
	DB              int  `yaml:"db"`
	PoolSize        int  `yaml:"poolSize"`
	IsCluster       bool `yaml:"isCluster"`
	RouteRandomly   bool `yaml:"routeRandomly"`
	ReadOnly        bool `yaml:"readOnly"`
	MinIdleConn     int  `yaml:"minIdleConn"`
	DialTimeoutSec  int  `yaml:"dialTimeoutSec"`
	ReadTimeoutSec  int  `yaml:"readTimeoutSec"`
	WriteTimeoutSec int  `yaml:"writeTimeoutSec"`
	PoolFIFO        bool `yaml:"poolFIFO"`
	// 最大生命周期, 超过释放回收
	MaxConnAgeSec int `yaml:"maxConnAgeSec"`
	// 连接池最大超时时间, 超过重置
	PoolTimeoutSec int `yaml:"poolTimeoutSec"`
	// 最大空闲时间, 超过释放
	IdleTimeoutSec int `yaml:"idleTimeoutSec"`
	// 检测空闲连接频率, 默认一分钟
	IdleCheckFrequencySec int `yaml:"idleCheckFrequencySec"`
	// 当设置此项时, 会直接引用此client作为客户端
	Cli redis.UniversalClient
	// 双写配置
	DualWrite *DualWriteConfig `yaml:"dualWrite"`
}

// DualWriteConfig 双写配置
type DualWriteConfig struct {
	dualwrite.Config `yaml:",inline"`           // 内嵌公共配置
	NewRedis         *RedisConfig `yaml:"newRedis"` // 新 Redis 配置
}

func NewRedisV8(cnf *RedisConfig) *RedisV8 {
	var inst redis.UniversalClient

	// 创建旧库客户端（使用主配置）
	oldCli := createRedisClientV8ForV2(cnf)

	// 检测双写配置
	if cnf.DualWrite != nil && cnf.DualWrite.Enabled && cnf.DualWrite.NewRedis != nil {
		// 创建新库客户端
		newCli := createNewRedisClientV8(cnf.DualWrite.NewRedis)

		// 测试新库连接（异步，不阻塞启动）
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := newCli.Ping(ctx).Err(); err != nil {
				fmt.Printf("[DualWrite] new redis connection test failed: %v, addr=%v\n", err, cnf.DualWrite.NewRedis.Addrs)
			} else {
				fmt.Printf("[DualWrite] new redis connected successfully, addr=%v\n", cnf.DualWrite.NewRedis.Addrs)
			}
		}()

		// 根据 ReadFrom 决定主客户端和双写目标
		var dualWriteTarget redis.UniversalClient
		if cnf.DualWrite.ReadFrom == "new" {
			// 主客户端使用新库，双写到旧库
			inst = newCli
			dualWriteTarget = oldCli
		} else {
			// 主客户端使用旧库（默认），双写到新库
			inst = oldCli
			dualWriteTarget = newCli
		}

		// 添加双写 Hook
		hook := dualwrite.NewHookV8(dualWriteTarget, &cnf.DualWrite.Config)
		inst.AddHook(hook)

		// 打印初始化日志
		primaryClient := "old"
		if cnf.DualWrite.ReadFrom == "new" {
			primaryClient = "new"
		}
		oldMode := "standalone"
		if cnf.IsCluster {
			oldMode = "cluster"
		}
		newMode := "standalone"
		if cnf.DualWrite.NewRedis.IsCluster {
			newMode = "cluster"
		}
		fmt.Printf("[DualWrite] Redis initialization: version=v2/v8, prefix=%s, oldRedis=%v (mode=%s), newRedis=%v (mode=%s), enabled=%v, syncRunning=%v, readFrom=%s, primaryClient=%s\n",
			cnf.Prefix, cnf.Addrs, oldMode, cnf.DualWrite.NewRedis.Addrs, newMode,
			cnf.DualWrite.Enabled, cnf.DualWrite.SyncRunning, cnf.DualWrite.ReadFrom, primaryClient)
	} else {
		// 无双写配置，直接使用旧库
		inst = oldCli
		oldMode := "standalone"
		if cnf.IsCluster {
			oldMode = "cluster"
		}
		fmt.Printf("[DualWrite] Redis initialization: version=v2/v8, prefix=%s, oldRedis=%v (mode=%s), dualWriteEnabled=false\n",
			cnf.Prefix, cnf.Addrs, oldMode)
	}

	return &RedisV8{
		Base: &Base{prefix: cnf.Prefix},
		cli:  inst,
	}
}

var hSetScriptV8 = redis.NewScript(`
local hset_ok, hset_result = pcall(redis.call, 'HSET', KEYS[1], ARGV[1], ARGV[2])
if not hset_ok or hset_result == nil then
    return redis.error_reply(hset_result)
end

call(redis.call, 'EXPIRE', KEYS[1], ARGV[3])

return 1
`)

var hMSetScriptV8 = redis.NewScript(`
local expire_time = tonumber(ARGV[#ARGV])
table.remove(ARGV, #ARGV)

local result = redis.pcall("HMSET", KEYS[1], unpack(ARGV))
if result.err then
    return result
end
redis.call("EXPIRE", KEYS[1], expire_time)
return 1
`)

var safeDelV8 = redis.NewScript(`
local exists = redis.call('EXISTS', KEYS[1])
if exists == 0 then
    return 1
end

local key_value = redis.call('GET', KEYS[1])
if key_value == ARGV[1] then
    redis.call('DEL', KEYS[1])
    return 1
else
    return 0
end
`)

// 当key存在时操作
var stringSetIfExists = redis.NewScript(`
local key = KEYS[1]
local value = ARGV[1]
local expireTime = tonumber(ARGV[2])

-- 判断key是否存在
local exists = redis.call('EXISTS', key)

if exists == 1 then
    -- key存在，使用SET命令进行操作，并设置过期时间
    redis.call('SET', key, value, 'EX', expireTime)
	return 1
else
    -- key不存在，返回错误信息或执行其他操作
	return -1
end
`)

var hMSetIfExists = redis.NewScript(`
local key = KEYS[1]
local fieldValues = cjson.decode(ARGV[1])
local expireTime = tonumber(ARGV[2])

-- 判断key是否存在
local exists = redis.call('EXISTS', key)
if exists == 1 then
    -- key存在，使用HMSET命令进行操作
    redis.call('HMSET', key, unpack(fieldValues))
	if expireTime > 0 then
    	redis.call('EXPIRE', key, expireTime)
	end	
	return 1
else
    -- key不存在，返回错误信息或执行其他操作
	return -1
end

`)

func (r *RedisV8) Lock(ctx context.Context, k string, v string, ttl time.Duration) (ok bool, err error) {
	//TODO implement me
	k = r.GetKey(k)
	ok, err = r.cli.SetNX(ctx, k, v, ttl).Result()

	return
}

func (r *RedisV8) Unlock(ctx context.Context, k string, ranVal string) (ok bool, err error) {
	//TODO implement me
	k = r.GetKey(k)
	result, err := safeDelV8.Run(ctx, r.cli, []string{k}, ranVal).Result()
	if err != nil {
		return false, err
	}
	if result.(int64) == 1 {
		return true, nil
	} else {
		return false, ErrUnlockValNotMatch
	}
}

func (r *RedisV8) Expire(ctx context.Context, k string, ttl time.Duration) (ok bool, err error) {
	k = r.GetKey(k)
	ok, err = r.cli.Expire(ctx, k, ttl).Result()

	return
}

func (r *RedisV8) Client() redis.UniversalClient {
	return r.cli
}

func (r *RedisV8) Set(ctx context.Context, k string, val interface{}, ttl time.Duration) (bool, error) {
	k = r.GetKey(k)
	result, err := r.cli.Set(ctx, k, val, ttl).Result()
	if err != nil {
		return false, err
	}

	return result == "OK", nil
}

func (r *RedisV8) SetIfExists(ctx context.Context, k string, val interface{}, ttl time.Duration) (bool, error) {
	k = r.GetKey(k)
	result, err := stringSetIfExists.Run(ctx, r.cli, []string{k}, val, ttl).Int()
	if err != nil {
		return false, err
	}

	return result == 1, nil
}

func (r *RedisV8) MGet(ctx context.Context, keys []string) ([]interface{}, error) {
	for i, _ := range keys {
		keys[i] = r.GetKey(keys[i])
	}
	return r.cli.MGet(ctx, keys...).Result()
}

func (r *RedisV8) MSet(ctx context.Context, keys []string, ttl time.Duration) (results []bool, err error) {
	pipeline := r.cli.Pipeline()
	for i, _ := range keys {
		pipeline.Set(ctx, r.GetKey(keys[i]), keys[i], ttl)
	}
	rs, err := pipeline.Exec(ctx)
	if err != nil {
		return nil, err
	}

	results = make([]bool, 0, len(keys))
	for i := range rs {
		results = append(results, rs[i].Err() == nil)
	}

	return results, nil
}

func (r *RedisV8) GetStr(ctx context.Context, k string) (v string, err error) {
	k = r.GetKey(k)
	return r.cli.Get(ctx, k).Result()
}

func (r *RedisV8) Get(ctx context.Context, k string) (v interface{}, err error) {
	k = r.GetKey(k)
	return r.cli.Get(ctx, k).Result()
}

func (r *RedisV8) Del(ctx context.Context, k string) (bool, error) {
	k = r.GetKey(k)
	result, err := r.cli.Del(ctx, k).Result()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}

func (r *RedisV8) PipeDel(ctx context.Context, keys []string) error {
	p := r.cli.Pipeline()
	for i, _ := range keys {
		p.Del(ctx, keys[i])
	}
	_, err := p.Exec(ctx)

	return err
}

func (r *RedisV8) Info(ctx context.Context, section ...string) (string, error) {
	return r.cli.Info(ctx, section...).Result()
}

func (r *RedisV8) Pipeline() redis.Pipeliner {
	return r.cli.Pipeline()
}

func (r *RedisV8) TxPipeline() redis.Pipeliner {
	return r.cli.TxPipeline()
}

func (r *RedisV8) Close() error {
	return r.cli.Close()
}

func (r *RedisV8) SAdd(ctx context.Context, k string, members ...string) (rs int64, err error) {
	k = r.GetKey(k)
	// to interface{}
	iKeys := make([]interface{}, 0, len(members))
	for i, _ := range members {
		iKeys = append(iKeys, members[i])
	}
	return r.cli.SAdd(ctx, k, iKeys...).Result()
}

func (r *RedisV8) SMIsMembers(ctx context.Context, k string, members ...string) (result []bool, err error) {
	k = r.GetKey(k)
	iKeys := make([]interface{}, 0, len(members))
	for i, _ := range members {
		iKeys = append(iKeys, members[i])
	}

	result, err = r.cli.SMIsMember(ctx, k, iKeys).Result()
	return
}

func (r *RedisV8) SRem(ctx context.Context, k string, members ...string) (rs int64, err error) {
	k = r.GetKey(k)
	iKeys := make([]interface{}, 0, len(members))
	for i, _ := range members {
		iKeys = append(iKeys, members[i])
	}
	return r.cli.SRem(ctx, k, iKeys...).Result()
}

func (r *RedisV8) SIsMember(ctx context.Context, k string, member string) (rs bool, err error) {
	k = r.GetKey(k)
	return r.cli.SIsMember(ctx, k, member).Result()
}

func (r *RedisV8) SCard(ctx context.Context, k string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.SCard(ctx, k).Result()
}

func (r *RedisV8) SMembers(ctx context.Context, k string) (rs []string, err error) {
	k = r.GetKey(k)
	return r.cli.SMembers(ctx, k).Result()
}

func (r *RedisV8) ZAdd(ctx context.Context, k string, member string, score float64) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.ZAdd(ctx, k, &redis.Z{Member: member, Score: score}).Result()
}

func (r *RedisV8) ZRem(ctx context.Context, k string, member string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.ZRem(ctx, k, member).Result()
}

func (r *RedisV8) ZCard(ctx context.Context, k string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.ZCard(ctx, k).Result()
}

func (r *RedisV8) ZScore(ctx context.Context, k string, member string) (rs float64, err error) {
	k = r.GetKey(k)
	return r.cli.ZScore(ctx, k, member).Result()
}

func (r *RedisV8) ZRange(ctx context.Context, k string, start int64, stop int64) (rs []string, err error) {
	k = r.GetKey(k)
	return r.cli.ZRange(ctx, k, start, stop).Result()
}

func (r *RedisV8) ZRevRange(ctx context.Context, k string, start int64, stop int64) (rs []string, err error) {
	k = r.GetKey(k)
	return r.cli.ZRevRange(ctx, k, start, stop).Result()
}

func (r *RedisV8) ZRangeWithScores(ctx context.Context, k string, start int64, stop int64) (s ZSorts, err error) {
	k = r.GetKey(k)

	rs, err := r.cli.ZRangeWithScores(ctx, k, start, stop).Result()
	if err != nil {
		return nil, err
	}
	s = make(ZSorts, 0, len(rs))
	for i := range rs {
		s = append(s, Z{
			Score:  rs[i].Score,
			Member: rs[i].Member,
		})
	}

	return s, nil
}

func (r *RedisV8) ZRevRangeWithScores(ctx context.Context, k string, start int64, stop int64) (s ZSorts, err error) {
	k = r.GetKey(k)

	rs, err := r.cli.ZRevRangeWithScores(ctx, k, start, stop).Result()
	if err != nil {
		return nil, err
	}
	s = make(ZSorts, 0, len(rs))
	for i := range rs {
		s = append(s, Z{
			Score:  rs[i].Score,
			Member: rs[i].Member,
		})
	}

	return s, nil
}

func (r *RedisV8) ZRangeByScore(ctx context.Context, k string, opt redis.ZRangeBy) (rs []string, err error) {
	k = r.GetKey(k)
	return r.cli.ZRangeByScore(ctx, k, &opt).Result()
}

func (r *RedisV8) ZRevRangeByScore(ctx context.Context, k string, opt redis.ZRangeBy) (rs []string, err error) {
	k = r.GetKey(k)
	return r.cli.ZRevRangeByScore(ctx, k, &opt).Result()
}

func (r *RedisV8) ZRangeByScoreWithScores(ctx context.Context, k string, opt redis.ZRangeBy) (s ZSorts, err error) {
	k = r.GetKey(k)
	rs, err := r.cli.ZRangeByScoreWithScores(ctx, k, &opt).Result()
	if err != nil {
		return nil, err
	}
	s = make(ZSorts, 0, len(rs))
	for i := range rs {
		s = append(s, Z{Member: rs[i].Member, Score: rs[i].Score})
	}

	return s, nil
}

func (r *RedisV8) ZRevRangeByScoreWithScores(ctx context.Context, k string, opt redis.ZRangeBy) (rs []redis.Z, err error) {
	k = r.GetKey(k)
	return r.cli.ZRevRangeByScoreWithScores(ctx, k, &opt).Result()
}

func (r *RedisV8) ZRank(ctx context.Context, k string, member string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.ZRank(ctx, k, member).Result()
}

func (r *RedisV8) ZRevRank(ctx context.Context, k string, member string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.ZRevRank(ctx, k, member).Result()
}

func (r *RedisV8) ZIncrBy(ctx context.Context, k string, member string, increment float64) (rs float64, err error) {
	k = r.GetKey(k)
	return r.cli.ZIncrBy(ctx, k, increment, member).Result()
}

func (r *RedisV8) ZCount(ctx context.Context, k string, min string, max string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.ZCount(ctx, k, min, max).Result()
}

func (r *RedisV8) ZRemRangeByRank(ctx context.Context, k string, start int64, stop int64) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.ZRemRangeByRank(ctx, k, start, stop).Result()
}

func (r *RedisV8) ZRemRangeByScore(ctx context.Context, k string, min string, max string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.ZRemRangeByScore(ctx, k, min, max).Result()
}

func (r *RedisV8) ZRemRangeByLex(ctx context.Context, k string, min string, max string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.ZRemRangeByLex(ctx, k, min, max).Result()
}

func (r *RedisV8) ZUnionStore(ctx context.Context, dest string, store redis.ZStore) (rs int64, err error) {
	dest = r.GetKey(dest)
	return r.cli.ZUnionStore(ctx, dest, &store).Result()
}

func (r *RedisV8) ZInterStore(ctx context.Context, dest string, store redis.ZStore) (rs int64, err error) {
	dest = r.GetKey(dest)
	return r.cli.ZInterStore(ctx, dest, &store).Result()
}

func (r *RedisV8) HDel(ctx context.Context, k string, fields ...string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.HDel(ctx, k, fields...).Result()
}

func (r *RedisV8) HExists(ctx context.Context, k string, field string) (rs bool, err error) {
	k = r.GetKey(k)
	return r.cli.HExists(ctx, k, field).Result()
}

func (r *RedisV8) Exists(ctx context.Context, k string) (exists bool, err error) {
	k = r.GetKey(k)
	status, err := r.cli.Exists(ctx, k).Result()
	if err != nil {
		return
	}

	exists = status > 0

	return
}

func (r *RedisV8) HGet(ctx context.Context, k string, field string) (rs string, err error) {
	k = r.GetKey(k)
	return r.cli.HGet(ctx, k, field).Result()
}

func (r *RedisV8) HGetAll(ctx context.Context, k string) (rs map[string]string, err error) {
	k = r.GetKey(k)
	return r.cli.HGetAll(ctx, k).Result()
}

func (r *RedisV8) HIncrBy(ctx context.Context, k string, field string, incr int64) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.HIncrBy(ctx, k, field, incr).Result()
}

func (r *RedisV8) Incr(ctx context.Context, k string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.Incr(ctx, k).Result()
}

func (r *RedisV8) IncrBy(ctx context.Context, k string, incr int64) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.IncrBy(ctx, k, incr).Result()
}

func (r *RedisV8) HIncrByFloat(ctx context.Context, k string, field string, incr float64) (rs float64, err error) {
	k = r.GetKey(k)
	return r.cli.HIncrByFloat(ctx, k, field, incr).Result()
}

func (r *RedisV8) HKeys(ctx context.Context, k string) (rs []string, err error) {
	k = r.GetKey(k)
	return r.cli.HKeys(ctx, k).Result()
}

func (r *RedisV8) HLen(ctx context.Context, k string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.HLen(ctx, k).Result()
}

func (r *RedisV8) HMGet(ctx context.Context, k string, fields ...string) (rs []interface{}, err error) {
	k = r.GetKey(k)
	return r.cli.HMGet(ctx, k, fields...).Result()

}

func (r *RedisV8) HMSet(ctx context.Context, k string, fields map[string]interface{}, ttl time.Duration) (rs bool, err error) {
	k = r.GetKey(k)
	if ttl > 0 {
		pairs := make([]interface{}, 0, len(fields)*2+1)
		for s := range fields {
			pairs = append(pairs, s, fields[s])
		}
		pairs = append(pairs, ttl.Seconds())
		result, err := hMSetScriptV8.Run(ctx, r.cli, []string{k}, pairs...).Result()
		if err != nil {
			return false, err
		}

		return cast.ToInt(result) == 1, nil
	}

	return r.cli.HMSet(ctx, k, fields).Result()
}

/*HMSetIfExists
* @Description: hash操作, 前提是hash数据已经存在
* @param ctx
* @param k
* @param fields
* @param ttl
* @return rs
* @return err
 */
func (r *RedisV8) HMSetIfExists(ctx context.Context, k string, fields map[string]interface{}, ttl time.Duration) (rs bool, err error) {
	i, err := hMSetScriptV8.Run(ctx, r.cli, []string{k}, fields, ttl.Seconds()).Int()
	if err != nil {
		return
	}

	return i == 1, nil
}

func (r *RedisV8) HSet(ctx context.Context, k string, field string, value interface{}, ttl time.Duration) (rs bool, err error) {
	k = r.GetKey(k)
	if ttl > 0 {
		result, err := hSetScriptV8.Run(ctx, r.cli, []string{k}, field, value, ttl.Seconds()).Result()
		if err != nil {
			return false, err
		}

		return result.(int64) == 1, nil
	} else {
		result, err := r.cli.HSet(ctx, k, field, value).Result()
		if err != nil {
			return false, err
		}

		return result == 1, nil
	}
}

func (r *RedisV8) HSetNX(ctx context.Context, k string, field string, value interface{}) (rs bool, err error) {
	k = r.GetKey(k)
	return r.cli.HSetNX(ctx, k, field, value).Result()
}

func (r *RedisV8) HVals(ctx context.Context, k string) (rs []string, err error) {
	k = r.GetKey(k)
	return r.cli.HVals(ctx, k).Result()
}

func (r *RedisV8) HScan(ctx context.Context, k string, cursor uint64, match string, count int64) (rs []string, ncursor uint64, err error) {
	k = r.GetKey(k)
	return r.cli.HScan(ctx, k, cursor, match, count).Result()
}

func (r *RedisV8) BLPop(ctx context.Context, timeout time.Duration, keys ...string) (rs []string, err error) {
	for i, k := range keys {
		keys[i] = r.GetKey(k)
	}
	return r.cli.BLPop(ctx, timeout, keys...).Result()
}

func (r *RedisV8) BRPop(ctx context.Context, timeout time.Duration, keys ...string) (rs []string, err error) {
	for i, k := range keys {
		keys[i] = r.GetKey(k)
	}
	return r.cli.BRPop(ctx, timeout, keys...).Result()
}

func (r *RedisV8) BRPopLPush(ctx context.Context, source string, destination string, timeout time.Duration) (rs string, err error) {
	source = r.GetKey(source)
	destination = r.GetKey(destination)
	return r.cli.BRPopLPush(ctx, source, destination, timeout).Result()
}

func (r *RedisV8) LIndex(ctx context.Context, k string, index int64) (rs string, err error) {
	k = r.GetKey(k)
	return r.cli.LIndex(ctx, k, index).Result()
}

func (r *RedisV8) LInsert(ctx context.Context, k string, op string, pivot interface{}, value interface{}) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.LInsert(ctx, k, op, pivot, value).Result()
}

func (r *RedisV8) LInsertBefore(ctx context.Context, k string, pivot interface{}, value interface{}) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.LInsertBefore(ctx, k, pivot, value).Result()
}

func (r *RedisV8) LInsertAfter(ctx context.Context, k string, pivot interface{}, value interface{}) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.LInsertAfter(ctx, k, pivot, value).Result()
}

func (r *RedisV8) LLen(ctx context.Context, k string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.LLen(ctx, k).Result()
}

func (r *RedisV8) LPop(ctx context.Context, k string) (rs string, err error) {
	k = r.GetKey(k)
	return r.cli.LPop(ctx, k).Result()
}

func (r *RedisV8) LPush(ctx context.Context, k string, values ...interface{}) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.LPush(ctx, k, values...).Result()
}

func (r *RedisV8) LPushX(ctx context.Context, k string, value interface{}) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.LPushX(ctx, k, value).Result()
}

func (r *RedisV8) LRange(ctx context.Context, k string, start int64, stop int64) (rs []string, err error) {
	k = r.GetKey(k)
	return r.cli.LRange(ctx, k, start, stop).Result()
}

func (r *RedisV8) LRem(ctx context.Context, k string, count int64, value interface{}) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.LRem(ctx, k, count, value).Result()
}

func (r *RedisV8) LSet(ctx context.Context, k string, index int64, value interface{}) (rs string, err error) {
	k = r.GetKey(k)
	return r.cli.LSet(ctx, k, index, value).Result()
}

func (r *RedisV8) LTrim(ctx context.Context, k string, start int64, stop int64) (rs string, err error) {
	k = r.GetKey(k)
	return r.cli.LTrim(ctx, k, start, stop).Result()
}

func (r *RedisV8) RPop(ctx context.Context, k string) (rs string, err error) {
	k = r.GetKey(k)
	return r.cli.RPop(ctx, k).Result()
}

func (r *RedisV8) RPopLPush(ctx context.Context, source string, destination string) (rs string, err error) {
	source = r.GetKey(source)
	destination = r.GetKey(destination)
	return r.cli.RPopLPush(ctx, source, destination).Result()
}

func (r *RedisV8) RPush(ctx context.Context, k string, values ...interface{}) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.RPush(ctx, k, values...).Result()
}

func (r *RedisV8) RPushX(ctx context.Context, k string, value interface{}) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.RPushX(ctx, k, value).Result()
}

type PipeHashGetSetOption struct {
	GetCacheKey func(k string) string
	Ttl         time.Duration
	// 当 > 0时, 会进行防止穿透处理
	EmptyTtl time.Duration
	// 指定需要获取的字段, 默认全部
	Fields []string
}

type HashItem interface {
	CacheMapStringString() map[string]string
	//CacheItem() string
}

func (p *PipeHashGetSetOption) Apply(opt *PipeHashGetSetOption) {
	if opt == nil {
		return
	}
	if opt.GetCacheKey != nil {
		p.GetCacheKey = opt.GetCacheKey
	}

	if opt.Ttl > 0 {
		p.Ttl = opt.Ttl
	}

	if opt.EmptyTtl > 0 {
		p.EmptyTtl = opt.EmptyTtl
	}
}

func getDftPipeHashGetSetOption() *PipeHashGetSetOption {
	return &PipeHashGetSetOption{
		GetCacheKey: func(k string) string {
			return k
		},
		Ttl: time.Minute * 30,
	}
}

type HashRs struct {
	items map[string]map[string]string
}

func (h *HashRs) Items() map[string]map[string]string {
	return h.items
}

func (h *HashRs) Set(key string, data map[string]string) {
	if h.items == nil {
		h.items = make(map[string]map[string]string)
	}
	h.items[key] = data
}

func (h *HashRs) MSet(items map[string]map[string]string) {
	if h.items == nil {
		h.items = make(map[string]map[string]string)
	}
	for k, v := range items {
		h.items[k] = v
	}
}

func (h *HashRs) Get(key string) map[string]string {
	if h.items == nil {
		return nil
	}
	return h.items[key]
}

func (h *HashRs) Decode(key string, decoder func(data map[string]string) interface{}) interface{} {
	if h.items == nil {
		return nil
	}
	if i, ok := h.items[key]; ok {
		return decoder(i)
	}

	return nil
}

type BatchGetter func(ctx context.Context, keys []string) (items map[string]HashItem, err error)

func (r *RedisV8) PipeHashGetSet(ctx context.Context, keys []string, batchGetter BatchGetter, option *PipeHashGetSetOption) (rs *HashRs, err error) {
	opt := getDftPipeHashGetSetOption()
	opt.Apply(option)

	pipe := r.Pipeline()
	for i := range keys {
		if len(opt.Fields) > 0 {
			pipe.HMGet(ctx, opt.GetCacheKey(keys[i]), opt.Fields...)
		} else {
			pipe.HGetAll(ctx, opt.GetCacheKey(keys[i]))
		}
	}
	cmders, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, err
	}
	rs = &HashRs{
		items: make(map[string]map[string]string, len(keys)),
	}

	var missKeys = make([]string, 0)
	for i := range cmders {
		cacheData, err1 := cmders[i].(*redis.StringStringMapCmd).Result()
		if err1 != nil && err1 != redis.Nil {
			return nil, cmders[i].Err()
		} else if len(cacheData) == 0 || err1 == redis.Nil {
			missKeys = append(missKeys, keys[i])
		} else if len(cacheData) == 0 && cacheData[EmptyString] == EmptyString {
			continue
		} else {
			rs.items[keys[i]] = cacheData
		}
	}
	if len(missKeys) > 0 {
		nPipe := r.Pipeline()
		dbItemRef, err := batchGetter(ctx, missKeys)
		if err != nil {
			return rs, err
		}

		for i := range missKeys {
			if dbItemRef[missKeys[i]] == nil {
				// empty
				if opt.EmptyTtl > 0 {
					nPipe.HSet(ctx, opt.GetCacheKey(missKeys[i]), EmptyString, EmptyString)
					nPipe.Expire(ctx, opt.GetCacheKey(missKeys[i]), opt.EmptyTtl)
				}
				continue
			} else {
				cacheData := dbItemRef[missKeys[i]].CacheMapStringString()
				nPipe.HMSet(ctx, opt.GetCacheKey(missKeys[i]), cacheData)
				if opt.Ttl > 0 {
					nPipe.Expire(ctx, opt.GetCacheKey(missKeys[i]), opt.Ttl)
				}
				rs.items[missKeys[i]] = cacheData
			}
		}
		_, err = nPipe.Exec(ctx)
		if err != nil {
			return rs, err
		}
	}

	return rs, err
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
		result, err = tryHashIncrByWithExtendScript.Run(ctx, r.cli, []string{k}, items...).Result()
	} else {
		result, err = tryHashIncrByScript.Run(ctx, r.cli, []string{k}, items...).Result()
	}
	if err != nil {
		return
	}

	// 不存在时返回 -1
	if _, ok := result.(int64); ok {
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
		result, err = tryHashSetWithExtendScript.Run(ctx, r.cli, []string{k}, items...).Result()
	} else {
		result, err = tryHashSetScript.Run(ctx, r.cli, []string{k}, items...).Result()
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
		result, err = hashIncrByWithExtendScript.Run(ctx, r.cli, []string{k}, items...).Result()
	} else {
		result, err = hashIncrByScript.Run(ctx, r.cli, []string{k}, items...).Result()
	}
	if err != nil {
		return
	}

	// 不存在时返回 -1
	if _, ok := result.(int64); ok {
		return nil, nil
	}
	rs = make(map[string]int64, len(data))
	results := result.([]interface{})

	for i := 0; i < len(results); i += 2 {
		rs[results[i].(string)] = cast.ToInt64(results[i+1])
	}

	return rs, nil
}

func (r *RedisV8) LockExec(ctx context.Context, param LockExecParam) (err error) {
	return LockExec(ctx, r, param)
}

func (r *RedisV8) Publish(ctx context.Context, topic string, key string) (err error) {
	_, err = r.cli.Publish(ctx, topic, key).Result()
	return
}
