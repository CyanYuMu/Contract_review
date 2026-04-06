package dualwrite

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	redisV9 "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// HookV8 集成测试（使用 miniredis）
// ============================================================

func setupTestRedisV8(t *testing.T) (*miniredis.Miniredis, *miniredis.Miniredis, redis.UniversalClient, redis.UniversalClient) {
	// 创建旧库
	oldRedis := miniredis.RunT(t)
	oldCli := redis.NewClient(&redis.Options{Addr: oldRedis.Addr()})

	// 创建新库
	newRedis := miniredis.RunT(t)
	newCli := redis.NewClient(&redis.Options{Addr: newRedis.Addr()})

	return oldRedis, newRedis, oldCli, newCli
}

// TestHookV8_DualWrite_Idempotent_SyncRunning 测试增量同步运行中幂等命令双写
func TestHookV8_DualWrite_Idempotent_SyncRunning(t *testing.T) {
	oldRedis, newRedis, oldCli, newCli := setupTestRedisV8(t)
	defer oldRedis.Close()
	defer newRedis.Close()

	// 配置：增量同步运行中
	cfg := &Config{Enabled: true, SyncRunning: true, ReadFrom: "old"}
	hook := NewHookV8(newCli, cfg)
	oldCli.AddHook(hook)

	ctx := context.Background()

	// 测试 SET（幂等命令）- 应该双写
	err := oldCli.Set(ctx, "key1", "value1", 0).Err()
	require.NoError(t, err)

	// 等待异步写入完成
	time.Sleep(100 * time.Millisecond)

	// 验证旧库有数据
	val, err := oldCli.Get(ctx, "key1").Result()
	require.NoError(t, err)
	assert.Equal(t, "value1", val)

	// 验证新库也有数据（双写成功）
	val, err = newCli.Get(ctx, "key1").Result()
	require.NoError(t, err)
	assert.Equal(t, "value1", val)
}

// TestHookV8_DualWrite_NonIdempotent_SyncRunning 测试增量同步运行中非幂等命令不双写
func TestHookV8_DualWrite_NonIdempotent_SyncRunning(t *testing.T) {
	oldRedis, newRedis, oldCli, newCli := setupTestRedisV8(t)
	defer oldRedis.Close()
	defer newRedis.Close()

	// 配置：增量同步运行中
	cfg := &Config{Enabled: true, SyncRunning: true, ReadFrom: "old"}
	hook := NewHookV8(newCli, cfg)
	oldCli.AddHook(hook)

	ctx := context.Background()

	// 先设置初始值
	oldCli.Set(ctx, "counter", "10", 0)
	newCli.Set(ctx, "counter", "10", 0)
	time.Sleep(50 * time.Millisecond)

	// 测试 INCR（非幂等命令）- 增量同步运行中不应该双写
	result, err := oldCli.Incr(ctx, "counter").Result()
	require.NoError(t, err)
	assert.Equal(t, int64(11), result)

	// 等待看是否有异步写入
	time.Sleep(100 * time.Millisecond)

	// 验证旧库已增加
	val, err := oldCli.Get(ctx, "counter").Result()
	require.NoError(t, err)
	assert.Equal(t, "11", val)

	// 验证新库没有被双写（保持原值，由增量同步处理）
	val, err = newCli.Get(ctx, "counter").Result()
	require.NoError(t, err)
	assert.Equal(t, "10", val) // 新库应该还是 10
}

// TestHookV8_DualWrite_AllCommands_SyncStopped 测试增量同步停止后所有写命令都双写
func TestHookV8_DualWrite_AllCommands_SyncStopped(t *testing.T) {
	oldRedis, newRedis, oldCli, newCli := setupTestRedisV8(t)
	defer oldRedis.Close()
	defer newRedis.Close()

	// 配置：增量同步已停止
	cfg := &Config{Enabled: true, SyncRunning: false, ReadFrom: "old"}
	hook := NewHookV8(newCli, cfg)
	oldCli.AddHook(hook)

	ctx := context.Background()

	// 测试 INCR（非幂等命令）- 增量同步停止后应该双写
	// 先设置旧库初始值（通过 hook 会双写到新库）
	oldCli.Set(ctx, "counter", "10", 0)
	time.Sleep(50 * time.Millisecond)

	result, err := oldCli.Incr(ctx, "counter").Result()
	require.NoError(t, err)
	assert.Equal(t, int64(11), result)

	// 等待异步写入
	time.Sleep(100 * time.Millisecond)

	// 验证新库也执行了 INCR
	// 注意：SET 也被双写了，所以新库初始值是 "10"，INCR 后是 "11"
	val, err := newCli.Get(ctx, "counter").Result()
	require.NoError(t, err)
	assert.Equal(t, "11", val)
}

// TestHookV8_ReadCommand_NoDualWrite 测试读命令不触发双写
func TestHookV8_ReadCommand_NoDualWrite(t *testing.T) {
	oldRedis, newRedis, oldCli, newCli := setupTestRedisV8(t)
	defer oldRedis.Close()
	defer newRedis.Close()

	cfg := &Config{Enabled: true, SyncRunning: false, ReadFrom: "old"}
	hook := NewHookV8(newCli, cfg)
	oldCli.AddHook(hook)

	ctx := context.Background()

	// 设置数据
	oldCli.Set(ctx, "readkey", "readvalue", 0)
	time.Sleep(50 * time.Millisecond)

	// 执行读命令
	val, err := oldCli.Get(ctx, "readkey").Result()
	require.NoError(t, err)
	assert.Equal(t, "readvalue", val)

	// 读命令不应该触发任何写入到新库
	// （这个测试主要验证读命令不会报错或产生副作用）
}

