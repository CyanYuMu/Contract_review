package cache

import (
	"context"
	"fmt"
	"time"

	redis "github.com/redis/go-redis/v9"
	"github.com/spf13/cast"
)

// RedisV9Compat 是 RedisV8 的 v9 兼容版本。
// 底层使用 go-redis/v9 客户端（支持 Valkey 集群），
// 对外提供与 RedisV8 完全一致的方法名和参数签名。
// 业务侧只需将类型从 *RedisV8 改为 *RedisV9Compat，方法调用零改动。
type RedisV9Compat struct {
	*Base
	cli redis.UniversalClient
}

// RedisV9CompatConfig 创建 RedisV9Compat 所需的配置
type RedisV9CompatConfig struct {
	Prefix string
	Cli    redis.UniversalClient
}

// NewRedisV9Compat 创建 RedisV9Compat 实例
func NewRedisV9Compat(cnf *RedisV9CompatConfig) *RedisV9Compat {
	return &RedisV9Compat{
		Base: &Base{prefix: cnf.Prefix},
		cli:  cnf.Cli,
	}
}

// --- Lua 脚本（v9 版本，内容与 v8 完全一致） ---

var hSetScriptV9Compat = redis.NewScript(`
local hset_ok, hset_result = pcall(redis.call, 'HSET', KEYS[1], ARGV[1], ARGV[2])
if not hset_ok or hset_result == nil then
    return redis.error_reply(hset_result)
end

call(redis.call, 'EXPIRE', KEYS[1], ARGV[3])

return 1
`)

var hMSetScriptV9Compat = redis.NewScript(`
local expire_time = tonumber(ARGV[#ARGV])
table.remove(ARGV, #ARGV)

local result = redis.pcall("HMSET", KEYS[1], unpack(ARGV))
if result.err then
    return result
end
redis.call("EXPIRE", KEYS[1], expire_time)
return 1
`)

var safeDelV9Compat = redis.NewScript(`
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

var stringSetIfExistsV9Compat = redis.NewScript(`
local key = KEYS[1]
local value = ARGV[1]
local expireTime = tonumber(ARGV[2])

local exists = redis.call('EXISTS', key)

if exists == 1 then
    redis.call('SET', key, value, 'EX', expireTime)
	return 1
else
	return -1
end
`)

var hMSetIfExistsV9Compat = redis.NewScript(`
local key = KEYS[1]
local fieldValues = cjson.decode(ARGV[1])
local expireTime = tonumber(ARGV[2])

local exists = redis.call('EXISTS', key)
if exists == 1 then
    redis.call('HMSET', key, unpack(fieldValues))
	if expireTime > 0 then
    	redis.call('EXPIRE', key, expireTime)
	end	
	return 1
else
	return -1
end
`)

var tryHashIncrByScriptV9Compat = redis.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 1 then
    local result = {}
    for i = 1, #ARGV, 2 do
        local value =  redis.call("HINCRBY", KEYS[1], ARGV[i], tonumber(ARGV[i + 1]))
        table.insert(result, ARGV[i])
        table.insert(result, value)
    end
   	 
    return result
else
    return -1
end
`)

var tryHashIncrByWithExtendScriptV9Compat = redis.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 1 then
   local result = {}
    for i = 2, #ARGV, 2 do
        local value =  redis.call("HINCRBY", KEYS[1], ARGV[i], tonumber(ARGV[i + 1]))
        table.insert(result, ARGV[i])
        table.insert(result, value)
    end
    redis.call('expire', KEYS[1], tonumber(ARGV[1]))
    return result
else
    return -1
end
`)

var hashIncrByScriptV9Compat = redis.NewScript(`
local result = {}
for i = 1, #ARGV, 2 do
	local value =  redis.call("HINCRBY", KEYS[1], ARGV[i], tonumber(ARGV[i + 1]))
	table.insert(result, ARGV[i])
	table.insert(result, value)
end

return result
`)

var hashIncrByWithExtendScriptV9Compat = redis.NewScript(`
local result = {}
for i = 2, #ARGV, 2 do
	local value =  redis.call("HINCRBY", KEYS[1], ARGV[i], tonumber(ARGV[i + 1]))
	table.insert(result, ARGV[i])
	table.insert(result, value)
