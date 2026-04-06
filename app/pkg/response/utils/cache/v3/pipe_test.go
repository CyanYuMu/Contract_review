package cache

import (
	"context"
	"fmt"
	"testing"
	"time"

	redis8 "github.com/go-redis/redis/v8"
	redis9 "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

// 测试辅助函数
func setupV8Pipeline(t *testing.T) *V8Pipeline {
	client := redis8.NewClient(&redis8.Options{
		Addr: "localhost:6379",
		DB:   0,
	})
	return NewV8Pipeline(client.Pipeline())
}

func setupV9Pipeline(t *testing.T) *V9Pipeline {
	client := redis9.NewClient(&redis9.Options{
		Addr: "localhost:6379",
		DB:   0,
	})
	return NewV9Pipeline(client.Pipeline())
}

// 通用的测试用例
type pipelineTestCase struct {
	name     string
	testFunc func(t *testing.T, ctx context.Context, pipe Pipeline)
}

// 测试用例集合
var testCases = []pipelineTestCase{
	{
		name: "String Operations",
		testFunc: func(t *testing.T, ctx context.Context, pipe Pipeline) {
			// 测试 Set 和 Get
			pipe.Set(ctx, "test_key", "test_value", time.Minute)
			pipe.Get(ctx, "test_key")
			pipe.TTL(ctx, "test_key")

			cmds, err := pipe.Exec(ctx)
			assert.NoError(t, err)
			assert.Len(t, cmds, 3)

			// 验证结果
			getCmd := cmds[1].(StringCmd)
			val, err := getCmd.Result()
			assert.NoError(t, err)
			assert.Equal(t, "test_value", val)
		},
	},
	{
		name: "Hash Operations",
		testFunc: func(t *testing.T, ctx context.Context, pipe Pipeline) {
			// 测试 Hash 操作
			pipe.HSet(ctx, "test_hash", "field1", "value1", "field2", "value2")
			pipe.HGetAll(ctx, "test_hash")
			pipe.HMGet(ctx, "test_hash", "field1", "field2")
			pipe.HDel(ctx, "test_hash", "field1")

			cmds, err := pipe.Exec(ctx)
			assert.NoError(t, err)
			assert.Len(t, cmds, 4)

			// 验证 HGetAll 结果
			hashCmd := cmds[1].(StringStringMapCmd)
			val, err := hashCmd.Result()
			assert.NoError(t, err)
			assert.Equal(t, "value1", val["field1"])
			assert.Equal(t, "value2", val["field2"])
		},
	},
	{
		name: "List Operations",
		testFunc: func(t *testing.T, ctx context.Context, pipe Pipeline) {
			// 测试 List 操作
			pipe.LPush(ctx, "test_list", "value1", "value2")
			pipe.RPush(ctx, "test_list", "value3")
			pipe.LPop(ctx, "test_list")
			pipe.RPop(ctx, "test_list")

			cmds, err := pipe.Exec(ctx)
			assert.NoError(t, err)
			assert.Len(t, cmds, 4)
		},
	},
	{
		name: "Set Operations",
		testFunc: func(t *testing.T, ctx context.Context, pipe Pipeline) {
			// 测试 Set 操作
			pipe.SAdd(ctx, "test_set", "member1", "member2")
			pipe.SCard(ctx, "test_set")
			pipe.SRem(ctx, "test_set", "member1")

			cmds, err := pipe.Exec(ctx)
			assert.NoError(t, err)
			assert.Len(t, cmds, 3)

			// 验证 SCard 结果
			cardCmd := cmds[1].(IntCmd)
			count, err := cardCmd.Result()
			assert.NoError(t, err)
			assert.Equal(t, int64(2), count)
		},
	},
	{
		name: "Sorted Set Operations",
		testFunc: func(t *testing.T, ctx context.Context, pipe Pipeline) {
			// 测试 Sorted Set 操作
			pipe.ZAdd(ctx, "test_zset", Z{Score: 1, Member: "member1"}, Z{Score: 2, Member: "member2"})
			pipe.ZCard(ctx, "test_zset")
			pipe.ZRange(ctx, "test_zset", 0, -1)
			pipe.ZRem(ctx, "test_zset", "member1")

			cmds, err := pipe.Exec(ctx)
			assert.NoError(t, err)
			assert.Len(t, cmds, 4)

			// 验证 ZRange 结果
			rangeCmd := cmds[2].(StringSliceCmd)
			members, err := rangeCmd.Result()
			assert.NoError(t, err)
			assert.Contains(t, members, "member1")
			assert.Contains(t, members, "member2")
		},
	},
	{
		name: "Key Operations",
		testFunc: func(t *testing.T, ctx context.Context, pipe Pipeline) {
			// 测试键操作
			pipe.Set(ctx, "test_key", "value", time.Minute)
			pipe.Exists(ctx, "test_key")
			pipe.Expire(ctx, "test_key", time.Hour)
			pipe.Del(ctx, "test_key")

			cmds, err := pipe.Exec(ctx)
			assert.NoError(t, err)
			assert.Len(t, cmds, 4)

			// 验证 Exists 结果
			existsCmd := cmds[1].(BoolCmd)
			exists, err := existsCmd.Result()
			assert.NoError(t, err)
			assert.True(t, exists)
		},
	},
}

// V8Pipeline 测试
func TestV8Pipeline(t *testing.T) {
	ctx := context.Background()
	pipe := setupV8Pipeline(t)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.testFunc(t, ctx, pipe)
		})
	}
}