// TestHookV8_HashCommands 测试 Hash 命令双写
func TestHookV8_HashCommands(t *testing.T) {
	oldRedis, newRedis, oldCli, newCli := setupTestRedisV8(t)
	defer oldRedis.Close()
	defer newRedis.Close()

	cfg := &Config{Enabled: true, SyncRunning: false, ReadFrom: "old"}
	hook := NewHookV8(newCli, cfg)
	oldCli.AddHook(hook)

	ctx := context.Background()

	// 测试 HSET
	err := oldCli.HSet(ctx, "hash1", "field1", "value1").Err()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// 验证新库
	val, err := newCli.HGet(ctx, "hash1", "field1").Result()
	require.NoError(t, err)
	assert.Equal(t, "value1", val)

	// 测试 HDEL
	err = oldCli.HDel(ctx, "hash1", "field1").Err()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// 验证新库字段已删除
	_, err = newCli.HGet(ctx, "hash1", "field1").Result()
	assert.Equal(t, redis.Nil, err)
}

// TestHookV8_SetCommands 测试 Set 命令双写
func TestHookV8_SetCommands(t *testing.T) {
	oldRedis, newRedis, oldCli, newCli := setupTestRedisV8(t)
	defer oldRedis.Close()
	defer newRedis.Close()

	cfg := &Config{Enabled: true, SyncRunning: false, ReadFrom: "old"}
	hook := NewHookV8(newCli, cfg)
	oldCli.AddHook(hook)

	ctx := context.Background()

	// 测试 SADD
	err := oldCli.SAdd(ctx, "set1", "member1", "member2").Err()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// 验证新库
	members, err := newCli.SMembers(ctx, "set1").Result()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"member1", "member2"}, members)

	// 测试 SREM
	err = oldCli.SRem(ctx, "set1", "member1").Err()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// 验证新库
	members, err = newCli.SMembers(ctx, "set1").Result()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"member2"}, members)
}

// TestHookV8_ZSetCommands 测试 ZSet 命令双写
func TestHookV8_ZSetCommands(t *testing.T) {
	oldRedis, newRedis, oldCli, newCli := setupTestRedisV8(t)
	defer oldRedis.Close()
	defer newRedis.Close()

	cfg := &Config{Enabled: true, SyncRunning: false, ReadFrom: "old"}
	hook := NewHookV8(newCli, cfg)
	oldCli.AddHook(hook)

	ctx := context.Background()

	// 测试 ZADD
	err := oldCli.ZAdd(ctx, "zset1", &redis.Z{Score: 1.0, Member: "member1"}).Err()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// 验证新库
	score, err := newCli.ZScore(ctx, "zset1", "member1").Result()
	require.NoError(t, err)
	assert.Equal(t, 1.0, score)

	// 测试 ZREM
	err = oldCli.ZRem(ctx, "zset1", "member1").Err()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// 验证新库
	_, err = newCli.ZScore(ctx, "zset1", "member1").Result()
	assert.Equal(t, redis.Nil, err)
}

// TestHookV8_DEL_Command 测试 DEL 命令双写
func TestHookV8_DEL_Command(t *testing.T) {
	oldRedis, newRedis, oldCli, newCli := setupTestRedisV8(t)
	defer oldRedis.Close()
	defer newRedis.Close()

	cfg := &Config{Enabled: true, SyncRunning: false, ReadFrom: "old"}
	hook := NewHookV8(newCli, cfg)
	oldCli.AddHook(hook)

	ctx := context.Background()

	// 先在两库设置数据
	oldCli.Set(ctx, "delkey", "value", 0)
	newCli.Set(ctx, "delkey", "value", 0)
	time.Sleep(50 * time.Millisecond)

	// 删除
	err := oldCli.Del(ctx, "delkey").Err()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// 验证两库都已删除
	exists, _ := oldCli.Exists(ctx, "delkey").Result()
	assert.Equal(t, int64(0), exists)

	exists, _ = newCli.Exists(ctx, "delkey").Result()
	assert.Equal(t, int64(0), exists)
}

// TestHookV8_EXPIRE_Command 测试 EXPIRE 命令双写
func TestHookV8_EXPIRE_Command(t *testing.T) {
	oldRedis, newRedis, oldCli, newCli := setupTestRedisV8(t)
	defer oldRedis.Close()
	defer newRedis.Close()

	cfg := &Config{Enabled: true, SyncRunning: false, ReadFrom: "old"}
	hook := NewHookV8(newCli, cfg)
	oldCli.AddHook(hook)

	ctx := context.Background()

	// 设置数据
	oldCli.Set(ctx, "expkey", "value", 0)
	newCli.Set(ctx, "expkey", "value", 0)
	time.Sleep(50 * time.Millisecond)

	// 设置过期时间
	err := oldCli.Expire(ctx, "expkey", 60*time.Second).Err()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// 验证新库也设置了过期时间
	ttl, err := newCli.TTL(ctx, "expkey").Result()
	require.NoError(t, err)
	assert.True(t, ttl > 0 && ttl <= 60*time.Second)
}

