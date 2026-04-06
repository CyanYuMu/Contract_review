package cache

import (
	"context"
	"github.com/go-redis/redis"
	"github.com/spf13/cast"
	"time"
)

type Redis struct {
	*Base
	cli redis.UniversalClient
}

func NewRedis(cnf *RedisConfig) *Redis {
	var inst redis.UniversalClient
	if cnf.IsCluster {
		inst = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:         cnf.Addrs,
			Password:      cnf.Password,
			PoolSize:      cnf.PoolSize,
			ReadOnly:      cnf.ReadOnly,
			RouteRandomly: cnf.RouteRandomly,
			MinIdleConns:  cnf.MinIdleConn,
		})
	} else {
		inst = redis.NewClient(&redis.Options{
			Addr:         cnf.Addrs[0],
			Password:     cnf.Password,
			DB:           cnf.DB,
			PoolSize:     cnf.PoolSize,
			MinIdleConns: cnf.MinIdleConn,
		})
	}

	return &Redis{
		Base: &Base{
			prefix: cnf.Prefix,
		},
		cli: inst,
	}
}

var hSetScript = redis.NewScript(`
local result = pcall(redis.call, 'HSET', KEYS[1], ARGV[1], ARGV[2])
if result.err then
    return result
end

redis.call('EXPIRE', KEYS[1], ARGV[3])

return 1
`)

var hMSetScript = redis.NewScript(`
local expire_time = tonumber(ARGV[#ARGV])
table.remove(ARGV, #ARGV)

local result = redis.pcall("HMSET", KEYS[1], unpack(ARGV))
if result.err then
    return result
end
redis.call("EXPIRE", KEYS[1], expire_time)
return 1
`)

var safeDel = redis.NewScript(`
local key_value = redis.call('GET', KEYS[1])

if key_value == ARGV[1] then
    redis.call('DEL', KEYS[1])
    return 1
else
    return 0
end
`)

func (r *Redis) Lock(ctx context.Context, k string, v string, ttl time.Duration) (ok bool, err error) {
	//TODO implement me
	k = r.GetKey(k)
	ok, err = r.cli.SetNX(k, v, ttl).Result()

	return
}

func (r *Redis) Unlock(ctx context.Context, k string, ranVal string) (ok bool, err error) {
	//TODO implement me
	k = r.GetKey(k)

	result, err := safeDel.Run(r.cli, []string{k}, ranVal).Result()
	if err != nil {
		return false, err
	}

	if result.(int64) == 1 {
		return true, nil
	} else {
		return false, ErrUnlockValNotMatch
	}
}

func (r *Redis) Expire(ctx context.Context, k string, ttl time.Duration) (ok bool, err error) {
	k = r.GetKey(k)
	ok, err = r.cli.Expire(k, ttl).Result()

	return
}

func (r *Redis) Client() redis.UniversalClient {
	return r.cli
}

func (r *Redis) Set(ctx context.Context, k string, val interface{}, ttl time.Duration) (bool, error) {
	k = r.GetKey(k)
	result, err := r.cli.Set(k, val, ttl).Result()
	if err != nil {
		return false, err
	}

	return result == "OK", nil
}

func (r *Redis) MGet(ctx context.Context, keys []string) ([]interface{}, error) {
	for i, _ := range keys {
		keys[i] = r.GetKey(keys[i])
	}
	return r.cli.MGet(keys...).Result()
}

func (r *Redis) MSet(ctx context.Context, keys []string, ttl time.Duration) (results []bool, err error) {
	pipeline := r.cli.Pipeline()
	for i, _ := range keys {
		pipeline.Set(r.GetKey(keys[i]), keys[i], ttl)
	}
	rs, err := pipeline.Exec()
	if err != nil {
		return nil, err
	}

	results = make([]bool, 0, len(keys))
	for i := range rs {
		results = append(results, rs[i].Err() == nil)
	}

	return results, nil
}

func (r *Redis) Get(ctx context.Context, k string) (v interface{}, err error) {
	k = r.GetKey(k)
	return r.cli.Get(k).Result()
}

