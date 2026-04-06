package cache

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9" // 更新为 v9
	"github.com/spf13/cast"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/cache/dualwrite"
)

// RedisV9 结构体定义
type RedisV9 struct {
	prefix string
	inst   redis.UniversalClient
}

var (
	SafeDelV9             = redis.NewScript(safeDelScript)
	HashMSetScriptV9      = redis.NewScript(hMSetScript)
	IncrByScriptV9        = redis.NewScript(incrByScript)
	HashIncrByScriptV9    = redis.NewScript(hashIncrByScript)
	StringSetScriptV9     = redis.NewScript(stringSetScript)
	ZsetScript            = redis.NewScript(zsetScript)
	ListPushScriptV9      = redis.NewScript(listPushScript)
	SetScriptV9           = redis.NewScript(setScript)
	ZsetAddFixLenScriptV9 = redis.NewScript(zsetAddFixLenScript)
	SetBitsScriptV9       = redis.NewScript(setBitsScript)
)

// NewRedisV9 创建实例
func NewRedisV9(cnf *RedisConfig) Rds {
	var inst redis.UniversalClient

	// 创建旧库客户端（使用主配置）
	oldCli := createRedisClientV9(cnf)

	// 检测双写配置
	if cnf.DualWrite != nil && cnf.DualWrite.Enabled && cnf.DualWrite.NewRedis != nil {
		// 创建新库客户端
		newCli := createNewRedisClientV9(cnf.DualWrite.NewRedis)

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
		hook := dualwrite.NewHookV9(dualWriteTarget, &cnf.DualWrite.Config)
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
		fmt.Printf("[DualWrite] Redis initialization: version=v3/v9, prefix=%s, oldRedis=%v (mode=%s), newRedis=%v (mode=%s), enabled=%v, syncRunning=%v, readFrom=%s, primaryClient=%s\n",
			cnf.Prefix, cnf.Addrs, oldMode, cnf.DualWrite.NewRedis.Addrs, newMode,
			cnf.DualWrite.Enabled, cnf.DualWrite.SyncRunning, cnf.DualWrite.ReadFrom, primaryClient)
	} else {
		// 无双写配置，直接使用旧库
		inst = oldCli
		oldMode := "standalone"
		if cnf.IsCluster {
			oldMode = "cluster"
		}
		fmt.Printf("[DualWrite] Redis initialization: version=v3/v9, prefix=%s, oldRedis=%v (mode=%s), dualWriteEnabled=false\n",
			cnf.Prefix, cnf.Addrs, oldMode)
	}

	// 预加载 Lua 脚本（避免 Pipeline 中 EVALSHA 失败）
	_ = SetBitsScriptV9.Load(context.Background(), inst)

	return &RedisV9{
		prefix: cnf.Prefix,
		inst:   inst,
	}
}

// createRedisClientV9 创建 Redis 客户端（使用主配置）
func createRedisClientV9(cnf *RedisConfig) redis.UniversalClient {
	dialTimeout := time.Duration(cnf.DialTimeoutSec) * time.Second
	readTimeout := time.Duration(cnf.ReadTimeoutSec) * time.Second
	writeTimeout := time.Duration(cnf.WriteTimeoutSec) * time.Second

	if cnf.Cli != nil {
		return cnf.Cli.V9
	}

	if cnf.IsCluster {
		return redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:         cnf.Addrs,
			Password:      cnf.Password,
			PoolSize:      cnf.PoolSize,
			ReadOnly:      cnf.ReadOnly,
			RouteRandomly: cnf.RouteRandomly,
			MinIdleConns:  cnf.MinIdleConn,
			PoolFIFO:      cnf.PoolFIFO,
			DialTimeout:   dialTimeout,
			ReadTimeout:   readTimeout,
			WriteTimeout:  writeTimeout,
			PoolTimeout:   time.Duration(cnf.PoolTimeoutSec) * time.Second,
		})
	}

	return redis.NewClient(&redis.Options{
		Addr:         cnf.Addrs[0],
		Password:     cnf.Password,
		DB:           cnf.DB,
		PoolSize:     cnf.PoolSize,
		MinIdleConns: cnf.MinIdleConn,
		DialTimeout:  dialTimeout,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		PoolFIFO:     cnf.PoolFIFO,
		PoolTimeout:  time.Duration(cnf.PoolTimeoutSec) * time.Second,
	})
}