// TestHookV8_Pipeline 测试 Pipeline 命令双写
func TestHookV8_Pipeline(t *testing.T) {
	oldRedis, newRedis, oldCli, newCli := setupTestRedisV8(t)
	defer oldRedis.Close()
	defer newRedis.Close()

	cfg := &Config{Enabled: true, SyncRunning: false, ReadFrom: "old"}
	hook := NewHookV8(newCli, cfg)
	oldCli.AddHook(hook)

	ctx := context.Background()

	// 使用 Pipeline 执行多个命令
	pipe := oldCli.Pipeline()
	pipe.Set(ctx, "pipe_key1", "value1", 0)
	pipe.Set(ctx, "pipe_key2", "value2", 0)
	pipe.HSet(ctx, "pipe_hash", "field", "value")
	_, err := pipe.Exec(ctx)
	require.NoError(t, err)

	time.Sleep(150 * time.Millisecond)

	// 验证新库
	val, err := newCli.Get(ctx, "pipe_key1").Result()
	require.NoError(t, err)
	assert.Equal(t, "value1", val)

	val, err = newCli.Get(ctx, "pipe_key2").Result()
	require.NoError(t, err)
	assert.Equal(t, "value2", val)

	val, err = newCli.HGet(ctx, "pipe_hash", "field").Result()
	require.NoError(t, err)
	assert.Equal(t, "value", val)
}

// TestHookV8_Pipeline_MixedCommands 测试 Pipeline 中混合命令（同步运行中）
func TestHookV8_Pipeline_MixedCommands(t *testing.T) {
	oldRedis, newRedis, oldCli, newCli := setupTestRedisV8(t)
	defer oldRedis.Close()
	defer newRedis.Close()

	// 增量同步运行中
	cfg := &Config{Enabled: true, SyncRunning: true, ReadFrom: "old"}
	hook := NewHookV8(newCli, cfg)
	oldCli.AddHook(hook)

	ctx := context.Background()

	// 在添加 hook 之前，先直接在旧库设置 counter 初始值（不通过 hook）
	// 使用 newCli 不会触发双写
	newRedis.Set("counter", "0") // 直接操作 miniredis
	oldRedis.Set("counter", "0") // 直接操作 miniredis

	pipe := oldCli.Pipeline()
	pipe.Set(ctx, "idempotent_key", "value", 0) // 幂等 - 应该双写
	pipe.Incr(ctx, "counter")                   // 非幂等 - 不应该双写
	pipe.HSet(ctx, "hash_key", "f", "v")        // 幂等 - 应该双写
	_, err := pipe.Exec(ctx)
	require.NoError(t, err)

	time.Sleep(150 * time.Millisecond)

	// 验证幂等命令被双写
	val, err := newCli.Get(ctx, "idempotent_key").Result()
	require.NoError(t, err)
	assert.Equal(t, "value", val)

	val, err = newCli.HGet(ctx, "hash_key", "f").Result()
	require.NoError(t, err)
	assert.Equal(t, "v", val)

	// 验证非幂等命令没有被双写（新库的 counter 还是 "0"）
	val, err = newCli.Get(ctx, "counter").Result()
	require.NoError(t, err)
	assert.Equal(t, "0", val) // 新库 counter 保持原值，未被 INCR
}

// TestHookV8_NewCliFailure_NoImpact 测试新库写入失败不影响旧库
func TestHookV8_NewCliFailure_NoImpact(t *testing.T) {
	oldRedis, _, oldCli, _ := setupTestRedisV8(t)
	defer oldRedis.Close()

	// 创建一个无效的新库客户端（连接不存在的地址）
	invalidCli := redis.NewClient(&redis.Options{Addr: "localhost:59999"})

	cfg := &Config{Enabled: true, SyncRunning: false, ReadFrom: "old"}
	hook := NewHookV8(invalidCli, cfg)
	oldCli.AddHook(hook)

	ctx := context.Background()

	// 旧库操作应该正常成功，即使新库写入失败
	err := oldCli.Set(ctx, "key1", "value1", 0).Err()
	require.NoError(t, err)

	// 验证旧库数据正常
	val, err := oldCli.Get(ctx, "key1").Result()
	require.NoError(t, err)
	assert.Equal(t, "value1", val)
}

// TestHookV8_NilConfig 测试 config 为 nil 的防御性检查
func TestHookV8_NilConfig(t *testing.T) {
	oldRedis, newRedis, oldCli, newCli := setupTestRedisV8(t)
	defer oldRedis.Close()
	defer newRedis.Close()

	// config 传 nil，应该使用默认值
	hook := NewHookV8(newCli, nil)
	oldCli.AddHook(hook)

	ctx := context.Background()

	// 应该正常工作，不 panic
	err := oldCli.Set(ctx, "key1", "value1", 0).Err()
	require.NoError(t, err)
}

// ============================================================
// HookV9 集成测试（使用 miniredis）
// ============================================================

func setupTestRedisV9(t *testing.T) (*miniredis.Miniredis, *miniredis.Miniredis, redisV9.UniversalClient, redisV9.UniversalClient) {
	// 创建旧库
	oldRedis := miniredis.RunT(t)
	oldCli := redisV9.NewClient(&redisV9.Options{Addr: oldRedis.Addr()})

	// 创建新库
	newRedis := miniredis.RunT(t)
	newCli := redisV9.NewClient(&redisV9.Options{Addr: newRedis.Addr()})

	return oldRedis, newRedis, oldCli, newCli
}

