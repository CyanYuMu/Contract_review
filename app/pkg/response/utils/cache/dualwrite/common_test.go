package dualwrite

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ============================================================
// 命令分类测试
// ============================================================

func TestIsReadCommand(t *testing.T) {
	readCmds := []string{
		"GET", "MGET", "HGET", "HMGET", "HGETALL", "HKEYS", "HVALS", "HLEN", "HEXISTS",
		"LRANGE", "LINDEX", "LLEN",
		"SMEMBERS", "SISMEMBER", "SCARD",
		"ZRANGE", "ZREVRANGE", "ZSCORE", "ZRANK", "ZCARD", "ZCOUNT",
		"EXISTS", "TYPE", "TTL", "PTTL", "KEYS", "SCAN",
		"GETBIT", "BITCOUNT", "PFCOUNT",
		"INFO", "DBSIZE",
	}
	for _, cmd := range readCmds {
		assert.True(t, IsReadCommand(cmd), "expected %s to be read command", cmd)
	}
	// 大小写不敏感
	assert.True(t, IsReadCommand("get"))
	assert.True(t, IsReadCommand("Get"))
}

func TestIsReadCommand_WriteCmds(t *testing.T) {
	writeCmds := []string{"SET", "DEL", "HSET", "INCR", "LPUSH", "SADD", "ZADD"}
	for _, cmd := range writeCmds {
		assert.False(t, IsReadCommand(cmd), "expected %s to NOT be read command", cmd)
	}
}

func TestIsIdempotentCommand(t *testing.T) {
	idempotent := []string{
		"SET", "SETEX", "PSETEX", "SETNX",
		"HSET", "HMSET", "HSETNX",
		"SADD", "ZADD",
		"DEL", "UNLINK", "HDEL", "SREM", "ZREM",
		"ZREMRANGEBYRANK", "ZREMRANGEBYSCORE",
		"EXPIRE", "EXPIREAT", "PEXPIRE", "PEXPIREAT", "PERSIST",
		"RENAME", "RENAMENX",
		"SCRIPT",
	}
	for _, cmd := range idempotent {
		assert.True(t, IsIdempotentCommand(cmd), "expected %s to be idempotent", cmd)
	}
	// 大小写不敏感
	assert.True(t, IsIdempotentCommand("set"))
	assert.True(t, IsIdempotentCommand("hSet"))
}

func TestIsIdempotentCommand_NonIdempotent(t *testing.T) {
	nonIdempotent := []string{
		"INCR", "INCRBY", "DECR", "DECRBY",
		"HINCRBY", "HINCRBYFLOAT",
		"LPUSH", "RPUSH", "LPOP", "RPOP",
		"ZINCRBY", "APPEND", "GETSET",
	}
	for _, cmd := range nonIdempotent {
		assert.False(t, IsIdempotentCommand(cmd), "expected %s to NOT be idempotent", cmd)
	}
}

// ============================================================
// ShouldWriteToNew 核心策略测试
// ============================================================

func TestShouldWriteToNew_ReadCommand(t *testing.T) {
	// 读命令永远不双写，无论配置如何
	configs := []*Config{
		{SyncRunning: true},
		{SyncRunning: false},
	}
	for _, cfg := range configs {
		assert.False(t, ShouldWriteToNew("GET", cfg))
		assert.False(t, ShouldWriteToNew("HGETALL", cfg))
		assert.False(t, ShouldWriteToNew("SMEMBERS", cfg))
		assert.False(t, ShouldWriteToNew("ZRANGE", cfg))
	}
}

func TestShouldWriteToNew_SyncRunning_Idempotent(t *testing.T) {
	// 增量同步运行中 + 幂等命令 → 双写
	cfg := &Config{SyncRunning: true}
	assert.True(t, ShouldWriteToNew("SET", cfg))
	assert.True(t, ShouldWriteToNew("HSET", cfg))
	assert.True(t, ShouldWriteToNew("DEL", cfg))
	assert.True(t, ShouldWriteToNew("SADD", cfg))
	assert.True(t, ShouldWriteToNew("ZADD", cfg))
	assert.True(t, ShouldWriteToNew("EXPIRE", cfg))
}

func TestShouldWriteToNew_SyncRunning_NonIdempotent(t *testing.T) {
	// 增量同步运行中 + 非幂等命令 → 不双写（由增量同步处理）
	cfg := &Config{SyncRunning: true}
	assert.False(t, ShouldWriteToNew("INCR", cfg))
	assert.False(t, ShouldWriteToNew("INCRBY", cfg))
	assert.False(t, ShouldWriteToNew("LPUSH", cfg))
	assert.False(t, ShouldWriteToNew("RPUSH", cfg))
	assert.False(t, ShouldWriteToNew("ZINCRBY", cfg))
	assert.False(t, ShouldWriteToNew("APPEND", cfg))
}

func TestShouldWriteToNew_SyncStopped_AllWriteCommands(t *testing.T) {
	// 增量同步已停止 → 所有写命令都双写
	cfg := &Config{SyncRunning: false}
	// 幂等命令
	assert.True(t, ShouldWriteToNew("SET", cfg))
	assert.True(t, ShouldWriteToNew("HSET", cfg))
	assert.True(t, ShouldWriteToNew("DEL", cfg))
	// 非幂等命令也要双写
	assert.True(t, ShouldWriteToNew("INCR", cfg))
	assert.True(t, ShouldWriteToNew("INCRBY", cfg))
	assert.True(t, ShouldWriteToNew("LPUSH", cfg))
	assert.True(t, ShouldWriteToNew("RPUSH", cfg))
	assert.True(t, ShouldWriteToNew("ZINCRBY", cfg))
}

// ============================================================
// 辅助函数测试
// ============================================================

func TestFormatArgs(t *testing.T) {
	args := []interface{}{"SET", "key1", "value1"}
	result := formatArgs(args, 200)
	assert.Contains(t, result, "SET")
	assert.Contains(t, result, "key1")

	// 测试截断
	longArgs := []interface{}{"SET", "key1", string(make([]byte, 300))}
	result = formatArgs(longArgs, 50)
	assert.True(t, len(result) <= 70) // 50 + "...(truncated)"
	assert.Contains(t, result, "...(truncated)")
}

func TestLogPanicRecovered(t *testing.T) {
	// 不应 panic
	assert.NotPanics(t, func() {
		LogPanicRecovered("test", "some error")
	})
}

func TestSafeGo(t *testing.T) {
	done := make(chan bool, 1)

	// 正常执行
	SafeGo("test", func() {
		done <- true
	})
	assert.True(t, <-done)

	// panic 恢复
	SafeGo("test-panic", func() {
		panic("test panic")
	})
	// 不应导致主 goroutine panic，给一点时间让 goroutine 执行
	// 如果 SafeGo 没有 recover，测试本身会失败
}
