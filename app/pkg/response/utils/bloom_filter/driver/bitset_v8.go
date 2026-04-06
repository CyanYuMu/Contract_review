package driver

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/spf13/cast"
)

var luaExistsCheckScriptV8 = redis.NewScript(bloomFilterExistsCheckScript)
var luaMSExistsCheckScriptV8 = redis.NewScript(bloomFilterMSExistsCheckScript)
var luaAddScriptV8 = redis.NewScript(bloomFilterAddScript)
var luaInsertScriptV8 = redis.NewScript(bloomFilterInsertScript)

type BitsetV8 struct {
	cap   uint
	p     float64
	redis redis.UniversalClient
	m     uint
	k     uint
}

func (b *BitsetV8) init() error {
	b.cap = getCap(b.cap)
	b.p = getErrorRatio(b.p)

	b.m = uint(math.Ceil(-1 * float64(b.cap) * math.Log(b.p) / math.Pow(math.Log(2), 2)))
	b.k = uint(math.Ceil(math.Log(2) * float64(b.m) / float64(b.cap)))

	if b.redis == nil {
		return errors.New("redis instance is nil")
	}

	return nil
}

type BitsetV8Conf struct {
	// redis的key
	//Key string
	// redis cli 实例
	Redis redis.UniversalClient
	// 过期时间, 可不填, 不填即不过期
	//Ttl time.Duration
	// 容量
	Cap uint
	// 误判率, 如: 0.00000001
	//1% error rate requires 7 hash functions and 10.08 bits per item.
	//0.1% error rate requires 10 hash functions and 14.4 bits per item.
	//0.01% error rate requires 14 hash functions and 20.16 bits per item
	//细节参考: https://redis.io/commands/bf.add/
	P float64
}

func NewBitsetV8(conf *BitsetV8Conf) (*BitsetV8, error) {
	b := &BitsetV8{
		cap:   conf.Cap,
		redis: conf.Redis,
		p:     conf.P,
	}

	err := b.init()
	return b, err
}

// location returns the ith hashed location using the four base hash values
func (b *BitsetV8) location(h [4]uint64, i uint) uint64 {
	return Location(h, i) % uint64(b.m)
}

func (b *BitsetV8) getLocation(key []byte) []interface{} {
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

func (b *BitsetV8) Exists(ctx context.Context, key string, item string) (exists bool, err error) {
	locations := b.getLocation([]byte(item))
	rs := luaExistsCheckScriptV8.Eval(ctx, b.redis, []string{key}, locations...)
	if rs.Err() != nil {
		return false, rs.Err()
	}

	v, err := rs.Int()
	if err != nil {
		return false, err
	}

	return v == 1, nil
}

func (b *BitsetV8) MSMExists(ctx context.Context, key string, items []string) (statusList []bool, err error) {
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
	rs := luaMSExistsCheckScriptV8.Eval(ctx, b.redis, []string{key}, locations...)
	if rs.Err() != nil {
		return nil, rs.Err()
	}

	v := rs.Val()
	if items, ok := v.([]interface{}); ok {
		for i := range items {
			statusList = append(statusList, cast.ToInt(items[i]) == 1)
		}

		return statusList, nil
	} else {
		return nil, ErrNotExists
	}
}

func (b *BitsetV8) MExists(ctx context.Context, key string, items []string) (statusList []bool, err error) {
	//TODO implement me
	for i, _ := range items {
		exists, err := b.Exists(ctx, key, items[i])
		if err != nil {
			return nil, err
		}
		statusList = append(statusList, exists)
	}

	return statusList, nil
}

func (b *BitsetV8) Add(ctx context.Context, key string, item string, ttl time.Duration) (ok bool, err error) {
	//TODO implement me
	locations := make([]interface{}, 0, b.k+1)
	locations = append(locations, ttl.Seconds())
	locations = append(locations, b.getLocation([]byte(item))...)
	rs := luaAddScriptV8.Eval(ctx, b.redis, []string{key}, locations...)
	if rs.Err() != nil {
		return false, rs.Err()
	}

	v, err := rs.Int()
	if err != nil {
		return false, err
	}

	return v == 1, nil
}

func (b *BitsetV8) MAdd(ctx context.Context, key string, items []string, ttl time.Duration) (statusList []bool, err error) {
	//TODO implement me
	for i, _ := range items {
		ok, err := b.Add(ctx, key, items[i], ttl)
		if err != nil {
			return nil, err
		}
		statusList = append(statusList, ok)
	}

	return statusList, nil
}

func (b *BitsetV8) Info(ctx context.Context, key string) (info map[string]interface{}, err error) {
	//TODO implement me
	return map[string]interface{}{
		"cap": b.cap,
		"k":   b.k,
		"m":   b.m,
	}, nil
}

func (b *BitsetV8) Insert(ctx context.Context, key string, item string, ttl time.Duration) (ok bool, err error) {
	//TODO implement me

	//TODO implement me
	locations := make([]interface{}, 0, b.k+1)
	locations = append(locations, ttl.Seconds())
	locations = append(locations, b.getLocation([]byte(item))...)
	rs := luaInsertScriptV8.Eval(ctx, b.redis, []string{key}, locations...)
	if rs.Err() != nil {
		return false, rs.Err()
	}

	v, err := rs.Int()
	if err != nil {
		return false, err
	}

	return v == 1, nil
}

func (b *BitsetV8) MInsert(ctx context.Context, key string, items []string, ttl time.Duration) (statusList []bool, err error) {
	for i, _ := range items {
		ok, err := b.Insert(ctx, key, items[i], ttl)
		if err != nil {
			return nil, err
		}
		statusList = append(statusList, ok)
	}

	return statusList, nil
}

func (b *BitsetV8) Clear(ctx context.Context, key string) (err error) {
	// 删除Redis中的布隆过滤器key
	return b.redis.Del(ctx, key).Err()
}