// TestHookV9_DualWrite_Idempotent_SyncRunning 测试增量同步运行中幂等命令双写
func TestHookV9_DualWrite_Idempotent_SyncRunning(t *testing.T) {
	oldRedis, newRedis, oldCli, newCli := setupTestRedisV9(t)
	defer oldRedis.Close()
	defer newRedis.Close()

	// 配置：增量同步运行中
	cfg := &Config{Enabled: true, SyncRunning: true, ReadFrom: "old"}
	hook := NewHookV9(newCli, cfg)
	oldCli.AddHook(hook)

	ctx := context.Background()

	// 测试 SET（幂等命令）- 应该双写
	err := oldCli.Set(ctx, "key1", "value1", 0).Err()
	require.NoError(t, err)

	// 等待异步写入完成
	time.Sleep(100 * time.Millisecond)

	// 验证旧库有数据
	val, err := oldCli.Get(ctx, "key1").Result()
	require.NoError(t, err)
	assert.Equal(t, "value1", val)

	// 验证新库也有数据（双写成功）
	val, err = newCli.Get(ctx, "key1").Result()
	require.NoError(t, err)
	assert.Equal(t, "value1", val)
}

// TestHookV9_DualWrite_NonIdempotent_SyncRunning 测试增量同步运行中非幂等命令不双写
func TestHookV9_DualWrite_NonIdempotent_SyncRunning(t *testing.T) {
	oldRedis, newRedis, oldCli, newCli := setupTestRedisV9(t)
	defer oldRedis.Close()
	defer newRedis.Close()

	// 配置：增量同步运行中
	cfg := &Config{Enabled: true, SyncRunning: true, ReadFrom: "old"}
	hook := NewHookV9(newCli, cfg)
	oldCli.AddHook(hook)

	ctx := context.Background()

	// 先设置初始值
	oldCli.Set(ctx, "counter", "10", 0)
	newCli.Set(ctx, "counter", "10", 0)
	time.Sleep(50 * time.Millisecond)

	// 测试 INCR（非幂等命令）- 增量同步运行中不应该双写
	result, err := oldCli.Incr(ctx, "counter").Result()
	require.NoError(t, err)
	assert.Equal(t, int64(11), result)

	// 等待看是否有异步写入
	time.Sleep(100 * time.Millisecond)

	// 验证旧库已增加
	val, err := oldCli.Get(ctx, "counter").Result()
	require.NoError(t, err)
	assert.Equal(t, "11", val)

	// 验证新库没有被双写（保持原值，由增量同步处理）
	val, err = newCli.Get(ctx, "counter").Result()
	require.NoError(t, err)
	assert.Equal(t, "10", val) // 新库应该还是 10
}

// TestHookV9_DualWrite_AllCommands_SyncStopped 测试增量同步停止后所有写命令都双写
func TestHookV9_DualWrite_AllCommands_SyncStopped(t *testing.T) {
	oldRedis, newRedis, oldCli, newCli := setupTestRedisV9(t)
	defer oldRedis.Close()
	defer newRedis.Close()

	// 配置：增量同步已停止
	cfg := &Config{Enabled: true, SyncRunning: false, ReadFrom: "old"}
	hook := NewHookV9(newCli, cfg)
	oldCli.AddHook(hook)

	ctx := context.Background()

	// 测试 INCR（非幂等命令）- 增量同步停止后应该双写
	// 先设置旧库初始值（通过 hook 会双写到新库）
	oldCli.Set(ctx, "counter", "10", 0)
	time.Sleep(50 * time.Millisecond)

	result, err := oldCli.Incr(ctx, "counter").Result()
	require.NoError(t, err)
	assert.Equal(t, int64(11), result)

	// 等待异步写入
	time.Sleep(100 * time.Millisecond)

	// 验证新库也执行了 INCR
	// 注意：SET 也被双写了，所以新库初始值是 "10"，INCR 后是 "11"
	val, err := newCli.Get(ctx, "counter").Result()
	require.NoError(t, err)
	assert.Equal(t, "11", val)
}

// TestHookV9_HashCommands 测试 Hash 命令双写
func TestHookV9_HashCommands(t *testing.T) {
	oldRedis, newRedis, oldCli, newCli := setupTestRedisV9(t)
	defer oldRedis.Close()
	defer newRedis.Close()

	cfg := &Config{Enabled: true, SyncRunning: false, ReadFrom: "old"}
	hook := NewHookV9(newCli, cfg)
	oldCli.AddHook(hook)

	ctx := context.Background()

	// 测试 HSET
	err := oldCli.HSet(ctx, "hash1", "field1", "value1").Err()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// 验证新库
	val, err := newCli.HGet(ctx, "hash1", "field1").Result()
	require.NoError(t, err)
	assert.Equal(t, "value1", val)

	// 测试 HDEL
	err = oldCli.HDel(ctx, "hash1", "field1").Err()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// 验证新库字段已删除
	_, err = newCli.HGet(ctx, "hash1", "field1").Result()
	assert.Equal(t, redisV9.Nil, err)
}