func (r *Redis) GetStr(ctx context.Context, k string) (v string, err error) {
	k = r.GetKey(k)
	return r.cli.Get(k).Result()
}

func (r *Redis) Del(ctx context.Context, k string) (bool, error) {
	k = r.GetKey(k)
	result, err := r.cli.Del(k).Result()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}

func (r *Redis) Info(ctx context.Context, section ...string) (string, error) {
	return r.cli.Info(section...).Result()
}

func (r *Redis) Pipeline() redis.Pipeliner {
	return r.cli.Pipeline()
}

func (r *Redis) TxPipeline() redis.Pipeliner {
	return r.cli.TxPipeline()
}

func (r *Redis) Close() error {
	return r.cli.Close()
}

func (r *Redis) SAdd(ctx context.Context, k string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.SAdd(k).Result()
}

func (r *Redis) SRem(ctx context.Context, k string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.SRem(k).Result()
}

func (r *Redis) SIsMember(ctx context.Context, k string, member string) (rs bool, err error) {
	k = r.GetKey(k)
	return r.cli.SIsMember(k, member).Result()
}

func (r *Redis) SCard(ctx context.Context, k string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.SCard(k).Result()
}

func (r *Redis) SMembers(ctx context.Context, k string) (rs []string, err error) {
	k = r.GetKey(k)
	return r.cli.SMembers(k).Result()
}

func (r *Redis) ZAdd(ctx context.Context, k string, member string, score float64) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.ZAdd(k, redis.Z{Member: member, Score: score}).Result()
}

func (r *Redis) ZRem(ctx context.Context, k string, member string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.ZRem(k, member).Result()
}

func (r *Redis) ZCard(ctx context.Context, k string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.ZCard(k).Result()
}

func (r *Redis) ZScore(ctx context.Context, k string, member string) (rs float64, err error) {
	k = r.GetKey(k)
	return r.cli.ZScore(k, member).Result()
}

func (r *Redis) ZRange(ctx context.Context, k string, start int64, stop int64) (rs []string, err error) {
	k = r.GetKey(k)
	return r.cli.ZRange(k, start, stop).Result()
}

func (r *Redis) ZRevRange(ctx context.Context, k string, start int64, stop int64) (rs []string, err error) {
	k = r.GetKey(k)
	return r.cli.ZRevRange(k, start, stop).Result()
}

