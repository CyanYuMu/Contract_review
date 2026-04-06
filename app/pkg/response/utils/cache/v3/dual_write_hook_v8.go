package cache

import (
	"time"

	"github.com/go-redis/redis/v8"
)

// createRedisClientV8 创建 Redis 客户端（使用主配置，v8 版本）
func createRedisClientV8(cnf *RedisConfig) redis.UniversalClient {
	dialTimeout := time.Duration(cnf.DialTimeoutSec) * time.Second
	readTimeout := time.Duration(cnf.ReadTimeoutSec) * time.Second
	writeTimeout := time.Duration(cnf.WriteTimeoutSec) * time.Second

	if cnf.Cli != nil {
		return cnf.Cli.V8
	}

	if cnf.IsCluster {
		return redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:              cnf.Addrs,
			Password:           cnf.Password,
			PoolSize:           cnf.PoolSize,
			ReadOnly:           cnf.ReadOnly,
			RouteRandomly:      cnf.RouteRandomly,
			MinIdleConns:       cnf.MinIdleConn,
			PoolFIFO:           cnf.PoolFIFO,
			DialTimeout:        dialTimeout,
			ReadTimeout:        readTimeout,
			WriteTimeout:       writeTimeout,
			MaxConnAge:         time.Duration(cnf.MaxConnAgeSec) * time.Second,
			PoolTimeout:        time.Duration(cnf.PoolTimeoutSec) * time.Second,
			IdleTimeout:        time.Duration(cnf.IdleTimeoutSec) * time.Second,
			IdleCheckFrequency: time.Duration(cnf.IdleCheckFrequencySec) * time.Second,
		})
	}

	return redis.NewClient(&redis.Options{
		Addr:               cnf.Addrs[0],
		Password:           cnf.Password,
		DB:                 cnf.DB,
		PoolSize:           cnf.PoolSize,
		MinIdleConns:       cnf.MinIdleConn,
		PoolFIFO:           cnf.PoolFIFO,
		DialTimeout:        dialTimeout,
		ReadTimeout:        readTimeout,
		WriteTimeout:       writeTimeout,
		MaxConnAge:         time.Duration(cnf.MaxConnAgeSec) * time.Second,
		PoolTimeout:        time.Duration(cnf.PoolTimeoutSec) * time.Second,
		IdleTimeout:        time.Duration(cnf.IdleTimeoutSec) * time.Second,
		IdleCheckFrequency: time.Duration(cnf.IdleCheckFrequencySec) * time.Second,
	})
}

// createNewRedisClientV8 创建新 Redis 客户端（v8 版本）
func createNewRedisClientV8(cnf *RedisConfig) redis.UniversalClient {
	dialTimeout := time.Duration(cnf.DialTimeoutSec) * time.Second
	readTimeout := time.Duration(cnf.ReadTimeoutSec) * time.Second
	writeTimeout := time.Duration(cnf.WriteTimeoutSec) * time.Second

	if cnf.IsCluster {
		return redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:              cnf.Addrs,
			Password:           cnf.Password,
			PoolSize:           cnf.PoolSize,
			ReadOnly:           cnf.ReadOnly,
			RouteRandomly:      cnf.RouteRandomly,
			MinIdleConns:       cnf.MinIdleConn,
			PoolFIFO:           cnf.PoolFIFO,
			DialTimeout:        dialTimeout,
			ReadTimeout:        readTimeout,
			WriteTimeout:       writeTimeout,
			MaxConnAge:         time.Duration(cnf.MaxConnAgeSec) * time.Second,
			PoolTimeout:        time.Duration(cnf.PoolTimeoutSec) * time.Second,
			IdleTimeout:        time.Duration(cnf.IdleTimeoutSec) * time.Second,
			IdleCheckFrequency: time.Duration(cnf.IdleCheckFrequencySec) * time.Second,
		})
	}

	return redis.NewClient(&redis.Options{
		Addr:               cnf.Addrs[0],
		Password:           cnf.Password,
		DB:                 cnf.DB,
		PoolSize:           cnf.PoolSize,
		MinIdleConns:       cnf.MinIdleConn,
		PoolFIFO:           cnf.PoolFIFO,
		DialTimeout:        dialTimeout,
		ReadTimeout:        readTimeout,
		WriteTimeout:       writeTimeout,
		MaxConnAge:         time.Duration(cnf.MaxConnAgeSec) * time.Second,
		PoolTimeout:        time.Duration(cnf.PoolTimeoutSec) * time.Second,
		IdleTimeout:        time.Duration(cnf.IdleTimeoutSec) * time.Second,
		IdleCheckFrequency: time.Duration(cnf.IdleCheckFrequencySec) * time.Second,
	})
}