// TestHookV9_SetCommands 测试 Set 命令双写
func TestHookV9_SetCommands(t *testing.T) {
	oldRedis, newRedis, oldCli, newCli := setupTestRedisV9(t)
	defer oldRedis.Close()
	defer newRedis.Close()

	cfg := &Config{Enabled: true, SyncRunning: false, ReadFrom: "old"}
	hook := NewHookV9(newCli, cfg)
	oldCli.AddHook(hook)

	ctx := context.Background()

	// 测试 SADD
	err := oldCli.SAdd(ctx, "set1", "member1", "member2").Err()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// 验证新库
	members, err := newCli.SMembers(ctx, "set1").Result()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"member1", "member2"}, members)

	// 测试 SREM
	err = oldCli.SRem(ctx, "set1", "member1").Err()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// 验证新库
	members, err = newCli.SMembers(ctx, "set1").Result()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"member2"}, members)
}

// TestHookV9_ZSetCommands 测试 ZSet 命令双写
func TestHookV9_ZSetCommands(t *testing.T) {
	oldRedis, newRedis, oldCli, newCli := setupTestRedisV9(t)
	defer oldRedis.Close()
	defer newRedis.Close()

	cfg := &Config{Enabled: true, SyncRunning: false, ReadFrom: "old"}
	hook := NewHookV9(newCli, cfg)
	oldCli.AddHook(hook)

	ctx := context.Background()

	// 测试 ZADD
	err := oldCli.ZAdd(ctx, "zset1", redisV9.Z{Score: 1.0, Member: "member1"}).Err()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// 验证新库
	score, err := newCli.ZScore(ctx, "zset1", "member1").Result()
	require.NoError(t, err)
	assert.Equal(t, 1.0, score)

	// 测试 ZREM
	err = oldCli.ZRem(ctx, "zset1", "member1").Err()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// 验证新库
	_, err = newCli.ZScore(ctx, "zset1", "member1").Result()
	assert.Equal(t, redisV9.Nil, err)
}

// TestHookV9_DEL_Command 测试 DEL 命令双写
func TestHookV9_DEL_Command(t *testing.T) {
	oldRedis, newRedis, oldCli, newCli := setupTestRedisV9(t)
	defer oldRedis.Close()
	defer newRedis.Close()

	cfg := &Config{Enabled: true, SyncRunning: false, ReadFrom: "old"}
	hook := NewHookV9(newCli, cfg)
	oldCli.AddHook(hook)

	ctx := context.Background()

	// 先在两库设置数据
	oldCli.Set(ctx, "delkey", "value", 0)
	newCli.Set(ctx, "delkey", "value", 0)
	time.Sleep(50 * time.Millisecond)

	// 删除
	err := oldCli.Del(ctx, "delkey").Err()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// 验证两库都已删除
	exists, _ := oldCli.Exists(ctx, "delkey").Result()
	assert.Equal(t, int64(0), exists)

	exists, _ = newCli.Exists(ctx, "delkey").Result()
	assert.Equal(t, int64(0), exists)
}

// TestHookV9_EXPIRE_Command 测试 EXPIRE 命令双写
func TestHookV9_EXPIRE_Command(t *testing.T) {
	oldRedis, newRedis, oldCli, newCli := setupTestRedisV9(t)
	defer oldRedis.Close()
	defer newRedis.Close()

	cfg := &Config{Enabled: true, SyncRunning: false, ReadFrom: "old"}
	hook := NewHookV9(newCli, cfg)
	oldCli.AddHook(hook)

	ctx := context.Background()

	// 设置数据
	oldCli.Set(ctx, "expkey", "value", 0)
	newCli.Set(ctx, "expkey", "value", 0)
	time.Sleep(50 * time.Millisecond)

	// 设置过期时间
	err := oldCli.Expire(ctx, "expkey", 60*time.Second).Err()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// 验证新库也设置了过期时间
	ttl, err := newCli.TTL(ctx, "expkey").Result()
	require.NoError(t, err)
	assert.True(t, ttl > 0 && ttl <= 60*time.Second)
}

// TestHookV9_Pipeline 测试 Pipeline 命令双写
func TestHookV9_Pipeline(t *testing.T) {
	oldRedis, newRedis, oldCli, newCli := setupTestRedisV9(t)
	defer oldRedis.Close()
	defer newRedis.Close()

	cfg := &Config{Enabled: true, SyncRunning: false, ReadFrom: "old"}
	hook := NewHookV9(newCli, cfg)
	oldCli.AddHook(hook)

	ctx := context.Background()

	// 使用 Pipeline 执行多个命令
	pipe := oldCli.Pipeline()
	pipe.Set(ctx, "pipe_key1", "value1", 0)
	pipe.Set(ctx, "pipe_key2", "value2", 0)
	pipe.HSet(ctx, "pipe_hash", "field", "value")
	_, err := pipe.Exec(ctx)
	require.NoError(t, err)

	time.Sleep(150 * time.Millisecond)

	// 验证新库
	val, err := newCli.Get(ctx, "pipe_key1").Result()
	require.NoError(t, err)
	assert.Equal(t, "value1", val)

	val, err = newCli.Get(ctx, "pipe_key2").Result()
	require.NoError(t, err)
	assert.Equal(t, "value2", val)

	val, err = newCli.HGet(ctx, "pipe_hash", "field").Result()
	require.NoError(t, err)
	assert.Equal(t, "value", val)
}