func (r *Redis) ZRangeWithScores(ctx context.Context, k string, start int64, stop int64) (s ZSorts, err error) {
	k = r.GetKey(k)

	rs, err := r.cli.ZRangeWithScores(k, start, stop).Result()
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

func (r *Redis) ZRevRangeWithScores(ctx context.Context, k string, start int64, stop int64) (s ZSorts, err error) {
	k = r.GetKey(k)

	rs, err := r.cli.ZRevRangeWithScores(k, start, stop).Result()
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

func (r *Redis) ZRangeByScore(ctx context.Context, k string, opt redis.ZRangeBy) (rs []string, err error) {
	k = r.GetKey(k)
	return r.cli.ZRangeByScore(k, opt).Result()
}

func (r *Redis) ZRevRangeByScore(ctx context.Context, k string, opt redis.ZRangeBy) (rs []string, err error) {
	k = r.GetKey(k)
	return r.cli.ZRevRangeByScore(k, opt).Result()
}

func (r *Redis) ZRangeByScoreWithScores(ctx context.Context, k string, opt redis.ZRangeBy) (s ZSorts, err error) {
	k = r.GetKey(k)
	rs, err := r.cli.ZRangeByScoreWithScores(k, opt).Result()
	if err != nil {
		return nil, err
	}
	s = make(ZSorts, 0, len(rs))
	for i, _ := range rs {
		s = append(s, Z{Member: rs[i].Member, Score: rs[i].Score})
	}

	return s, nil
}

func (r *Redis) ZRevRangeByScoreWithScores(ctx context.Context, k string, opt redis.ZRangeBy) (rs []redis.Z, err error) {
	k = r.GetKey(k)
	return r.cli.ZRevRangeByScoreWithScores(k, opt).Result()
}

func (r *Redis) ZRank(ctx context.Context, k string, member string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.ZRank(k, member).Result()
}

func (r *Redis) ZRevRank(ctx context.Context, k string, member string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.ZRevRank(k, member).Result()
}

func (r *Redis) ZIncrBy(ctx context.Context, k string, member string, increment float64) (rs float64, err error) {
	k = r.GetKey(k)
	return r.cli.ZIncrBy(k, increment, member).Result()
}

func (r *Redis) ZCount(ctx context.Context, k string, min string, max string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.ZCount(k, min, max).Result()
}

func (r *Redis) ZRemRangeByRank(ctx context.Context, k string, start int64, stop int64) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.ZRemRangeByRank(k, start, stop).Result()
}

func (r *Redis) ZRemRangeByScore(ctx context.Context, k string, min string, max string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.ZRemRangeByScore(k, min, max).Result()
}

func (r *Redis) ZRemRangeByLex(ctx context.Context, k string, min string, max string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.ZRemRangeByLex(k, min, max).Result()
}

func (r *Redis) ZUnionStore(ctx context.Context, dest string, store redis.ZStore, keys ...string) (rs int64, err error) {
	dest = r.GetKey(dest)
	for i, k := range keys {
		keys[i] = r.GetKey(k)
	}
	return r.cli.ZUnionStore(dest, store, keys...).Result()
}

func (r *Redis) ZInterStore(ctx context.Context, dest string, store redis.ZStore, keys ...string) (rs int64, err error) {
	dest = r.GetKey(dest)
	for i, k := range keys {
		keys[i] = r.GetKey(k)
	}
	return r.cli.ZInterStore(dest, store, keys...).Result()
}

func (r *Redis) HDel(ctx context.Context, k string, fields ...string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.HDel(k, fields...).Result()
}

func (r *Redis) HExists(ctx context.Context, k string, field string) (rs bool, err error) {
	k = r.GetKey(k)
	return r.cli.HExists(k, field).Result()
}

func (r *Redis) HGet(ctx context.Context, k string, field string) (rs string, err error) {
	k = r.GetKey(k)
	return r.cli.HGet(k, field).Result()
}

func (r *Redis) HGetAll(ctx context.Context, k string) (rs map[string]string, err error) {
	k = r.GetKey(k)
	return r.cli.HGetAll(k).Result()
}

func (r *Redis) HIncrBy(ctx context.Context, k string, field string, incr int64) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.HIncrBy(k, field, incr).Result()
}

func (r *Redis) HIncrByFloat(ctx context.Context, k string, field string, incr float64) (rs float64, err error) {
	k = r.GetKey(k)
	return r.cli.HIncrByFloat(k, field, incr).Result()
}

func (r *Redis) HKeys(ctx context.Context, k string) (rs []string, err error) {
	k = r.GetKey(k)
	return r.cli.HKeys(k).Result()
}

func (r *Redis) HLen(ctx context.Context, k string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.HLen(k).Result()
}

func (r *Redis) HMGet(ctx context.Context, k string, fields ...string) (rs []interface{}, err error) {
	k = r.GetKey(k)
	return r.cli.HMGet(k, fields...).Result()

}

func (r *Redis) HMSet(ctx context.Context, k string, fields map[string]interface{}, ttl time.Duration) (ok bool, err error) {
	k = r.GetKey(k)
	if ttl > 0 {
		pairs := make([]interface{}, 0, len(fields)*2+1)
		for i := range fields {
			pairs = append(pairs, i, fields[i])
		}
		pairs = append(pairs, ttl.Seconds())
		result, err := hMSetScript.Run(r.cli, []string{k}, pairs...).Result()
		if err != nil {
			return false, err
		}

		return cast.ToInt(result) == 1, nil
	}

	result, err := r.cli.HMSet(k, fields).Result()
	if err != nil {
		return false, err
	}

	return result == "OK", nil
}

func (r *Redis) HSet(ctx context.Context, k string, field string, value interface{}, ttl time.Duration) (rs bool, err error) {
	k = r.GetKey(k)
	if ttl > 0 {
		result, err := hSetScript.Run(r.cli, []string{k}, field, value, ttl.Seconds()).Result()
		if err != nil {
			return false, err
		}

		return result.(int64) == 1, nil
	} else {
		return r.cli.HSet(k, field, value).Result()
	}
}

