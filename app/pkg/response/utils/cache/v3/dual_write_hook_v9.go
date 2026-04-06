package cache

import (
	"time"

	"github.com/redis/go-redis/v9"
)

// createNewRedisClientV9 创建新 Redis 客户端（v9 版本）
func createNewRedisClientV9(cnf *RedisConfig) redis.UniversalClient {
	dialTimeout := time.Duration(cnf.DialTimeoutSec) * time.Second
	readTimeout := time.Duration(cnf.ReadTimeoutSec) * time.Second
	writeTimeout := time.Duration(cnf.WriteTimeoutSec) * time.Second

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