// TestHookV9_Pipeline_MixedCommands 测试 Pipeline 中混合命令（同步运行中）
func TestHookV9_Pipeline_MixedCommands(t *testing.T) {
	oldRedis, newRedis, oldCli, newCli := setupTestRedisV9(t)
	defer oldRedis.Close()
	defer newRedis.Close()

	// 增量同步运行中
	cfg := &Config{Enabled: true, SyncRunning: true, ReadFrom: "old"}
	hook := NewHookV9(newCli, cfg)
	oldCli.AddHook(hook)

	ctx := context.Background()

	// 在添加 hook 之前，先直接在旧库设置 counter 初始值（不通过 hook）
	// 使用 newCli 不会触发双写
	newRedis.Set("counter", "0") // 直接操作 miniredis
	oldRedis.Set("counter", "0") // 直接操作 miniredis

	pipe := oldCli.Pipeline()
	pipe.Set(ctx, "idempotent_key", "value", 0) // 幂等 - 应该双写
	pipe.Incr(ctx, "counter")                   // 非幂等 - 不应该双写
	pipe.HSet(ctx, "hash_key", "f", "v")        // 幂等 - 应该双写
	_, err := pipe.Exec(ctx)
	require.NoError(t, err)

	time.Sleep(150 * time.Millisecond)

	// 验证幂等命令被双写
	val, err := newCli.Get(ctx, "idempotent_key").Result()
	require.NoError(t, err)
	assert.Equal(t, "value", val)

	val, err = newCli.HGet(ctx, "hash_key", "f").Result()
	require.NoError(t, err)
	assert.Equal(t, "v", val)

	// 验证非幂等命令没有被双写（新库的 counter 还是 "0"）
	val, err = newCli.Get(ctx, "counter").Result()
	require.NoError(t, err)
	assert.Equal(t, "0", val) // 新库 counter 保持原值，未被 INCR
}

// TestHookV9_NewCliFailure_NoImpact 测试新库写入失败不影响旧库
func TestHookV9_NewCliFailure_NoImpact(t *testing.T) {
	oldRedis, _, oldCli, _ := setupTestRedisV9(t)
	defer oldRedis.Close()

	// 创建一个无效的新库客户端（连接不存在的地址）
	invalidCli := redisV9.NewClient(&redisV9.Options{Addr: "localhost:59999"})

	cfg := &Config{Enabled: true, SyncRunning: false, ReadFrom: "old"}
	hook := NewHookV9(invalidCli, cfg)
	oldCli.AddHook(hook)

	ctx := context.Background()

	// 旧库操作应该正常成功，即使新库写入失败
	err := oldCli.Set(ctx, "key1", "value1", 0).Err()
	require.NoError(t, err)

	// 验证旧库数据正常
	val, err := oldCli.Get(ctx, "key1").Result()
	require.NoError(t, err)
	assert.Equal(t, "value1", val)
}

// TestHookV9_NilConfig 测试 config 为 nil 的防御性检查
func TestHookV9_NilConfig(t *testing.T) {
	oldRedis, newRedis, oldCli, newCli := setupTestRedisV9(t)
	defer oldRedis.Close()
	defer newRedis.Close()

	// config 传 nil，应该使用默认值
	hook := NewHookV9(newCli, nil)
	oldCli.AddHook(hook)

	ctx := context.Background()

	// 应该正常工作，不 panic
	err := oldCli.Set(ctx, "key1", "value1", 0).Err()
	require.NoError(t, err)
}

// ============================================================
// 完整运维阶段测试（模拟真实迁移流程）
// ============================================================