func (r *Redis) HSetNX(ctx context.Context, k string, field string, value interface{}) (rs bool, err error) {
	k = r.GetKey(k)
	return r.cli.HSetNX(k, field, value).Result()
}

func (r *Redis) HVals(ctx context.Context, k string) (rs []string, err error) {
	k = r.GetKey(k)
	return r.cli.HVals(k).Result()
}

func (r *Redis) HScan(ctx context.Context, k string, cursor uint64, match string, count int64) (rs []string, ncursor uint64, err error) {
	k = r.GetKey(k)
	return r.cli.HScan(k, cursor, match, count).Result()
}

func (r *Redis) BLPop(ctx context.Context, timeout time.Duration, keys ...string) (rs []string, err error) {
	for i, k := range keys {
		keys[i] = r.GetKey(k)
	}
	return r.cli.BLPop(timeout, keys...).Result()
}

func (r *Redis) BRPop(ctx context.Context, timeout time.Duration, keys ...string) (rs []string, err error) {
	for i, k := range keys {
		keys[i] = r.GetKey(k)
	}
	return r.cli.BRPop(timeout, keys...).Result()
}

func (r *Redis) BRPopLPush(ctx context.Context, source string, destination string, timeout time.Duration) (rs string, err error) {
	source = r.GetKey(source)
	destination = r.GetKey(destination)
	return r.cli.BRPopLPush(source, destination, timeout).Result()
}

func (r *Redis) LIndex(ctx context.Context, k string, index int64) (rs string, err error) {
	k = r.GetKey(k)
	return r.cli.LIndex(k, index).Result()
}

func (r *Redis) LInsert(ctx context.Context, k string, op string, pivot interface{}, value interface{}) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.LInsert(k, op, pivot, value).Result()
}

func (r *Redis) LInsertBefore(ctx context.Context, k string, pivot interface{}, value interface{}) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.LInsertBefore(k, pivot, value).Result()
}

func (r *Redis) LInsertAfter(ctx context.Context, k string, pivot interface{}, value interface{}) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.LInsertAfter(k, pivot, value).Result()
}

func (r *Redis) LLen(ctx context.Context, k string) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.LLen(k).Result()
}

func (r *Redis) LPop(ctx context.Context, k string) (rs string, err error) {
	k = r.GetKey(k)
	return r.cli.LPop(k).Result()
}

func (r *Redis) LPush(ctx context.Context, k string, values ...interface{}) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.LPush(k, values...).Result()
}

func (r *Redis) LPushX(ctx context.Context, k string, value interface{}) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.LPushX(k, value).Result()
}

func (r *Redis) LRange(ctx context.Context, k string, start int64, stop int64) (rs []string, err error) {
	k = r.GetKey(k)
	return r.cli.LRange(k, start, stop).Result()
}

func (r *Redis) LRem(ctx context.Context, k string, count int64, value interface{}) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.LRem(k, count, value).Result()
}

func (r *Redis) LSet(ctx context.Context, k string, index int64, value interface{}) (rs string, err error) {
	k = r.GetKey(k)
	return r.cli.LSet(k, index, value).Result()
}

func (r *Redis) LTrim(ctx context.Context, k string, start int64, stop int64) (rs string, err error) {
	k = r.GetKey(k)
	return r.cli.LTrim(k, start, stop).Result()
}

func (r *Redis) RPop(ctx context.Context, k string) (rs string, err error) {
	k = r.GetKey(k)
	return r.cli.RPop(k).Result()
}

func (r *Redis) RPopLPush(ctx context.Context, source string, destination string) (rs string, err error) {
	source = r.GetKey(source)
	destination = r.GetKey(destination)
	return r.cli.RPopLPush(source, destination).Result()
}

func (r *Redis) RPush(ctx context.Context, k string, values ...interface{}) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.RPush(k, values...).Result()
}

func (r *Redis) RPushX(ctx context.Context, k string, value interface{}) (rs int64, err error) {
	k = r.GetKey(k)
	return r.cli.RPushX(k, value).Result()
}