// Lock implements distributed locking
func (r *RedisV9) Lock(ctx context.Context, k string, v string, ttl time.Duration) (bool, error) {
	return r.inst.SetNX(ctx, r.getKey(k), v, ttl).Result()
}

// Unlock releases a distributed lock
func (r *RedisV9) Unlock(ctx context.Context, k string, ranVal string) (bool, error) {

	result, err := SafeDelV9.Run(ctx, r.inst, []string{r.getKey(k)}, ranVal).Result()
	if err != nil {
		return false, err
	}
	return result.(int64) == 1, nil
}

// Client returns the underlying Redis client
func (r *RedisV9) Client() Client {
	return Client{V9: r.inst}
}

// Close closes the Redis connection
func (r *RedisV9) Close() error {
	return r.inst.Close()
}

// Pipeline executes commands in a pipeline
func (r *RedisV9) PipeExec(ctx context.Context, handler PipeExecHandler) error {
	err := handler(ctx, &Client{V9: r.inst})
	return err
}

// Set sets a key-value pair with optional expiration
func (r *RedisV9) Set(ctx context.Context, k string, val interface{}, ttl time.Duration, opt ...*StringSetOption) (bool, error) {
	key := r.getKey(k)
	var setOpt redis.SetArgs
	var extendTTL bool

	if len(opt) > 0 && opt[0] != nil {
		if opt[0].Mode != "" {
			setOpt.Mode = opt[0].Mode
		}
		extendTTL = opt[0].ExtendTtl
	}

	if ttl > 0 {
		setOpt.TTL = ttl
	}

	result, err := r.inst.SetArgs(ctx, key, val, setOpt).Result()
	if err != nil {
		return false, err
	}
	if extendTTL && ttl > 0 {
		r.inst.Expire(ctx, key, ttl)
	}

	return result == "OK", nil
}