// V9Pipeline 测试
func TestV9Pipeline(t *testing.T) {
	ctx := context.Background()
	pipe := setupV9Pipeline(t)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.testFunc(t, ctx, pipe)
		})
	}
}

// 测试错误处理
func TestPipelineErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("V8 Pipeline Errors", func(t *testing.T) {
		pipe := setupV8Pipeline(t)

		// 测试无效的类型转换
		pipe.Set(ctx, "test_key", make(chan int), 0) // 无效的值类型
		_, err := pipe.Exec(ctx)
		assert.Error(t, err)
	})

	t.Run("V9 Pipeline Errors", func(t *testing.T) {
		pipe := setupV9Pipeline(t)

		// 测试无效的命令
		pipe.Get(ctx, "") // 空键
		_, err := pipe.Exec(ctx)
		assert.Error(t, err)
	})
}

// 测试并发安全性
func TestPipelineConcurrency(t *testing.T) {
	ctx := context.Background()

	testPipeline := func(t *testing.T, pipe Pipeline) {
		const goroutines = 10
		done := make(chan bool, goroutines)

		for i := 0; i < goroutines; i++ {
			go func(id int) {
				// 每个 goroutine 执行一系列操作
				pipe.Set(ctx, "key", "value", time.Minute)
				pipe.Get(ctx, "key")
				pipe.Del(ctx, "key")

				cmds, err := pipe.Exec(ctx)
				assert.NoError(t, err)
				assert.Len(t, cmds, 3)

				done <- true
			}(i)
		}

		// 等待所有 goroutine 完成
		for i := 0; i < goroutines; i++ {
			<-done
		}
	}

	t.Run("V8 Pipeline Concurrency", func(t *testing.T) {
		testPipeline(t, setupV8Pipeline(t))
	})

	t.Run("V9 Pipeline Concurrency", func(t *testing.T) {
		testPipeline(t, setupV9Pipeline(t))
	})
}

// 测试大批量操作
func TestPipelineBulkOperations(t *testing.T) {
	ctx := context.Background()

	testBulkOperations := func(t *testing.T, pipe Pipeline) {
		// 准备大量数据
		const count = 1000
		for i := 0; i < count; i++ {
			pipe.Set(ctx, fmt.Sprintf("bulk_key_%d", i), i, time.Minute)
		}

		cmds, err := pipe.Exec(ctx)
		assert.NoError(t, err)
		assert.Len(t, cmds, count)

		// 清理数据
		for i := 0; i < count; i++ {
			pipe.Del(ctx, fmt.Sprintf("bulk_key_%d", i))
		}
		_, err = pipe.Exec(ctx)
		assert.NoError(t, err)
	}

	t.Run("V8 Pipeline Bulk Operations", func(t *testing.T) {
		testBulkOperations(t, setupV8Pipeline(t))
	})

	t.Run("V9 Pipeline Bulk Operations", func(t *testing.T) {
		testBulkOperations(t, setupV9Pipeline(t))
	})
}