end
redis.call('expire', KEYS[1], tonumber(ARGV[1]))
return result
`)

var tryHashSetScriptV9Compat = redis.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 1 then
	local args = {}
    for i = 1, #ARGV, 2 do
        table.insert(args, ARGV[i])
        table.insert(args, ARGV[i + 1])
    end
    redis.call("HMSET", KEYS[1], unpack(args))
    return 1
else
	return -1
end
`)

var tryHashSetWithExtendScriptV9Compat = redis.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 1 then
	local args = {}
    for i = 2, #ARGV, 2 do
        table.insert(args, ARGV[i])
        table.insert(args, ARGV[i + 1])
    end
    redis.call("HMSET", KEYS[1], unpack(args))
	redis.call('expire', KEYS[1], tonumber(ARGV[1]))
	return 1
else
	return -1
end
`)

// --- 基础操作 ---

func (r *RedisV9Compat) Lock(ctx context.Context, k string, v string, ttl time.Duration) (ok bool, err error) {
	k = r.GetKey(k)
	ok, err = r.cli.SetNX(ctx, k, v, ttl).Result()
	return
}

func (r *RedisV9Compat) Unlock(ctx context.Context, k string, ranVal string) (ok bool, err error) {
	k = r.GetKey(k)
	result, err := safeDelV9Compat.Run(ctx, r.cli, []string{k}, ranVal).Result()
	if err != nil {
		return false, err
	}
	if result.(int64) == 1 {
		return true, nil
	} else {
		return false, ErrUnlockValNotMatch
	}
}

func (r *RedisV9Compat) Expire(ctx context.Context, k string, ttl time.Duration) (ok bool, err error) {
	k = r.GetKey(k)
	ok, err = r.cli.Expire(ctx, k, ttl).Result()
	return
}

func (r *RedisV9Compat) Client() redis.UniversalClient {
	return r.cli
}

func (r *RedisV9Compat) Set(ctx context.Context, k string, val interface{}, ttl time.Duration) (bool, error) {
	k = r.GetKey(k)
	result, err := r.cli.Set(ctx, k, val, ttl).Result()
	if err != nil {
		return false, err
	}
	return result == "OK", nil
}

func (r *RedisV9Compat) SetIfExists(ctx context.Context, k string, val interface{}, ttl time.Duration) (bool, error) {
	k = r.GetKey(k)
	result, err := stringSetIfExistsV9Compat.Run(ctx, r.cli, []string{k}, val, ttl).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func (r *RedisV9Compat) MGet(ctx context.Context, keys []string) ([]interface{}, error) {
	for i := range keys {
		keys[i] = r.GetKey(keys[i])
	}
	return r.cli.MGet(ctx, keys...).Result()
}

func (r *RedisV9Compat) MSet(ctx context.Context, keys []string, ttl time.Duration) (results []bool, err error) {
	pipeline := r.cli.Pipeline()
	for i := range keys {
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

func (r *RedisV9Compat) GetStr(ctx context.Context, k string) (v string, err error) {
	k = r.GetKey(k)
	return r.cli.Get(ctx, k).Result()
}

func (r *RedisV9Compat) Get(ctx context.Context, k string) (v interface{}, err error) {
	k = r.GetKey(k)
	return r.cli.Get(ctx, k).Result()
}

func (r *RedisV9Compat) Del(ctx context.Context, k string) (bool, error) {
	k = r.GetKey(k)
	result, err := r.cli.Del(ctx, k).Result()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}

func (r *RedisV9Compat) PipeDel(ctx context.Context, keys []string) error {
	p := r.cli.Pipeline()
	for i := range keys {
		p.Del(ctx, keys[i])
	}
	_, err := p.Exec(ctx)
	return err
}

func (r *RedisV9Compat) Info(ctx context.Context, section ...string) (string, error) {
	return r.cli.Info(ctx, section...).Result()
}

func (r *RedisV9Compat) Pipeline() redis.Pipeliner {
	return r.cli.Pipeline()
}

func (r *RedisV9Compat) TxPipeline() redis.Pipeliner {
	return r.cli.TxPipeline()
}

func (r *RedisV9Compat) Close() error {
	return r.cli.Close()
}

// --- Set 操作 ---

func (r *RedisV9Compat) SAdd(ctx context.Context, k string, members ...string) (rs int64, err error) {
	k = r.GetKey(k)
	iKeys := make([]interface{}, 0, len(members))
	for i := range members {
		iKeys = append(iKeys, members[i])
	}
	return r.cli.SAdd(ctx, k, iKeys...).Result()
}

func (r *RedisV9Compat) SMIsMembers(ctx context.Context, k string, members ...string) (result []bool, err error) {
	k = r.GetKey(k)
	iKeys := make([]interface{}, 0, len(members))
	for i := range members {
		iKeys = append(iKeys, members[i])
	}
	result, err = r.cli.SMIsMember(ctx, k, iKeys...).Result()
	return
}

func (r *RedisV9Compat) SRem(ctx context.Context, k string, members ...string) (rs int64, err error) {
	k = r.GetKey(k)
	iKeys := make([]interface{}, 0, len(members))
	for i := range members {
		iKeys = append(iKeys, members[i])
	}
	return r.cli.SRem(ctx, k, iKeys...).Result()
}

func (r *RedisV9Compat) SIsMember(ctx context.Context, k string, member string) (rs bool, err error) {
	k = r.GetKey(k)
	return r.cli.SIsMember(ctx, k, member).Result()
}

func (r *RedisV9Compat) SCard(ctx context.Context, k string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.SCard(ctx, k).Result()
}

func (r *RedisV9Compat) SMembers(ctx context.Context, k string) (rs []string, err error) {
	k = r.GetKey(k)
	return r.cli.SMembers(ctx, k).Result()
}

// --- Sorted Set 操作 ---

func (r *RedisV9Compat) ZAdd(ctx context.Context, k string, member string, score float64) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.ZAdd(ctx, k, redis.Z{Member: member, Score: score}).Result()
}

func (r *RedisV9Compat) ZRem(ctx context.Context, k string, member string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.ZRem(ctx, k, member).Result()
}

func (r *RedisV9Compat) ZCard(ctx context.Context, k string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.ZCard(ctx, k).Result()
}

func (r *RedisV9Compat) ZScore(ctx context.Context, k string, member string) (rs float64, err error) {
	k = r.GetKey(k)
	return r.cli.ZScore(ctx, k, member).Result()
}

func (r *RedisV9Compat) ZRange(ctx context.Context, k string, start int64, stop int64) (rs []string, err error) {
	k = r.GetKey(k)
	return r.cli.ZRange(ctx, k, start, stop).Result()
}

func (r *RedisV9Compat) ZRevRange(ctx context.Context, k string, start int64, stop int64) (rs []string, err error) {
	k = r.GetKey(k)
	return r.cli.ZRevRange(ctx, k, start, stop).Result()
}

func (r *RedisV9Compat) ZRangeWithScores(ctx context.Context, k string, start int64, stop int64) (s ZSorts, err error) {
	k = r.GetKey(k)
	rs, err := r.cli.ZRangeWithScores(ctx, k, start, stop).Result()
	if err != nil {
		return nil, err
	}
	s = make(ZSorts, 0, len(rs))
	for i := range rs {
		s = append(s, Z{Score: rs[i].Score, Member: rs[i].Member})
	}
	return s, nil
}

func (r *RedisV9Compat) ZRevRangeWithScores(ctx context.Context, k string, start int64, stop int64) (s ZSorts, err error) {
	k = r.GetKey(k)
	rs, err := r.cli.ZRevRangeWithScores(ctx, k, start, stop).Result()
	if err != nil {
		return nil, err
	}
	s = make(ZSorts, 0, len(rs))
	for i := range rs {
		s = append(s, Z{Score: rs[i].Score, Member: rs[i].Member})
	}
	return s, nil
}

func (r *RedisV9Compat) ZRangeByScore(ctx context.Context, k string, opt redis.ZRangeBy) (rs []string, err error) {
	k = r.GetKey(k)
	return r.cli.ZRangeByScore(ctx, k, &opt).Result()
}

func (r *RedisV9Compat) ZRevRangeByScore(ctx context.Context, k string, opt redis.ZRangeBy) (rs []string, err error) {
	k = r.GetKey(k)
	return r.cli.ZRevRangeByScore(ctx, k, &opt).Result()
}

func (r *RedisV9Compat) ZRangeByScoreWithScores(ctx context.Context, k string, opt redis.ZRangeBy) (s ZSorts, err error) {
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

func (r *RedisV9Compat) ZRevRangeByScoreWithScores(ctx context.Context, k string, opt redis.ZRangeBy) (rs []redis.Z, err error) {
	k = r.GetKey(k)
	return r.cli.ZRevRangeByScoreWithScores(ctx, k, &opt).Result()
}

func (r *RedisV9Compat) ZRank(ctx context.Context, k string, member string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.ZRank(ctx, k, member).Result()
}

func (r *RedisV9Compat) ZRevRank(ctx context.Context, k string, member string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.ZRevRank(ctx, k, member).Result()
}

func (r *RedisV9Compat) ZIncrBy(ctx context.Context, k string, member string, increment float64) (rs float64, err error) {
	k = r.GetKey(k)
	return r.cli.ZIncrBy(ctx, k, increment, member).Result()
}

func (r *RedisV9Compat) ZCount(ctx context.Context, k string, min string, max string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.ZCount(ctx, k, min, max).Result()
}

func (r *RedisV9Compat) ZRemRangeByRank(ctx context.Context, k string, start int64, stop int64) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.ZRemRangeByRank(ctx, k, start, stop).Result()
}

func (r *RedisV9Compat) ZRemRangeByScore(ctx context.Context, k string, min string, max string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.ZRemRangeByScore(ctx, k, min, max).Result()
}

func (r *RedisV9Compat) ZRemRangeByLex(ctx context.Context, k string, min string, max string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.ZRemRangeByLex(ctx, k, min, max).Result()
}

func (r *RedisV9Compat) ZUnionStore(ctx context.Context, dest string, store redis.ZStore) (rs int64, err error) {
	dest = r.GetKey(dest)
	return r.cli.ZUnionStore(ctx, dest, &store).Result()
}

func (r *RedisV9Compat) ZInterStore(ctx context.Context, dest string, store redis.ZStore) (rs int64, err error) {
	dest = r.GetKey(dest)
	return r.cli.ZInterStore(ctx, dest, &store).Result()
}

// --- Hash 操作 ---

func (r *RedisV9Compat) HDel(ctx context.Context, k string, fields ...string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.HDel(ctx, k, fields...).Result()
}

func (r *RedisV9Compat) HExists(ctx context.Context, k string, field string) (rs bool, err error) {
	k = r.GetKey(k)
	return r.cli.HExists(ctx, k, field).Result()
}

func (r *RedisV9Compat) Exists(ctx context.Context, k string) (exists bool, err error) {
	k = r.GetKey(k)
	status, err := r.cli.Exists(ctx, k).Result()
	if err != nil {
		return
	}
	exists = status > 0
	return
}

func (r *RedisV9Compat) HGet(ctx context.Context, k string, field string) (rs string, err error) {
	k = r.GetKey(k)
	return r.cli.HGet(ctx, k, field).Result()
}

func (r *RedisV9Compat) HGetAll(ctx context.Context, k string) (rs map[string]string, err error) {
	k = r.GetKey(k)
	return r.cli.HGetAll(ctx, k).Result()
}

func (r *RedisV9Compat) HIncrBy(ctx context.Context, k string, field string, incr int64) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.HIncrBy(ctx, k, field, incr).Result()
}

func (r *RedisV9Compat) Incr(ctx context.Context, k string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.Incr(ctx, k).Result()
}

func (r *RedisV9Compat) IncrBy(ctx context.Context, k string, incr int64) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.IncrBy(ctx, k, incr).Result()
}

func (r *RedisV9Compat) HIncrByFloat(ctx context.Context, k string, field string, incr float64) (rs float64, err error) {
	k = r.GetKey(k)
	return r.cli.HIncrByFloat(ctx, k, field, incr).Result()
}

func (r *RedisV9Compat) HKeys(ctx context.Context, k string) (rs []string, err error) {
	k = r.GetKey(k)
	return r.cli.HKeys(ctx, k).Result()
}

func (r *RedisV9Compat) HLen(ctx context.Context, k string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.HLen(ctx, k).Result()
}

func (r *RedisV9Compat) HMGet(ctx context.Context, k string, fields ...string) (rs []interface{}, err error) {
	k = r.GetKey(k)
	return r.cli.HMGet(ctx, k, fields...).Result()
}

func (r *RedisV9Compat) HMSet(ctx context.Context, k string, fields map[string]interface{}, ttl time.Duration) (rs bool, err error) {
	k = r.GetKey(k)
	if ttl > 0 {
		pairs := make([]interface{}, 0, len(fields)*2+1)
		for s := range fields {
			pairs = append(pairs, s, fields[s])
		}
		pairs = append(pairs, ttl.Seconds())
		result, err := hMSetScriptV9Compat.Run(ctx, r.cli, []string{k}, pairs...).Result()
		if err != nil {
			return false, err
		}
		return cast.ToInt(result) == 1, nil
	}
	return r.cli.HMSet(ctx, k, fields).Result()
}

func (r *RedisV9Compat) HMSetIfExists(ctx context.Context, k string, fields map[string]interface{}, ttl time.Duration) (rs bool, err error) {
	i, err := hMSetScriptV9Compat.Run(ctx, r.cli, []string{k}, fields, ttl.Seconds()).Int()
	if err != nil {
		return
	}
	return i == 1, nil
}

func (r *RedisV9Compat) HSet(ctx context.Context, k string, field string, value interface{}, ttl time.Duration) (rs bool, err error) {
	k = r.GetKey(k)
	if ttl > 0 {
		result, err := hSetScriptV9Compat.Run(ctx, r.cli, []string{k}, field, value, ttl.Seconds()).Result()
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

func (r *RedisV9Compat) HSetNX(ctx context.Context, k string, field string, value interface{}) (rs bool, err error) {
	k = r.GetKey(k)
	return r.cli.HSetNX(ctx, k, field, value).Result()
}

func (r *RedisV9Compat) HVals(ctx context.Context, k string) (rs []string, err error) {
	k = r.GetKey(k)
	return r.cli.HVals(ctx, k).Result()
}

func (r *RedisV9Compat) HScan(ctx context.Context, k string, cursor uint64, match string, count int64) (rs []string, ncursor uint64, err error) {
	k = r.GetKey(k)
	return r.cli.HScan(ctx, k, cursor, match, count).Result()
}

// --- List 操作 ---

func (r *RedisV9Compat) BLPop(ctx context.Context, timeout time.Duration, keys ...string) (rs []string, err error) {
	for i, k := range keys {
		keys[i] = r.GetKey(k)
	}
	return r.cli.BLPop(ctx, timeout, keys...).Result()
}

func (r *RedisV9Compat) BRPop(ctx context.Context, timeout time.Duration, keys ...string) (rs []string, err error) {
	for i, k := range keys {
		keys[i] = r.GetKey(k)
	}
	return r.cli.BRPop(ctx, timeout, keys...).Result()
}

func (r *RedisV9Compat) BRPopLPush(ctx context.Context, source string, destination string, timeout time.Duration) (rs string, err error) {
	source = r.GetKey(source)
	destination = r.GetKey(destination)
	return r.cli.BRPopLPush(ctx, source, destination, timeout).Result()
}

func (r *RedisV9Compat) LIndex(ctx context.Context, k string, index int64) (rs string, err error) {
	k = r.GetKey(k)
	return r.cli.LIndex(ctx, k, index).Result()
}

func (r *RedisV9Compat) LInsert(ctx context.Context, k string, op string, pivot interface{}, value interface{}) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.LInsert(ctx, k, op, pivot, value).Result()
}

func (r *RedisV9Compat) LInsertBefore(ctx context.Context, k string, pivot interface{}, value interface{}) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.LInsertBefore(ctx, k, pivot, value).Result()
}

func (r *RedisV9Compat) LInsertAfter(ctx context.Context, k string, pivot interface{}, value interface{}) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.LInsertAfter(ctx, k, pivot, value).Result()
}

func (r *RedisV9Compat) LLen(ctx context.Context, k string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.LLen(ctx, k).Result()
}

func (r *RedisV9Compat) LPop(ctx context.Context, k string) (rs string, err error) {
	k = r.GetKey(k)
	return r.cli.LPop(ctx, k).Result()
}

func (r *RedisV9Compat) LPush(ctx context.Context, k string, values ...interface{}) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.LPush(ctx, k, values...).Result()
}

func (r *RedisV9Compat) LPushX(ctx context.Context, k string, value interface{}) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.LPushX(ctx, k, value).Result()
}

func (r *RedisV9Compat) LRange(ctx context.Context, k string, start int64, stop int64) (rs []string, err error) {
	k = r.GetKey(k)
	return r.cli.LRange(ctx, k, start, stop).Result()
}

func (r *RedisV9Compat) LRem(ctx context.Context, k string, count int64, value interface{}) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.LRem(ctx, k, count, value).Result()
}

func (r *RedisV9Compat) LSet(ctx context.Context, k string, index int64, value interface{}) (rs string, err error) {
	k = r.GetKey(k)
	return r.cli.LSet(ctx, k, index, value).Result()
}

func (r *RedisV9Compat) LTrim(ctx context.Context, k string, start int64, stop int64) (rs string, err error) {
	k = r.GetKey(k)
	return r.cli.LTrim(ctx, k, start, stop).Result()
}

func (r *RedisV9Compat) RPop(ctx context.Context, k string) (rs string, err error) {
	k = r.GetKey(k)
	return r.cli.RPop(ctx, k).Result()
}

func (r *RedisV9Compat) RPopLPush(ctx context.Context, source string, destination string) (rs string, err error) {
	source = r.GetKey(source)
	destination = r.GetKey(destination)
	return r.cli.RPopLPush(ctx, source, destination).Result()
}

func (r *RedisV9Compat) RPush(ctx context.Context, k string, values ...interface{}) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.RPush(ctx, k, values...).Result()
}

func (r *RedisV9Compat) RPushX(ctx context.Context, k string, value interface{}) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.RPushX(ctx, k, value).Result()
}

// --- PipeHashGetSet ---

func (r *RedisV9Compat) PipeHashGetSet(ctx context.Context, keys []string, batchGetter BatchGetter, option *PipeHashGetSetOption) (rs *HashRs, err error) {
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
		cacheData, err1 := cmders[i].(*redis.MapStringStringCmd).Result()
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

// --- 复杂 Hash 操作（Lua 脚本） ---

func (r *RedisV9Compat) TryHashIncrBy(ctx context.Context, k string, data map[string]int64, ttl time.Duration, opt *HashOption) (rs map[string]int64, err error) {
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
		result, err = tryHashIncrByWithExtendScriptV9Compat.Run(ctx, r.cli, []string{k}, items...).Result()
	} else {
		result, err = tryHashIncrByScriptV9Compat.Run(ctx, r.cli, []string{k}, items...).Result()
	}
	if err != nil {
		return
	}
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

func (r *RedisV9Compat) TryHashSet(ctx context.Context, k string, data map[string]interface{}, ttl time.Duration, opt *HashOption) (status int8, err error) {
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
		result, err = tryHashSetWithExtendScriptV9Compat.Run(ctx, r.cli, []string{k}, items...).Result()
	} else {
		result, err = tryHashSetScriptV9Compat.Run(ctx, r.cli, []string{k}, items...).Result()
	}
	if err != nil {
		return
	}
	status = cast.ToInt8(result)
	return status, nil
}

func (r *RedisV9Compat) HashIncrBy(ctx context.Context, k string, data map[string]int64, ttl time.Duration, opt *HashOption) (rs map[string]int64, err error) {
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
		result, err = hashIncrByWithExtendScriptV9Compat.Run(ctx, r.cli, []string{k}, items...).Result()
	} else {
		result, err = hashIncrByScriptV9Compat.Run(ctx, r.cli, []string{k}, items...).Result()
	}
	if err != nil {
		return
	}
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

// --- Lock / Publish ---

func (r *RedisV9Compat) LockExec(ctx context.Context, param LockExecParam) (err error) {
	return LockExec(ctx, r, param)
}

func (r *RedisV9Compat) Publish(ctx context.Context, topic string, key string) (err error) {
	_, err = r.cli.Publish(ctx, topic, key).Result()
	return
}

// --- 日志辅助 ---

func NewRedisV9CompatFromConfig(cnf *RedisV9CompatConfig) *RedisV9Compat {
	inst := cnf.Cli
	if inst == nil {
		panic("RedisV9CompatConfig.Cli must not be nil")
	}

	fmt.Printf("[RedisV9Compat] initialized: prefix=%s\n", cnf.Prefix)

	return &RedisV9Compat{
		Base: &Base{prefix: cnf.Prefix},
		cli:  inst,
	}
}