// SetBit sets or clears a bit at offset in the string value stored at key
func (r *RedisV9) SetBit(ctx context.Context, k string, offset int64, val int) (bool, error) {
	result, err := r.inst.SetBit(ctx, r.getKey(k), offset, val).Result()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

// SetBits 批量设置多个位，仅在 key 无过期时间时设置 TTL
func (r *RedisV9) SetBits(ctx context.Context, k string, ttl time.Duration, positions []uint64) (bool, error) {
	if len(positions) == 0 {
		return true, nil
	}

	// 构建参数: TTL(秒), positions...
	args := make([]interface{}, 0, len(positions)+1)
	args = append(args, int64(ttl.Seconds()))
	for _, pos := range positions {
		args = append(args, pos)
	}

	_, err := SetBitsScriptV9.Run(ctx, r.inst, []string{r.getKey(k)}, args...).Result()
	if err != nil {
		return false, err
	}
	return true, nil
}

// GetBitCount gets the number of bits set to 1 in the string value stored at key
func (r *RedisV9) BitCount(ctx context.Context, k string, opt *BitCount) (int64, error) {
	var options *redis.BitCount
	if opt != nil {
		options = &redis.BitCount{
			Start: opt.Start,
			End:   opt.End,
			Unit:  opt.Unit,
		}
	}
	return r.inst.BitCount(ctx, r.getKey(k), options).Result()
}

func (r *RedisV9) BitInfo(ctx context.Context, k string) (*BitInfo, error) {
	res, err := r.inst.Do(ctx, "BF.INFO", r.getKey(k)).Result()
	if err != nil {
		return nil, err
	}

	var capacity, kNum, mSize int64

	switch v := res.(type) {
	case []interface{}:
		for i := 0; i+1 < len(v); i += 2 {
			key, _ := v[i].(string)
			val := cast.ToInt64(v[i+1])
			switch strings.ToLower(key) {
			case "capacity":
				capacity = val
			case "k", "number of hashes", "number of hash functions":
				kNum = val
			case "m", "size", "bytes":
				mSize = val
			}
		}
	case map[interface{}]interface{}:
		for kk, vv := range v {
			key := strings.ToLower(cast.ToString(kk))
			val := cast.ToInt64(vv)
			switch key {
			case "capacity":
				capacity = val
			case "k", "number of hashes", "number of hash functions":
				kNum = val
			case "m", "size", "bytes":
				mSize = val
			}
		}
	case map[string]interface{}:
		for kk, vv := range v {
			key := strings.ToLower(kk)
			val := cast.ToInt64(vv)
			switch key {
			case "capacity":
				capacity = val
			case "k", "number of hashes", "number of hash functions":
				kNum = val
			case "m", "size", "bytes":
				mSize = val
			}
		}
	case string:
		return nil, fmt.Errorf("unexpected BF.INFO string response: %q", v)
	default:
		return nil, fmt.Errorf("unsupported BF.INFO response type: %T", v)
	}

	return &BitInfo{Capacity: capacity, K: kNum, M: mSize}, nil
}

// GetSet sets a new value and returns the old value
func (r *RedisV9) GetSet(ctx context.Context, k string, val interface{}) (string, error) {
	return r.inst.GetSet(ctx, r.getKey(k), val).Result()
}

// Get retrieves a value by key
func (r *RedisV9) GetString(ctx context.Context, k string) (string, error) {
	return r.inst.Get(ctx, r.getKey(k)).Result()
}

func (r *RedisV9) Get(ctx context.Context, k string) (interface{}, error) {
	return r.inst.Get(ctx, r.getKey(k)).Result()
}

func (r *RedisV9) TTL(ctx context.Context, k string) (time.Duration, error) {
	return r.inst.TTL(ctx, r.getKey(k)).Result()
}

// Del deletes a key
func (r *RedisV9) Del(ctx context.Context, k string) (bool, error) {
	result, err := r.inst.Del(ctx, r.getKey(k)).Result()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}

// MDel deletes multiple keys
func (r *RedisV9) MDel(ctx context.Context, keys ...string) (int64, error) {
	return r.inst.Del(ctx, keys...).Result()
}

// Exists checks if a key exists
func (r *RedisV9) Exists(ctx context.Context, k string) (bool, error) {
	result, err := r.inst.Exists(ctx, r.getKey(k)).Result()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}

// Expire sets a key's time to live in seconds
func (r *RedisV9) Expire(ctx context.Context, k string, ttl time.Duration) (bool, error) {
	return r.inst.Expire(ctx, r.getKey(k), ttl).Result()
}

// getKey prefixes the key with the configured prefix
func (r *RedisV9) getKey(k string) string {
	if r.prefix != "" {
		return r.prefix + ":" + k
	}
	return k
}

// HSet sets field in the hash stored at key to value
func (r *RedisV9) HSet(ctx context.Context, k string, field string, value interface{}, opt ...*HashOption) (bool, error) {
	key := r.getKey(k)

	if len(opt) > 0 {
		args := opt[0].Args()
		args = append(args, field, value)
		_, err := HashMSetScriptV9.Run(ctx, r.inst, []string{key}, args...).Result()
		if err != nil {
			return false, err
		}
		return true, nil
	} else {
		result, err := r.inst.HSet(ctx, key, field, value).Result()
		if err != nil {
			return false, err
		}
		return result > 0, nil
	}
}

// HGet gets the value of a hash field
func (r *RedisV9) HGet(ctx context.Context, k string, field string) (string, error) {
	return r.inst.HGet(ctx, r.getKey(k), field).Result()
}

// HDel deletes one or more hash fields
func (r *RedisV9) HDel(ctx context.Context, k string, fields ...string) (int64, error) {
	return r.inst.HDel(ctx, r.getKey(k), fields...).Result()
}

// HExists determines if a hash field exists
func (r *RedisV9) HExists(ctx context.Context, k string, field string) (bool, error) {
	return r.inst.HExists(ctx, r.getKey(k), field).Result()
}

// HGetAll gets all the fields and values in a hash
func (r *RedisV9) HGetAll(ctx context.Context, k string) (map[string]string, error) {
	return r.inst.HGetAll(ctx, r.getKey(k)).Result()
}

// HMGet gets the values of all the given hash fields
func (r *RedisV9) HMGet(ctx context.Context, k string, fields []string) ([]interface{}, error) {
	return r.inst.HMGet(ctx, r.getKey(k), fields...).Result()
}

// HMSet sets multiple hash fields to multiple values
func (r *RedisV9) HMSet(ctx context.Context, k string, memKV map[string]string, opt ...*HashOption) (bool, error) {
	key := r.getKey(k)

	if len(opt) > 0 {
		args := opt[0].Args()
		for field, value := range memKV {
			args = append(args, field, value)
		}

		_, err := HashMSetScriptV9.Run(ctx, r.inst, []string{key}, args...).Result()
		if err != nil {
			return false, err
		}
		return true, nil
	}

	args := []interface{}{}
	for field, value := range memKV {
		args = append(args, field, value)
	}

	err := r.inst.HMSet(ctx, key, args...).Err()
	if err != nil {
		return false, err
	}

	return true, nil
}

// HIncrBy increments the integer value of a hash field by the given number
func (r *RedisV9) HIncrBy(ctx context.Context, k string, field string, incr int64, opt ...*HashOption) (int64, error) {
	key := r.getKey(k)

	if len(opt) > 0 {
		args := opt[0].Args()
		args = append(args, field, incr)
		result, err := HashIncrByScriptV9.Run(ctx, r.inst, []string{key}, args...).Result()
		if err != nil {
			return 0, err
		}
		return result.(int64), nil
	}

	result, err := r.inst.HIncrBy(ctx, key, field, incr).Result()
	if err != nil {
		return 0, err
	}

	return result, nil
}

// HIncrByFloat increments the float value of a hash field by the given amount
func (r *RedisV9) HIncrByFloat(ctx context.Context, k string, field string, incr float64, opt ...*HashOption) (float64, error) {
	key := r.getKey(k)

	if len(opt) > 0 {
		args := opt[0].Args()
		args = append(args, field, incr)
		result, err := HashIncrByScriptV9.Run(ctx, r.inst, []string{key}, args...).Result()
		if err != nil {
			return 0, err
		}
		return result.(float64), nil
	}

	result, err := r.inst.HIncrByFloat(ctx, key, field, incr).Result()
	if err != nil {
		return 0, err
	}

	return result, nil
}

// LPush inserts all the specified values at the head of the list
func (r *RedisV9) LPush(ctx context.Context, k string, values []interface{}, opt ...*ListOption) (int64, error) {
	key := r.getKey(k)
	if len(opt) > 0 {
		args := opt[0].Args()
		args[3] = "LEFT"
		args = append(args, values...)
		result, err := ListPushScriptV9.Run(ctx, r.inst, []string{key}, args...).Result()
		if err != nil {
			return 0, err
		}
		return result.(int64), nil
	}
	return r.inst.LPush(ctx, key, values...).Result()
}

// RPush inserts all the specified values at the tail of the list
func (r *RedisV9) RPush(ctx context.Context, k string, values []interface{}, opt ...*ListOption) (int64, error) {
	key := r.getKey(k)
	if len(opt) > 0 {
		args := opt[0].Args()
		args[3] = "RIGHT"
		args = append(args, values...)
		result, err := ListPushScriptV9.Run(ctx, r.inst, []string{key}, args...).Result()
		if err != nil {
			return 0, err
		}
		return result.(int64), nil
	}
	return r.inst.RPush(ctx, key, values...).Result()
}

// LPop removes and returns the first element of the list
func (r *RedisV9) LPop(ctx context.Context, k string) (string, error) {
	return r.inst.LPop(ctx, r.getKey(k)).Result()
}

// LPopCount removes and returns the first count elements of the list
func (r *RedisV9) LPopCount(ctx context.Context, k string, count int) ([]string, error) {
	return r.inst.LPopCount(ctx, r.getKey(k), count).Result()
}

// RPop removes and returns the last element of the list
func (r *RedisV9) RPop(ctx context.Context, k string) (string, error) {
	return r.inst.RPop(ctx, r.getKey(k)).Result()
}

// RPopCount removes and returns the last count elements of the list
func (r *RedisV9) RPopCount(ctx context.Context, k string, count int) ([]string, error) {
	return r.inst.RPopCount(ctx, r.getKey(k), count).Result()
}

// LLen returns the length of the list
func (r *RedisV9) LLen(ctx context.Context, k string) (int64, error) {
	return r.inst.LLen(ctx, r.getKey(k)).Result()
}

// LRange returns the specified elements of the list
func (r *RedisV9) LRange(ctx context.Context, k string, start, stop int64) ([]string, error) {
	return r.inst.LRange(ctx, r.getKey(k), start, stop).Result()
}

// SAdd adds one or more members to a set
func (r *RedisV9) SAdd(ctx context.Context, k string, members []interface{}, opt ...*SetOption) (int64, error) {
	key := r.getKey(k)

	if len(opt) > 0 {
		args := opt[0].Args()
		args = append(args, members...)
		result, err := SetScriptV9.Run(ctx, r.inst, []string{key}, args...).Result()
		if err != nil {
			return 0, err
		}
		return result.(int64), nil
	}
	return r.inst.SAdd(ctx, key, members...).Result()
}

// SRem removes one or more members from a set
func (r *RedisV9) SRem(ctx context.Context, k string, members []interface{}) (int64, error) {
	return r.inst.SRem(ctx, r.getKey(k), members...).Result()
}

// SMembers returns all the members of the set
func (r *RedisV9) SMembers(ctx context.Context, k string) ([]string, error) {
	return r.inst.SMembers(ctx, r.getKey(k)).Result()
}

func (r *RedisV9) SIsMember(ctx context.Context, k string, member interface{}) (bool, error) {
	return r.inst.SIsMember(ctx, r.getKey(k), member).Result()
}

// SCard returns the cardinality (number of elements) of the set
func (r *RedisV9) SCard(ctx context.Context, k string) (int64, error) {
	return r.inst.SCard(ctx, r.getKey(k)).Result()
}

// ZAdd adds a member to a sorted set, or updates its score if it already exists
func (r *RedisV9) ZAdd(ctx context.Context, k string, mem Z, opt ...*ZsetOption) (int64, error) {
	return r.ZMAdd(ctx, k, []Z{mem}, opt...)
}

func (r *RedisV9) ZCard(ctx context.Context, k string) (int64, error) {
	return r.inst.ZCard(ctx, r.getKey(k)).Result()
}

// ZMAdd adds multiple members to a sorted set
func (r *RedisV9) ZMAdd(ctx context.Context, k string, mems []Z, opt ...*ZsetOption) (int64, error) {
	if len(opt) > 0 {
		args := opt[0].Args()
		for _, mem := range mems {
			args = append(args, mem.Score, mem.Member)
		}
		result, err := ZsetScript.Run(ctx, r.inst, []string{r.getKey(k)}, args...).Result()
		if err != nil {
			return 0, err
		}
		return result.(int64), nil
	}
	zs := make([]redis.Z, len(mems))
	for i, mem := range mems {
		zs[i] = redis.Z{
			Score:  mem.Score,
			Member: mem.Member,
		}
	}
	return r.inst.ZAdd(ctx, r.getKey(k), zs...).Result()
}

// ZRem removes one or more members from a sorted set
func (r *RedisV9) ZRem(ctx context.Context, k string, members ...string) (int64, error) {
	return r.inst.ZRem(ctx, r.getKey(k), members).Result()
}

// ZScore returns the score of member in the sorted set
func (r *RedisV9) ZScore(ctx context.Context, k string, member string) (float64, error) {
	return r.inst.ZScore(ctx, r.getKey(k), member).Result()
}

// ZRange returns the specified range of elements in the sorted set
func (r *RedisV9) ZRange(ctx context.Context, k string, start, stop int64) ([]string, error) {
	return r.inst.ZRange(ctx, r.getKey(k), start, stop).Result()
}

// ZRevRange returns the specified range of elements in the sorted set, ordered from highest to lowest score
func (r *RedisV9) ZRevRange(ctx context.Context, k string, start, stop int64) ([]string, error) {
	return r.inst.ZRevRange(ctx, r.getKey(k), start, stop).Result()
}

// ZRangeByScore returns the specified range of elements in the sorted set, ordered from lowest to highest score
func (r *RedisV9) ZRangeByScore(ctx context.Context, k string, start, stop int64, opts ...*ZRangeOption) ([]string, error) {
	var offset, count int64
	if len(opts) > 0 {
		offset = opts[0].GetOffset()
		count = opts[0].GetCount()
	}
	return r.inst.ZRangeByScore(ctx, r.getKey(k), &redis.ZRangeBy{
		Min:    cast.ToString(start),
		Max:    cast.ToString(stop),
		Offset: offset,
		Count:  count,
	}).Result()
}

// ZRevRangeByScore returns the specified range of elements in the sorted set, ordered from highest to lowest score
func (r *RedisV9) ZRevRangeByScore(ctx context.Context, k string, start, stop int64, opts ...*ZRangeOption) ([]string, error) {
	var offset, count int64
	if len(opts) > 0 {
		offset = opts[0].GetOffset()
		count = opts[0].GetCount()
	}
	return r.inst.ZRevRangeByScore(ctx, r.getKey(k), &redis.ZRangeBy{
		Min:    cast.ToString(start),
		Max:    cast.ToString(stop),
		Offset: offset,
		Count:  count,
	}).Result()
}

// ZRangeWithScores returns the specified range of elements in the sorted set, ordered from lowest to highest score
func (r *RedisV9) ZRangeWithScores(ctx context.Context, k string, start, stop int64) (ZSorts, error) {
	set, err := r.inst.ZRangeWithScores(ctx, r.getKey(k), start, stop).Result()
	if err != nil {
		return nil, err
	}
	zs := make([]Z, len(set))
	for i, z := range set {
		zs[i] = Z{Score: z.Score, Member: z.Member}
	}
	return zs, nil
}

// ZRevRangeWithScores returns the specified range of elements in the sorted set, ordered from highest to lowest score
func (r *RedisV9) ZRevRangeWithScores(ctx context.Context, k string, start, stop int64) (ZSorts, error) {
	set, err := r.inst.ZRevRangeWithScores(ctx, r.getKey(k), start, stop).Result()
	if err != nil {
		return nil, err
	}
	zs := make([]Z, len(set))
	for i, z := range set {
		zs[i] = Z{Score: z.Score, Member: z.Member}
	}
	return zs, nil
}

// ZCount returns the number of elements in the sorted set with scores within the given range
func (r *RedisV9) ZCount(ctx context.Context, k string, start, stop int64) (int64, error) {
	return r.inst.ZCount(ctx, r.getKey(k), cast.ToString(start), cast.ToString(stop)).Result()
}

// ZRemRangeByScore removes all elements in the sorted set with scores within the given range
func (r *RedisV9) ZRemRangeByScore(ctx context.Context, k string, start, stop int64) (int64, error) {
	return r.inst.ZRemRangeByScore(ctx, r.getKey(k), cast.ToString(start), cast.ToString(stop)).Result()
}

// ZRemRangeByRank removes all elements in the sorted set with ranks within the given range
func (r *RedisV9) ZRemRangeByRank(ctx context.Context, k string, start, stop int64) (int64, error) {
	return r.inst.ZRemRangeByRank(ctx, r.getKey(k), start, stop).Result()
}

// Publish publishes a message to a channel
func (r *RedisV9) Publish(ctx context.Context, topic string, msg string) error {
	return r.inst.Publish(ctx, topic, msg).Err()
}

func (r *RedisV9) Subscribe(ctx context.Context, channels ...string) (<-chan Message, error) {
	sub := r.inst.Subscribe(ctx, channels...)

	ch := make(chan Message)
	go func() {
		for msg := range sub.Channel() {
			ch <- Message{
				Channel: msg.Channel,
				Payload: msg.Payload,
			}
		}
	}()

	r.inst.Pipeline()

	return ch, nil
}

func (r *RedisV9) Pipeline() Pipeline {
	return NewV9Pipeline(r.inst.Pipeline())
}

func (r *RedisV9) Incr(ctx context.Context, k string) (int64, error) {
	return r.inst.Incr(ctx, r.getKey(k)).Result()
}

func (r *RedisV9) IncrBy(ctx context.Context, k string, incr int64) (int64, error) {
	return r.inst.IncrBy(ctx, r.getKey(k), incr).Result()
}

func (r *RedisV9) Count(ctx context.Context, key string) (count int, err error) {
	return r.inst.Get(ctx, key).Int()
}

func (r *RedisV9) ZAddFixLen(ctx context.Context, key string, members []Z, maxLength int, ttl time.Duration, opt ...*FixLenOptions) error {
	var autoExtend int8
	if len(opt) > 0 {
		if opt[0].ExtendTtl {
			autoExtend = 1
		}
	}
	args := make([]interface{}, 0, len(members)*2+3)
	for _, member := range members {
		args = append(args, member.Member, member.Score)
	}
	args = append(args, maxLength, ttl.Seconds(), autoExtend)
	return ZsetAddFixLenScriptV9.Run(ctx, r.inst, []string{r.getKey(key)}, args...).Err()
}
