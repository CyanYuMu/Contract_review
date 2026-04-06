package cache

import (
	"sync"
	time "time"

	"github.com/go-redis/redis/v8"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/cache/common"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/cache/driver"
)

type Type int

const (
	TypeRedis   Type = 1
	TypeGoCache Type = 2
	TypeRedisV8 Type = 3
)

type Cache interface {
	Set(k string, v interface{}, ttl time.Duration) (ok bool, err error)
	Get(k string) (val interface{}, err error)
	Del(k string) (ok bool, err error)
	Expire(k string, ttl time.Duration) (ok bool, err error)
	Incr(k string) (n int64, err error)
	Decr(k string) (n int64, err error)
	IncrBy(k string, v int64) (n int64, err error)
	DecrBy(k string, v int64) (n int64, err error)
	TTL(k string) (ttl time.Duration, err error)
	Lock(k string, v string, ttl time.Duration) (ok bool, err error)
	Unlock(k string, randVal string) (ok bool, err error)
	Exists(k string) (exists bool, err error)
	LockExec(param common.LockExecParam) (err error)
	GetInst() interface{}
	HGetAll(key string) (data map[string]string, err error)
	HGet(key string, field string) (rs string, err error)
	HMGet(key string, fields []string) (rs interface{}, err error)
	HDel(key string, fields []string) (err error)
	HMSet(key string, data map[string]interface{}) (err error)
}

var defaultRedisInst Cache
var defaultRedisV8Inst Cache
var defaultGoCacheInst Cache
var redisOnce sync.Once
var goCacheOnce sync.Once

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
}

// InitRedis
// @description 使用前需要进行初始化
func InitRedis(conf *RedisConfig) error {
	if defaultRedisInst == nil {
		redisOnce.Do(func() {
			defaultRedisInst = NewRedis(conf)
		})
	}

	return nil
}

// InitRedisV8
// @description 使用前需要进行初始化
func InitRedisV8(conf *RedisConfig) error {
	if defaultRedisInst == nil {
		redisOnce.Do(func() {
			defaultRedisV8Inst = NewRedisV8(conf)
		})
	}

	return nil
}

type GoCacheConfig struct {
	// 单位秒
	DefaultTtlSec int `yaml:"defaultTtlSec"`
	// 单位秒
	CleanIntervalSec int `yaml:"cleanIntervalSec"`
}

// InitGoCache
// @description 需要在项目中先进行初始化
func InitGoCache(conf *GoCacheConfig) error {
	if defaultGoCacheInst == nil {
		goCacheOnce.Do(func() {
			defaultGoCacheInst = NewGoCache(conf)
		})
	}

	return nil
}

func NewRedis(conf *RedisConfig) Cache {
	inst := driver.NewRedisCache(driver.RedisCacheConfig{
		Prefix:        conf.Prefix,
		Addrs:         conf.Addrs,
		Password:      conf.Password,
		DB:            conf.DB,
		PoolSize:      conf.PoolSize,
		IsCluster:     conf.IsCluster,
		RouteRandomly: conf.RouteRandomly,
		MinIdleConn:   conf.MinIdleConn,
	})

	return inst
}

func NewRedisV8(conf *RedisConfig) Cache {
	inst := driver.NewRedisV8(driver.RedisCacheConfig{
		Prefix:          conf.Prefix,
		Addrs:           conf.Addrs,
		Password:        conf.Password,
		DB:              conf.DB,
		PoolSize:        conf.PoolSize,
		IsCluster:       conf.IsCluster,
		ReadOnly:        conf.ReadOnly,
		RouteRandomly:   conf.RouteRandomly,
		MinIdleConn:     conf.MinIdleConn,
		DialTimeoutSec:  conf.DialTimeoutSec,
		ReadTimeoutSec:  conf.ReadTimeoutSec,
		WriteTimeoutSec: conf.WriteTimeoutSec,
		// PoolFIFO uses FIFO mode for each node connection pool GET/PUT (default LIFO).
		PoolFIFO: conf.PoolFIFO,
		// 最大生命周期, 超过释放回收
		MaxConnAgeSec: conf.MaxConnAgeSec,
		// 连接池最大超时时间, 超过重置
		PoolTimeoutSec: conf.PoolTimeoutSec,
		// 最大空闲时间, 超过释放
		IdleTimeoutSec: conf.IdleTimeoutSec,
		// 检测空闲连接频率, 默认一分钟
		IdleCheckFrequencySec: conf.IdleCheckFrequencySec,
	})

	return inst
}

func NewGoCache(conf *GoCacheConfig) Cache {
	inst := driver.NewGoCache(driver.GoCacheConfig{
		DefaultTtlSec:    conf.DefaultTtlSec,
		CleanIntervalSec: conf.CleanIntervalSec,
	})

	return inst
}

func Get(t Type) Cache {
	if t == TypeRedis {
		return defaultRedisInst
	} else if t == TypeGoCache {
		return defaultGoCacheInst
	} else if t == TypeRedisV8 {
		return defaultRedisV8Inst
	} else {
		return nil
	}
}