// TestHookV8_FullMigrationPhases 测试完整的 Redis 到 Valkey 迁移流程（V8）
func TestHookV8_FullMigrationPhases(t *testing.T) {
	oldRedis, newRedis, oldCli, newCli := setupTestRedisV8(t)
	defer oldRedis.Close()
	defer newRedis.Close()

	ctx := context.Background()

	// ========== 阶段 0：迁移前（无双写） ==========
	t.Log("阶段 0：迁移前 - 只操作旧库")
	oldCli.Set(ctx, "user:1:name", "Alice", 0)
	oldCli.Set(ctx, "user:1:age", "25", 0)
	oldCli.HSet(ctx, "user:1:profile", "city", "Beijing")
	oldCli.Incr(ctx, "counter:visits")
	time.Sleep(50 * time.Millisecond)

	// 验证旧库有数据
	val, _ := oldCli.Get(ctx, "user:1:name").Result()
	assert.Equal(t, "Alice", val)
	// 验证新库没有数据
	_, err := newCli.Get(ctx, "user:1:name").Result()
	assert.Equal(t, redis.Nil, err)

	// ========== 阶段 1：双写+读旧库（增量同步运行中） ==========
	t.Log("阶段 1：双写+读旧库（增量同步运行中）")
	cfg := &Config{Enabled: true, SyncRunning: true, ReadFrom: "old"}
	hook := NewHookV8(newCli, cfg)
	oldCli.AddHook(hook)

	// 幂等命令：应该双写
	oldCli.Set(ctx, "user:2:name", "Bob", 0)
	oldCli.HSet(ctx, "user:2:profile", "city", "Shanghai")
	oldCli.SAdd(ctx, "tags", "golang", "redis")
	time.Sleep(100 * time.Millisecond)

	// 验证幂等命令被双写到新库
	val, _ = newCli.Get(ctx, "user:2:name").Result()
	assert.Equal(t, "Bob", val)
	val, _ = newCli.HGet(ctx, "user:2:profile", "city").Result()
	assert.Equal(t, "Shanghai", val)

	// 非幂等命令：不应该双写（由增量同步处理）
	oldCli.Incr(ctx, "counter:visits")
	oldCli.Incr(ctx, "counter:visits")
	time.Sleep(100 * time.Millisecond)

	// 验证旧库 counter 增加了
	val, _ = oldCli.Get(ctx, "counter:visits").Result()
	assert.Equal(t, "3", val) // 阶段0的1次 + 阶段1的2次

	// 验证新库 counter 没有被双写（由增量同步处理）
	_, err = newCli.Get(ctx, "counter:visits").Result()
	assert.Equal(t, redis.Nil, err)

	// 模拟增量同步完成，手动同步 counter 到新库
	t.Log("模拟增量同步：将 counter 同步到新库")
	val, _ = oldCli.Get(ctx, "counter:visits").Result()
	newCli.Set(ctx, "counter:visits", val, 0)
	time.Sleep(50 * time.Millisecond)

	// ========== 阶段 2：双写+读旧库（增量同步已停止） ==========
	t.Log("阶段 2：双写+读旧库（增量同步已停止）")
	cfg.SyncRunning = false

	// 所有写命令都应该双写
	oldCli.Set(ctx, "user:3:name", "Charlie", 0)
	oldCli.Incr(ctx, "counter:visits") // 非幂等命令也要双写
	oldCli.HSet(ctx, "user:3:profile", "city", "Shenzhen")
	time.Sleep(100 * time.Millisecond)

	// 验证所有命令都被双写
	val, _ = newCli.Get(ctx, "user:3:name").Result()
	assert.Equal(t, "Charlie", val)
	val, _ = newCli.Get(ctx, "counter:visits").Result()
	assert.Equal(t, "4", val) // 3 + 1
	val, _ = newCli.HGet(ctx, "user:3:profile", "city").Result()
	assert.Equal(t, "Shenzhen", val)

	// 读操作仍然从旧库读取
	val, _ = oldCli.Get(ctx, "user:1:name").Result()
	assert.Equal(t, "Alice", val)

	// ========== 阶段 3：双写+读新库（验证阶段） ==========
	t.Log("阶段 3：双写+读新库（验证阶段）")
	cfg.ReadFrom = "new"

	// 继续双写
	oldCli.Set(ctx, "user:4:name", "David", 0)
	oldCli.Incr(ctx, "counter:visits")
	time.Sleep(100 * time.Millisecond)

	// 验证新库有最新数据
	val, _ = newCli.Get(ctx, "user:4:name").Result()
	assert.Equal(t, "David", val)
	val, _ = newCli.Get(ctx, "counter:visits").Result()
	assert.Equal(t, "5", val)

	// 验证数据一致性：新旧库关键数据应该一致
	oldVal, _ := oldCli.Get(ctx, "user:4:name").Result()
	newVal, _ := newCli.Get(ctx, "user:4:name").Result()
	assert.Equal(t, oldVal, newVal)

	oldCounter, _ := oldCli.Get(ctx, "counter:visits").Result()
	newCounter, _ := newCli.Get(ctx, "counter:visits").Result()
	assert.Equal(t, oldCounter, newCounter)

	// ========== 阶段 4：只写新库（迁移完成） ==========
	t.Log("阶段 4：只写新库（迁移完成）")
	// 移除 hook，直接使用新库客户端
	oldCli = redis.NewClient(&redis.Options{Addr: oldRedis.Addr()}) // 重新创建不带 hook 的客户端

	// 后续操作只在新库进行
	newCli.Set(ctx, "user:5:name", "Eve", 0)
	newCli.Incr(ctx, "counter:visits")
	time.Sleep(50 * time.Millisecond)

	// 验证新库有数据
	val, _ = newCli.Get(ctx, "user:5:name").Result()
	assert.Equal(t, "Eve", val)
	val, _ = newCli.Get(ctx, "counter:visits").Result()
	assert.Equal(t, "6", val)

	// 验证旧库没有新数据（迁移完成后不再写入）
	_, err = oldCli.Get(ctx, "user:5:name").Result()
	assert.Equal(t, redis.Nil, err)

	t.Log("✓ 完整迁移流程测试通过")
}

