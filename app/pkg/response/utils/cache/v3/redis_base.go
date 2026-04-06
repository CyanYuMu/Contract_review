package cache

import (
	"context"
	"errors"
	"time"

	v8Redis "github.com/go-redis/redis/v8"
	v9Redis "github.com/redis/go-redis/v9"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/cache/dualwrite"
)

type Base struct {
	prefix string
}

func (b *Base) GetKey(k string) string {
	if b.prefix != "" {
		k = b.prefix + ":" + k
	}

	//if len(k) > 64 {
	//	t := sha1.New()
	//	_, _ = io.WriteString(t, k)
	//	k = fmt.Sprintf("%x", t.Sum(nil))
	//}

	return k
}

type ZSorts []Z

type Z struct {
	Score  float64
	Member interface{}
}

func (z ZSorts) GetScore(mem interface{}) float64 {
	for _, v := range z {
		if v.Member == mem {
			return v.Score
		}
	}

	return 0
}

var ErrUnlockValNotMatch = errors.New("unlock failed value not match")

const EmptyString = "_empty_"

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
	// 自定义客户端, 当设置此项时, 会直接引用此client作为客户端
	Cli *Client
	// 双写配置
	DualWrite *DualWriteConfig `yaml:"dualWrite"`
}

// DualWriteConfig 双写配置
type DualWriteConfig struct {
	dualwrite.Config `yaml:",inline"`           // 内嵌公共配置
	NewRedis         *RedisConfig `yaml:"newRedis"` // 新 Redis 配置
}

// Client represents either a v8 or v9 Redis client
type Client struct {
	V8 v8Redis.UniversalClient
	V9 v9Redis.UniversalClient
}

type ZsetOption struct {
	XX        bool
	NX        bool
	ExtendTtl bool
	Ttl       time.Duration
}

func (o *ZsetOption) Args() []interface{} {
	args := []interface{}{0, 0, 0}
	if o.XX {
		args[0] = 1
	} else if o.NX {
		args[0] = 2
	}
	if o.ExtendTtl {
		args[1] = 1
	}
	if o.Ttl > 0 {
		args[2] = o.Ttl.Seconds()
	}
	return args
}

type HashOption struct {
	// 是否延长ttl
	ExtendTtl bool
	// 过期时间
	Ttl time.Duration
	// 是否仅在记录存在时, 指定操作
	XX bool
}

func (t *HashOption) Apply(opt *HashOption) {
	if opt == nil {
		return
	}

	t.ExtendTtl = opt.ExtendTtl
}

func (o *HashOption) Args() []interface{} {
	args := []interface{}{0, 0, 0}
	if o.XX {
		args[0] = 1
	}
	if o.ExtendTtl {
		args[1] = 1
	}
	if o.Ttl > 0 {
		args[2] = o.Ttl.Seconds()
	}

	return args
}

// func dftTryOption() *HashOption {
// 	return &HashOption{}
// }

type StringSetOption struct {
	// 模式 NX XX
	Mode string
	// 是否延长ttl
	ExtendTtl bool
}

// func (o *StringSetOption) Args() []interface{} {
// 	args := []interface{}{0, 0, 0}
// 	if o.NX {
// 		args[0] = 1
// 	}
// 	if o.ExtendTtl {
// 		args[1] = 1
// 	}
// 	return args
// }

type SetOption struct {
	XX        bool
	ExtendTtl bool
	Ttl       time.Duration
}

func (o *SetOption) Args() []interface{} {
	args := []interface{}{0, 0, 0}
	if o.XX {
		args[0] = 1
	}
	if o.ExtendTtl {
		args[1] = 1
	}

	if o.Ttl > 0 {
		args[2] = o.Ttl.Seconds()
	}

	return args
}

type ListOption struct {
	XX        bool
	ExtendTtl bool
	Ttl       time.Duration
}

func (o *ListOption) Args() []interface{} {
	args := []interface{}{0, 0, 0, "LEFT"}
	if o.XX {
		args[0] = 1
	}
	if o.ExtendTtl {
		args[1] = 1
	}

	if o.Ttl > 0 {
		args[2] = o.Ttl.Seconds()
	}

	return args
}

type ZRangeOption struct {
	Offset int64
	Count  int64
}

func (o *ZRangeOption) Args() []interface{} {
	args := []interface{}{0, 0}
	if o.Offset != 0 {
		args[0] = o.Offset
	}
	if o.Count != 0 {
		args[1] = o.Count
	}
	return args
}

func (z *ZRangeOption) GetOffset() int64 {
	if z == nil {
		return 0
	}

	return z.Offset
}

func (z *ZRangeOption) GetCount() int64 {
	if z == nil {
		return 0
	}

	return z.Count
}

type BitCount struct {
	Start, End int64
	Unit       string // BYTE(default) | BIT
}

type BitInfo struct {
	Capacity int64
	K        int64
	M        int64
}

