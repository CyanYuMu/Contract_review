package driver

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/spf13/cast"
)

var luaExistsCheckScriptV9 = redis.NewScript(bloomFilterExistsCheckScript)
var luaMSExistsCheckScriptV9 = redis.NewScript(bloomFilterMSExistsCheckScript)
var luaAddScriptV9 = redis.NewScript(bloomFilterAddScript)
var luaInsertScriptV9 = redis.NewScript(bloomFilterInsertScript)

type BitsetV9 struct {
	cap   uint
	p     float64
	redis redis.UniversalClient
	m     uint
	k     uint
}

func (b *BitsetV9) init() error {
	b.cap = getCap(b.cap)
	b.p = getErrorRatio(b.p)

	b.m = uint(math.Ceil(-1 * float64(b.cap) * math.Log(b.p) / math.Pow(math.Log(2), 2)))
	b.k = uint(math.Ceil(math.Log(2) * float64(b.m) / float64(b.cap)))

	if b.redis == nil {
		return errors.New("redis instance is nil")
	}

	return nil
}

type BitsetV9Conf struct {
	// redis cli 实例
	Redis redis.UniversalClient
	// 容量
	Cap uint
	// 误判率, 如: 0.00000001
	//1% error rate requires 7 hash functions and 10.08 bits per item.
	//0.1% error rate requires 10 hash functions and 14.4 bits per item.
	//0.01% error rate requires 14 hash functions and 20.16 bits per item
	//细节参考: https://redis.io/commands/bf.add/
	P float64
}

func NewBitsetV9(conf *BitsetV9Conf) (*BitsetV9, error) {
	b := &BitsetV9{
		cap:   conf.Cap,
		redis: conf.Redis,
		p:     conf.P,
	}

	err := b.init()
	return b, err
}

// location returns the ith hashed location using the four base hash values
func (b *BitsetV9) location(h [4]uint64, i uint) uint64 {
	return Location(h, i) % uint64(b.m)
}

func (b *BitsetV9) getLocation(key []byte) []interface{} {
	var d digest128 // murmur hashing
	hash1, hash2, hash3, hash4 := d.sum256(key)
	hashList := [4]uint64{hash1, hash2, hash3, hash4}
	rs := make([]interface{}, b.k)
	var i uint = 0
	for ; i < b.k; i++ {
		rs[i] = b.location(hashList, i)
	}

	return rs
}

func (b *BitsetV9) Exists(ctx context.Context, key string, item string) (exists bool, err error) {
	locations := b.getLocation([]byte(item))
	rs, err := luaExistsCheckScriptV9.Run(ctx, b.redis, []string{key}, locations...).Result()
	if err != nil {
		return false, err
	}

	v, ok := rs.(int64)
	if !ok {
		return false, errors.New("unexpected response format")
	}

	return v == 1, nil
}

func (b *BitsetV9) MSMExists(ctx context.Context, key string, items []string) (statusList []bool, err error) {
	// 从对象池获取大切片，避免频繁内存分配
	requiredCap := len(items)*int(b.k) + 1
	locations := GetLargeLocationSlice(requiredCap)
	defer ReleaseLargeLocationSlice(locations)

	locations = append(locations, 0) // 占位符，后面会设置为 cnt
	var cnt int
	for i := range items {
		curLoc := b.getLocation([]byte(items[i]))
		cnt = len(curLoc)
		locations = append(locations, curLoc...)
	}
	locations[0] = cnt
	rs, err := luaMSExistsCheckScriptV9.Run(ctx, b.redis, []string{key}, locations...).Result()
	if err != nil {
		return nil, err
	}

	if items, ok := rs.([]interface{}); ok {
		for i := range items {
			statusList = append(statusList, cast.ToInt64(items[i]) == 1)
		}
		return statusList, nil
	} else {
		return nil, ErrNotExists
	}
}

func (b *BitsetV9) MExists(ctx context.Context, key string, items []string) (statusList []bool, err error) {
	statusList = make([]bool, 0, len(items))
	for _, item := range items {
		exists, err := b.Exists(ctx, key, item)
		if err != nil {
			return nil, err
		}
		statusList = append(statusList, exists)
	}

	return statusList, nil
}

func (b *BitsetV9) Add(ctx context.Context, key string, item string, ttl time.Duration) (ok bool, err error) {
	locations := make([]interface{}, 0, b.k+1)
	locations = append(locations, ttl.Seconds())
	locations = append(locations, b.getLocation([]byte(item))...)
	rs, err := luaAddScriptV9.Run(ctx, b.redis, []string{key}, locations...).Result()
	if err != nil {
		return false, err
	}

	v, ok := rs.(int64)
	if !ok {
		return false, errors.New("unexpected response format")
	}

	return v == 1, nil
}

func (b *BitsetV9) MAdd(ctx context.Context, key string, items []string, ttl time.Duration) (statusList []bool, err error) {
	statusList = make([]bool, 0, len(items))
	for _, item := range items {
		ok, err := b.Add(ctx, key, item, ttl)
		if err != nil {
			return nil, err
		}
		statusList = append(statusList, ok)
	}

	return statusList, nil
}

func (b *BitsetV9) Info(ctx context.Context, key string) (info map[string]interface{}, err error) {
	return map[string]interface{}{
		"cap": b.cap,
		"k":   b.k,
		"m":   b.m,
	}, nil
}

func (b *BitsetV9) Insert(ctx context.Context, key string, item string, ttl time.Duration) (ok bool, err error) {
	locations := make([]interface{}, 0, b.k+1)
	locations = append(locations, ttl.Seconds())
	locations = append(locations, b.getLocation([]byte(item))...)
	rs, err := luaInsertScriptV9.Run(ctx, b.redis, []string{key}, locations...).Result()
	if err != nil {
		return false, err
	}

	v, ok := rs.(int64)
	if !ok {
		return false, errors.New("unexpected response format")
	}

	return v == 1, nil
}

func (b *BitsetV9) MInsert(ctx context.Context, key string, items []string, ttl time.Duration) (statusList []bool, err error) {
	statusList = make([]bool, 0, len(items))
	for _, item := range items {
		ok, err := b.Insert(ctx, key, item, ttl)
		if err != nil {
			return nil, err
		}
		statusList = append(statusList, ok)
	}

	return statusList, nil
}

func (b *BitsetV9) Clear(ctx context.Context, key string) (err error) {
	// 删除Redis中的布隆过滤器key
	return b.redis.Del(ctx, key).Err()
}