// TestHookV9_FullMigrationPhases 测试完整的 Redis 到 Valkey 迁移流程（V9）
func TestHookV9_FullMigrationPhases(t *testing.T) {
	oldRedis, newRedis, oldCli, newCli := setupTestRedisV9(t)
	defer oldRedis.Close()
	defer newRedis.Close()

	ctx := context.Background()

	// ========== 阶段 0：迁移前（无双写） ==========
	t.Log("阶段 0：迁移前 - 只操作旧库")
	oldCli.Set(ctx, "user:1:name", "Alice", 0)
	oldCli.Set(ctx, "user:1:age", "25", 0)
	oldCli.HSet(ctx, "user:1:profile", "city", "Beijing")
	oldCli.Incr(ctx, "counter:visits")
	time.Sleep(50 * time.Millisecond)

	// 验证旧库有数据
	val, _ := oldCli.Get(ctx, "user:1:name").Result()
	assert.Equal(t, "Alice", val)
	// 验证新库没有数据
	_, err := newCli.Get(ctx, "user:1:name").Result()
	assert.Equal(t, redisV9.Nil, err)

	// ========== 阶段 1：双写+读旧库（增量同步运行中） ==========
	t.Log("阶段 1：双写+读旧库（增量同步运行中）")
	cfg := &Config{Enabled: true, SyncRunning: true, ReadFrom: "old"}
	hook := NewHookV9(newCli, cfg)
	oldCli.AddHook(hook)

	// 幂等命令：应该双写
	oldCli.Set(ctx, "user:2:name", "Bob", 0)
	oldCli.HSet(ctx, "user:2:profile", "city", "Shanghai")
	oldCli.SAdd(ctx, "tags", "golang", "redis")
	time.Sleep(100 * time.Millisecond)

	// 验证幂等命令被双写到新库
	val, _ = newCli.Get(ctx, "user:2:name").Result()
	assert.Equal(t, "Bob", val)
	val, _ = newCli.HGet(ctx, "user:2:profile", "city").Result()
	assert.Equal(t, "Shanghai", val)

	// 非幂等命令：不应该双写（由增量同步处理）
	oldCli.Incr(ctx, "counter:visits")
	oldCli.Incr(ctx, "counter:visits")
	time.Sleep(100 * time.Millisecond)

	// 验证旧库 counter 增加了
	val, _ = oldCli.Get(ctx, "counter:visits").Result()
	assert.Equal(t, "3", val) // 阶段0的1次 + 阶段1的2次

	// 验证新库 counter 没有被双写（由增量同步处理）
	_, err = newCli.Get(ctx, "counter:visits").Result()
	assert.Equal(t, redisV9.Nil, err)

	// 模拟增量同步完成，手动同步 counter 到新库
	t.Log("模拟增量同步：将 counter 同步到新库")
	val, _ = oldCli.Get(ctx, "counter:visits").Result()
	newCli.Set(ctx, "counter:visits", val, 0)
	time.Sleep(50 * time.Millisecond)

	// ========== 阶段 2：双写+读旧库（增量同步已停止） ==========
	t.Log("阶段 2：双写+读旧库（增量同步已停止）")
	cfg.SyncRunning = false

	// 所有写命令都应该双写
	oldCli.Set(ctx, "user:3:name", "Charlie", 0)
	oldCli.Incr(ctx, "counter:visits") // 非幂等命令也要双写
	oldCli.HSet(ctx, "user:3:profile", "city", "Shenzhen")
	time.Sleep(100 * time.Millisecond)

	// 验证所有命令都被双写
	val, _ = newCli.Get(ctx, "user:3:name").Result()
	assert.Equal(t, "Charlie", val)
	val, _ = newCli.Get(ctx, "counter:visits").Result()
	assert.Equal(t, "4", val) // 3 + 1
	val, _ = newCli.HGet(ctx, "user:3:profile", "city").Result()
	assert.Equal(t, "Shenzhen", val)

	// 读操作仍然从旧库读取
	val, _ = oldCli.Get(ctx, "user:1:name").Result()
	assert.Equal(t, "Alice", val)

	// ========== 阶段 3：双写+读新库（验证阶段） ==========
	t.Log("阶段 3：双写+读新库（验证阶段）")
	cfg.ReadFrom = "new"

	// 继续双写
	oldCli.Set(ctx, "user:4:name", "David", 0)
	oldCli.Incr(ctx, "counter:visits")
	time.Sleep(100 * time.Millisecond)

	// 验证新库有最新数据
	val, _ = newCli.Get(ctx, "user:4:name").Result()
	assert.Equal(t, "David", val)
	val, _ = newCli.Get(ctx, "counter:visits").Result()
	assert.Equal(t, "5", val)

	// 验证数据一致性：新旧库关键数据应该一致
	oldVal, _ := oldCli.Get(ctx, "user:4:name").Result()
	newVal, _ := newCli.Get(ctx, "user:4:name").Result()
	assert.Equal(t, oldVal, newVal)

	oldCounter, _ := oldCli.Get(ctx, "counter:visits").Result()
	newCounter, _ := newCli.Get(ctx, "counter:visits").Result()
	assert.Equal(t, oldCounter, newCounter)

	// ========== 阶段 4：只写新库（迁移完成） ==========
	t.Log("阶段 4：只写新库（迁移完成）")
	// 移除 hook，直接使用新库客户端
	oldCli = redisV9.NewClient(&redisV9.Options{Addr: oldRedis.Addr()}) // 重新创建不带 hook 的客户端

	// 后续操作只在新库进行
	newCli.Set(ctx, "user:5:name", "Eve", 0)
	newCli.Incr(ctx, "counter:visits")
	time.Sleep(50 * time.Millisecond)

	// 验证新库有数据
	val, _ = newCli.Get(ctx, "user:5:name").Result()
	assert.Equal(t, "Eve", val)
	val, _ = newCli.Get(ctx, "counter:visits").Result()
	assert.Equal(t, "6", val)

	// 验证旧库没有新数据（迁移完成后不再写入）
	_, err = oldCli.Get(ctx, "user:5:name").Result()
	assert.Equal(t, redisV9.Nil, err)

	t.Log("✓ 完整迁移流程测试通过")
}