type Cmd interface {
	// 基础操作
	Set(ctx context.Context, k string, val interface{}, ttl time.Duration, opt ...*StringSetOption) (bool, error)
	SetBit(ctx context.Context, k string, offset int64, val int) (bool, error)
	SetBits(ctx context.Context, k string, ttl time.Duration, positions []uint64) (bool, error)
	BitCount(ctx context.Context, k string, opt *BitCount) (int64, error)
	// BF.INFO key 获取布隆过滤器信息
	BitInfo(ctx context.Context, k string) (*BitInfo, error)
	GetSet(ctx context.Context, k string, val interface{}) (string, error)
	Get(ctx context.Context, k string) (interface{}, error)
	GetString(ctx context.Context, k string) (string, error)
	TTL(ctx context.Context, k string) (time.Duration, error)
	Del(ctx context.Context, k string) (bool, error)
	MDel(ctx context.Context, keys ...string) (int64, error)
	Exists(ctx context.Context, k string) (bool, error)
	Expire(ctx context.Context, k string, ttl time.Duration) (bool, error)
	Incr(ctx context.Context, k string) (int64, error)
	IncrBy(ctx context.Context, k string, incr int64) (int64, error)

	Count(ctx context.Context, key string) (count int, err error)

	// Hash操作
	HSet(ctx context.Context, k string, field string, value interface{}, opt ...*HashOption) (bool, error)
	HGet(ctx context.Context, k string, field string) (string, error)
	HDel(ctx context.Context, k string, fields ...string) (int64, error)
	HExists(ctx context.Context, k string, field string) (bool, error)
	HGetAll(ctx context.Context, k string) (map[string]string, error)
	HMGet(ctx context.Context, k string, fields []string) ([]interface{}, error)
	HMSet(ctx context.Context, k string, values map[string]string, opt ...*HashOption) (bool, error)
	HIncrBy(ctx context.Context, k string, field string, incr int64, opt ...*HashOption) (int64, error)
	HIncrByFloat(ctx context.Context, k string, field string, incr float64, opt ...*HashOption) (float64, error)

	// List操作
	LPush(ctx context.Context, k string, values []interface{}, opt ...*ListOption) (int64, error)
	RPush(ctx context.Context, k string, values []interface{}, opt ...*ListOption) (int64, error)
	LPop(ctx context.Context, k string) (string, error)
	LPopCount(ctx context.Context, k string, count int) ([]string, error)
	RPop(ctx context.Context, k string) (string, error)
	RPopCount(ctx context.Context, k string, count int) ([]string, error)
	LLen(ctx context.Context, k string) (int64, error)
	LRange(ctx context.Context, k string, start, stop int64) ([]string, error)

	// Set操作
	SAdd(ctx context.Context, k string, members []interface{}, opt ...*SetOption) (int64, error)
	SRem(ctx context.Context, k string, members []interface{}) (int64, error)
	SMembers(ctx context.Context, k string) ([]string, error)
	SCard(ctx context.Context, k string) (int64, error)
	SIsMember(ctx context.Context, k string, member interface{}) (bool, error)

	// Sorted Set操作
	ZAdd(ctx context.Context, k string, mem Z, opt ...*ZsetOption) (int64, error)
	ZMAdd(ctx context.Context, k string, mems []Z, opt ...*ZsetOption) (int64, error)
	ZRem(ctx context.Context, k string, members ...string) (int64, error)
	ZScore(ctx context.Context, k string, member string) (float64, error)
	ZRange(ctx context.Context, k string, start, stop int64) ([]string, error)
	ZRevRange(ctx context.Context, k string, start, stop int64) ([]string, error)
	// 发布订阅
	ZRangeByScore(ctx context.Context, k string, start, stop int64, opt ...*ZRangeOption) ([]string, error)
	ZRevRangeByScore(ctx context.Context, k string, start, stop int64, opt ...*ZRangeOption) ([]string, error)
	ZRangeWithScores(ctx context.Context, k string, start, stop int64) (ZSorts, error)
	ZRevRangeWithScores(ctx context.Context, k string, start, stop int64) (ZSorts, error)
	ZCount(ctx context.Context, k string, start, stop int64) (int64, error)
	ZRemRangeByScore(ctx context.Context, k string, start, stop int64) (int64, error)
	ZRemRangeByRank(ctx context.Context, k string, start, stop int64) (int64, error)
	ZCard(ctx context.Context, k string) (int64, error)
	Publish(ctx context.Context, topic string, msg string) error
	Subscribe(ctx context.Context, channels ...string) (<-chan Message, error)
	// ttl 是过期时间 > 0 就会设置过期时间, 超过maxLength 会删除score最小的成员
	ZAddFixLen(ctx context.Context, key string, members []Z, maxLength int, ttl time.Duration, opt ...*FixLenOptions) error
	Pipeline() Pipeline
}

type FixLenOptions struct {
	// 如果记录已存在，主动延长过期时间
	ExtendTtl bool
}

type PipeExecHandler func(ctx context.Context, cli *Client) error

// Store defines the interface for Redis operations
type Rds interface {
	Cmd
	// 锁相关
	Lock(ctx context.Context, k string, v string, ttl time.Duration) (bool, error)
	Unlock(ctx context.Context, k string, ranVal string) (bool, error)
	// 工具方法
	Client() Client
	Close() error

	PipeExec(ctx context.Context, fn PipeExecHandler) error
}

type Message struct {
	Channel      string
	Pattern      string
	Payload      string
	PayloadSlice []string
}
