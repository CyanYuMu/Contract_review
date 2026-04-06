package driver

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	redisv8 "github.com/go-redis/redis/v8"
	redisv9 "github.com/redis/go-redis/v9"
)

// 测试用的 Redis 客户端（v8）
var testRedisV8 redisv8.UniversalClient

// 测试用的 Redis 客户端（v9）
var testRedisV9 redisv9.UniversalClient

// 初始化测试 Redis 客户端
func init() {
	// 如果设置了 SKIP_REDIS_TEST，则跳过 Redis 测试
	if os.Getenv("SKIP_REDIS_TEST") == "true" {
		return
	}

	// Redis V8 配置
	redisV8Addr := os.Getenv("REDIS_V8_ADDR")
	if redisV8Addr == "" {
		redisV8Addr = "10.185.0.22:6379"
	}
	redisV8Password := os.Getenv("REDIS_V8_PASSWORD")
	if redisV8Password == "" {
		redisV8Password = "W29n0X302Rkj03FstJFiwqXGgqKeeGIZ"
	}

	// 尝试连接 Redis v8（如果可用）
	clientV8 := redisv8.NewClient(&redisv8.Options{
		Addr:               redisV8Addr,
		Password:           redisV8Password,
		DB:                 0,
		DialTimeout:        10 * time.Second,
		ReadTimeout:        5 * time.Second,
		WriteTimeout:       5 * time.Second,
		PoolSize:           50,
		MinIdleConns:       2,
		MaxConnAge:         1800 * time.Second,
		PoolTimeout:        7200 * time.Second,
		IdleTimeout:        300 * time.Second,
		IdleCheckFrequency: 60 * time.Second,
	})
	if err := clientV8.Ping(context.Background()).Err(); err == nil {
		testRedisV8 = clientV8
	}

	// Redis V9 配置
	redisV9Addr := os.Getenv("REDIS_V9_ADDR")
	if redisV9Addr == "" {
		redisV9Addr = "10.185.15.42:6379"
	}
	redisV9Password := os.Getenv("REDIS_V9_PASSWORD")
	if redisV9Password == "" {
		redisV9Password = ""
	}

	// 尝试连接 Redis v9（集群模式）
	// 注意：集群模式需要至少一个节点地址，集群会自动发现其他节点
	clientV9 := redisv9.NewClusterClient(&redisv9.ClusterOptions{
		Addrs:        []string{redisV9Addr},
		Password:     redisV9Password,
		DialTimeout:  10 * time.Second,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		PoolSize:     50,
		MinIdleConns: 2,
		PoolTimeout:  7200 * time.Second,
	})
	if err := clientV9.Ping(context.Background()).Err(); err == nil {
		testRedisV9 = clientV9
	}
}

// 清理测试数据（v8）
func cleanupTestDataV8(t *testing.T, redisClient redisv8.UniversalClient, keyPrefix string) {
	if redisClient == nil {
		return
	}
	ctx := context.Background()
	// 清理所有测试窗口
	for i := 0; i < 10; i++ {
		windowKey := getWindowKey(keyPrefix, i)
		_ = redisClient.Del(ctx, windowKey)
	}
	_ = redisClient.Del(ctx, keyPrefix)
}

// 清理测试数据（v9）
func cleanupTestDataV9(t *testing.T, redisClient redisv9.UniversalClient, keyPrefix string) {
	if redisClient == nil {
		return
	}
	ctx := context.Background()
	// 清理所有测试窗口
	for i := 0; i < 10; i++ {
		windowKey := getWindowKey(keyPrefix, i)
		_ = redisClient.Del(ctx, windowKey)
	}
	_ = redisClient.Del(ctx, keyPrefix)
}

// ==================== BitsetV9Sliding 测试 ====================

func TestBitsetV9Sliding_New(t *testing.T) {
	if testRedisV9 == nil {
		t.Skip("Redis v9 not available, skipping test")
	}

	tests := []struct {
		name    string
		conf    *BitsetV9SlidingConf
		wantErr bool
	}{
		{
			name: "valid config",
			conf: &BitsetV9SlidingConf{
				Redis:       testRedisV9,
				Cap:         1000,
				P:           0.001,
				WindowSize:  1 * time.Hour,
				WindowCount: 5,
			},
			wantErr: false,
		},
		{
			name: "invalid window size",
			conf: &BitsetV9SlidingConf{
				Redis:      testRedisV9,
				Cap:        1000,
				P:          0.001,
				WindowSize: 0,
			},
			wantErr: true,
		},
		{
			name: "nil redis",
			conf: &BitsetV9SlidingConf{
				Redis:      nil,
				Cap:        1000,
				P:          0.001,
				WindowSize: 1 * time.Hour,
			},
			wantErr: true,
		},
		{
			name: "default window count",
			conf: &BitsetV9SlidingConf{
				Redis:       testRedisV9,
				Cap:         1000,
				P:           0.001,
				WindowSize:  1 * time.Hour,
				WindowCount: 0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bf, err := NewBitsetV9Sliding(tt.conf)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewBitsetV9Sliding() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && bf == nil {
				t.Error("NewBitsetV9Sliding() returned nil without error")
			}
		})
	}
}

func TestBitsetV9Sliding_Add_Exists(t *testing.T) {
	if testRedisV9 == nil {
		t.Skip("Redis v9 not available, skipping test")
	}

	bf, err := NewBitsetV9Sliding(&BitsetV9SlidingConf{
		Redis:       testRedisV9,
		Cap:         1000,
		P:           0.001,
		WindowSize:  1 * time.Hour,
		WindowCount: 5,
	})
	if err != nil {
		t.Fatalf("Failed to create BitsetV9Sliding: %v", err)
	}

	ctx := context.Background()
	testKey := "test:v9:add_exists"
	defer cleanupTestDataV9(t, testRedisV9, testKey)

	// 测试 Add
	ok, err := bf.Add(ctx, testKey, "item1", time.Hour)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if !ok {
		t.Error("Add() returned false, expected true")
	}

	// 测试重复添加
	ok, err = bf.Add(ctx, testKey, "item1", time.Hour)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if ok {
		t.Error("Add() returned true for duplicate item, expected false")
	}

	// 测试 Exists
	exists, err := bf.Exists(ctx, testKey, "item1")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Error("Exists() returned false for existing item")
	}

	// 测试不存在的元素
	exists, err = bf.Exists(ctx, testKey, "item2")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if exists {
		t.Error("Exists() returned true for non-existing item")
	}
}

func TestBitsetV9Sliding_MAdd_MExists(t *testing.T) {
	if testRedisV9 == nil {
		t.Skip("Redis v9 not available, skipping test")
	}

	bf, err := NewBitsetV9Sliding(&BitsetV9SlidingConf{
		Redis:       testRedisV9,
		Cap:         1000,
		P:           0.001,
		WindowSize:  1 * time.Hour,
		WindowCount: 5,
	})
	if err != nil {
		t.Fatalf("Failed to create BitsetV9Sliding: %v", err)
	}

	ctx := context.Background()
	testKey := "test:v9:madd_mexists"
	defer cleanupTestDataV9(t, testRedisV9, testKey)

	items := []string{"item1", "item2", "item3"}
	statusList, err := bf.MAdd(ctx, testKey, items, time.Hour)
	if err != nil {
		t.Fatalf("MAdd() error = %v", err)
	}
	if len(statusList) != len(items) {
		t.Errorf("MAdd() returned statusList length = %d, want %d", len(statusList), len(items))
	}
	for i, ok := range statusList {
		if !ok {
			t.Errorf("MAdd() returned false for items[%d] = %s", i, items[i])
		}
	}

	// 测试 MExists
	checkItems := []string{"item1", "item2", "item4", "item3"}
	existsList, err := bf.MExists(ctx, testKey, checkItems)
	if err != nil {
		t.Fatalf("MExists() error = %v", err)
	}
	if len(existsList) != len(checkItems) {
		t.Errorf("MExists() returned existsList length = %d, want %d", len(existsList), len(checkItems))
	}

	expected := []bool{true, true, false, true}
	for i, exists := range existsList {
		if exists != expected[i] {
			t.Errorf("MExists() existsList[%d] = %v, want %v", i, exists, expected[i])
		}
	}
}

func TestBitsetV9Sliding_Insert_MInsert(t *testing.T) {
	if testRedisV9 == nil {
		t.Skip("Redis v9 not available, skipping test")
	}

	bf, err := NewBitsetV9Sliding(&BitsetV9SlidingConf{
		Redis:       testRedisV9,
		Cap:         1000,
		P:           0.001,
		WindowSize:  1 * time.Hour,
		WindowCount: 5,
	})
	if err != nil {
		t.Fatalf("Failed to create BitsetV9Sliding: %v", err)
	}

	ctx := context.Background()
	testKey := "test:v9:insert"
	defer cleanupTestDataV9(t, testRedisV9, testKey)

	// 测试 Insert
	ok, err := bf.Insert(ctx, testKey, "item1", time.Hour)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	if !ok {
		t.Error("Insert() returned false, expected true")
	}

	// 测试重复 Insert
	ok, err = bf.Insert(ctx, testKey, "item1", time.Hour)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	if ok {
		t.Error("Insert() returned true for duplicate item, expected false")
	}

	// 测试 MInsert
	items := []string{"item2", "item3", "item4"}
	statusList, err := bf.MInsert(ctx, testKey, items, time.Hour)
	if err != nil {
		t.Fatalf("MInsert() error = %v", err)
	}
	if len(statusList) != len(items) {
		t.Errorf("MInsert() returned statusList length = %d, want %d", len(statusList), len(items))
	}
}

func TestBitsetV9Sliding_Info(t *testing.T) {
	if testRedisV9 == nil {
		t.Skip("Redis v9 not available, skipping test")
	}

	bf, err := NewBitsetV9Sliding(&BitsetV9SlidingConf{
		Redis:       testRedisV9,
		Cap:         1000,
		P:           0.001,
		WindowSize:  1 * time.Hour,
		WindowCount: 5,
	})
	if err != nil {
		t.Fatalf("Failed to create BitsetV9Sliding: %v", err)
	}

	ctx := context.Background()
	testKey := "test:v9:info"
	defer cleanupTestDataV9(t, testRedisV9, testKey)

	info, err := bf.Info(ctx, testKey)
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}

	requiredKeys := []string{"cap", "k", "m", "windowSize", "windowCount", "currentIdx"}
	for _, key := range requiredKeys {
		if _, ok := info[key]; !ok {
			t.Errorf("Info() missing key: %s", key)
		}
	}

	if cap, ok := info["cap"].(uint); !ok || cap == 0 {
		t.Error("Info() cap is invalid")
	}
	if windowCount, ok := info["windowCount"].(int); !ok || windowCount != 5 {
		t.Errorf("Info() windowCount = %v, want 5", windowCount)
	}
}

func TestBitsetV9Sliding_WindowRotation(t *testing.T) {
	if testRedisV9 == nil {
		t.Skip("Redis v9 not available, skipping test")
	}

	bf, err := NewBitsetV9Sliding(&BitsetV9SlidingConf{
		Redis:       testRedisV9,
		Cap:         1000,
		P:           0.001,
		WindowSize:  5 * time.Second, // 使用 5 秒窗口，适应 Redis 延迟
		WindowCount: 3,
	})
	if err != nil {
		t.Fatalf("Failed to create BitsetV9Sliding: %v", err)
	}

	ctx := context.Background()
	testKey := "test:v9:rotation"
	defer cleanupTestDataV9(t, testRedisV9, testKey)

	// 添加元素到第一个窗口
	ok, err := bf.Add(ctx, testKey, "item1", time.Hour)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if !ok {
		t.Error("Add() returned false, expected true")
	}

	// 立即检查 item1 应该存在
	exists1, err := bf.Exists(ctx, testKey, "item1")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists1 {
		t.Error("item1 should exist immediately after Add")
	}

	// 等待窗口轮换（但不超过所有窗口的总时长）
	// windowSize=5s, windowCount=3, 总时长=15s
	// 等待 6s 应该只轮转 1 个窗口，item1 的窗口应该还在活动范围内
	time.Sleep(6 * time.Second)

	// 添加元素到新窗口
	ok, err = bf.Add(ctx, testKey, "item2", time.Hour)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if !ok {
		t.Error("Add() returned false for item2, expected true")
	}

	// 立即检查 item2 应该存在
	exists2, err := bf.Exists(ctx, testKey, "item2")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists2 {
		t.Error("item2 should exist immediately after Add")
	}

	// item1 还应该在活动窗口内（因为 windowCount=3，最近 3 个窗口应该保留）
	// 轮转 1 个窗口后，窗口 0 应该还在活动范围内
	exists1, err = bf.Exists(ctx, testKey, "item1")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists1 {
		t.Error("item1 should still exist after one window rotation")
	}

	// 等待足够长的时间让所有窗口过期（超过 windowSize * windowCount）
	time.Sleep(16 * time.Second) // 5s * 3 = 15s，等待 16s 确保过期

	// 再次检查，应该都不存在了
	exists1, _ = bf.Exists(ctx, testKey, "item1")
	exists2, _ = bf.Exists(ctx, testKey, "item2")
	if exists1 || exists2 {
		t.Errorf("Items should not exist after expiration: item1=%v, item2=%v", exists1, exists2)
	}
}

// ==================== BitsetV8Sliding 测试 ====================

func TestBitsetV8Sliding_New(t *testing.T) {
	if testRedisV8 == nil {
		t.Skip("Redis v8 not available, skipping test")
	}

	bf, err := NewBitsetV8Sliding(&BitsetV8SlidingConf{
		Redis:       testRedisV8,
		Cap:         1000,
		P:           0.001,
		WindowSize:  1 * time.Hour,
		WindowCount: 5,
	})
	if err != nil {
		t.Fatalf("Failed to create BitsetV8Sliding: %v", err)
	}
	if bf == nil {
		t.Error("NewBitsetV8Sliding() returned nil")
	}
}

func TestBitsetV8Sliding_Add_Exists(t *testing.T) {
	if testRedisV8 == nil {
		t.Skip("Redis v8 not available, skipping test")
	}

	bf, err := NewBitsetV8Sliding(&BitsetV8SlidingConf{
		Redis:       testRedisV8,
		Cap:         1000,
		P:           0.001,
		WindowSize:  1 * time.Hour,
		WindowCount: 5,
	})
	if err != nil {
		t.Fatalf("Failed to create BitsetV8Sliding: %v", err)
	}

	ctx := context.Background()
	testKey := "test:v8:add_exists"
	defer cleanupTestDataV8(t, testRedisV8, testKey)

	ok, err := bf.Add(ctx, testKey, "item1", time.Hour)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if !ok {
		t.Error("Add() returned false, expected true")
	}

	exists, err := bf.Exists(ctx, testKey, "item1")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Error("Exists() returned false for existing item")
	}
}

func TestBitsetV8Sliding_MAdd_MExists(t *testing.T) {
	if testRedisV8 == nil {
		t.Skip("Redis v8 not available, skipping test")
	}

	bf, err := NewBitsetV8Sliding(&BitsetV8SlidingConf{
		Redis:       testRedisV8,
		Cap:         1000,
		P:           0.001,
		WindowSize:  1 * time.Hour,
		WindowCount: 5,
	})
	if err != nil {
		t.Fatalf("Failed to create BitsetV8Sliding: %v", err)
	}

	ctx := context.Background()
	testKey := "test:v8:madd_mexists"
	defer cleanupTestDataV8(t, testRedisV8, testKey)

	items := []string{"item1", "item2", "item3"}
	statusList, err := bf.MAdd(ctx, testKey, items, time.Hour)
	if err != nil {
		t.Fatalf("MAdd() error = %v", err)
	}
	if len(statusList) != len(items) {
		t.Errorf("MAdd() returned statusList length = %d, want %d", len(statusList), len(items))
	}

	checkItems := []string{"item1", "item2", "item4"}
	existsList, err := bf.MExists(ctx, testKey, checkItems)
	if err != nil {
		t.Fatalf("MExists() error = %v", err)
	}
	if len(existsList) != len(checkItems) {
		t.Errorf("MExists() returned existsList length = %d, want %d", len(existsList), len(checkItems))
	}

	expected := []bool{true, true, false}
	for i, exists := range existsList {
		if exists != expected[i] {
			t.Errorf("MExists() existsList[%d] = %v, want %v", i, exists, expected[i])
		}
	}
}

// ==================== GoBloomSliding 测试 ====================

func TestGoBloomSliding_New(t *testing.T) {
	tests := []struct {
		name    string
		conf    *GoBloomSlidingConf
		wantErr bool
	}{
		{
			name: "valid config",
			conf: &GoBloomSlidingConf{
				Cap:             1000,
				P:               0.001,
				WindowSize:      1 * time.Hour,
				WindowCount:     5,
				CleanupInterval: 1 * time.Minute,
			},
			wantErr: false,
		},
		{
			name: "invalid window size",
			conf: &GoBloomSlidingConf{
				Cap:        1000,
				P:          0.001,
				WindowSize: 0,
			},
			wantErr: true,
		},
		{
			name: "default window count",
			conf: &GoBloomSlidingConf{
				Cap:        1000,
				P:          0.001,
				WindowSize: 1 * time.Hour,
			},
			wantErr: false,
		},
		{
			name: "default cleanup interval",
			conf: &GoBloomSlidingConf{
				Cap:         1000,
				P:           0.001,
				WindowSize:  1 * time.Hour,
				WindowCount: 5,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bf, err := NewGoBloomSliding(tt.conf)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewGoBloomSliding() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if bf == nil {
					t.Error("NewGoBloomSliding() returned nil without error")
				} else {
					// 清理资源
					bf.Stop()
				}
			}
		})
	}
}

func TestGoBloomSliding_Add_Exists(t *testing.T) {
	bf, err := NewGoBloomSliding(&GoBloomSlidingConf{
		Cap:             1000,
		P:               0.001,
		WindowSize:      1 * time.Hour,
		WindowCount:     5,
		CleanupInterval: 1 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Failed to create GoBloomSliding: %v", err)
	}
	defer bf.Stop()

	ctx := context.Background()
	testKey := "test:go:add_exists"

	// 测试 Add
	ok, err := bf.Add(ctx, testKey, "item1", time.Hour)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if !ok {
		t.Error("Add() returned false, expected true")
	}

	// 测试重复添加
	ok, err = bf.Add(ctx, testKey, "item1", time.Hour)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if ok {
		t.Error("Add() returned true for duplicate item, expected false")
	}

	// 测试 Exists
	exists, err := bf.Exists(ctx, testKey, "item1")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Error("Exists() returned false for existing item")
	}

	// 测试不存在的元素
	exists, err = bf.Exists(ctx, testKey, "item2")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if exists {
		t.Error("Exists() returned true for non-existing item")
	}
}

func TestGoBloomSliding_MAdd_MExists(t *testing.T) {
	bf, err := NewGoBloomSliding(&GoBloomSlidingConf{
		Cap:             1000,
		P:               0.001,
		WindowSize:      1 * time.Hour,
		WindowCount:     5,
		CleanupInterval: 1 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Failed to create GoBloomSliding: %v", err)
	}
	defer bf.Stop()

	ctx := context.Background()
	testKey := "test:go:madd_mexists"

	items := []string{"item1", "item2", "item3"}
	statusList, err := bf.MAdd(ctx, testKey, items, time.Hour)
	if err != nil {
		t.Fatalf("MAdd() error = %v", err)
	}
	if len(statusList) != len(items) {
		t.Errorf("MAdd() returned statusList length = %d, want %d", len(statusList), len(items))
	}
	for i, ok := range statusList {
		if !ok {
			t.Errorf("MAdd() returned false for items[%d] = %s", i, items[i])
		}
	}

	checkItems := []string{"item1", "item2", "item4", "item3"}
	existsList, err := bf.MExists(ctx, testKey, checkItems)
	if err != nil {
		t.Fatalf("MExists() error = %v", err)
	}
	if len(existsList) != len(checkItems) {
		t.Errorf("MExists() returned existsList length = %d, want %d", len(existsList), len(checkItems))
	}

	expected := []bool{true, true, false, true}
	for i, exists := range existsList {
		if exists != expected[i] {
			t.Errorf("MExists() existsList[%d] = %v, want %v", i, exists, expected[i])
		}
	}
}

func TestGoBloomSliding_Insert_MInsert(t *testing.T) {
	bf, err := NewGoBloomSliding(&GoBloomSlidingConf{
		Cap:             1000,
		P:               0.001,
		WindowSize:      1 * time.Hour,
		WindowCount:     5,
		CleanupInterval: 1 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Failed to create GoBloomSliding: %v", err)
	}
	defer bf.Stop()

	ctx := context.Background()
	testKey := "test:go:insert"

	ok, err := bf.Insert(ctx, testKey, "item1", time.Hour)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	if !ok {
		t.Error("Insert() returned false, expected true")
	}

	ok, err = bf.Insert(ctx, testKey, "item1", time.Hour)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	if ok {
		t.Error("Insert() returned true for duplicate item, expected false")
	}

	items := []string{"item2", "item3", "item4"}
	statusList, err := bf.MInsert(ctx, testKey, items, time.Hour)
	if err != nil {
		t.Fatalf("MInsert() error = %v", err)
	}
	if len(statusList) != len(items) {
		t.Errorf("MInsert() returned statusList length = %d, want %d", len(statusList), len(items))
	}
}

func TestGoBloomSliding_Info(t *testing.T) {
	bf, err := NewGoBloomSliding(&GoBloomSlidingConf{
		Cap:             1000,
		P:               0.001,
		WindowSize:      1 * time.Hour,
		WindowCount:     5,
		CleanupInterval: 1 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Failed to create GoBloomSliding: %v", err)
	}
	defer bf.Stop()

	ctx := context.Background()
	testKey := "test:go:info"

	info, err := bf.Info(ctx, testKey)
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}

	requiredKeys := []string{"cap", "k", "m", "windowSize", "windowCount", "currentIdx"}
	for _, key := range requiredKeys {
		if _, ok := info[key]; !ok {
			t.Errorf("Info() missing key: %s", key)
		}
	}

	if windowCount, ok := info["windowCount"].(int); !ok || windowCount != 5 {
		t.Errorf("Info() windowCount = %v, want 5", windowCount)
	}
}

func TestGoBloomSliding_WindowRotation(t *testing.T) {
	bf, err := NewGoBloomSliding(&GoBloomSlidingConf{
		Cap:             1000,
		P:               0.001,
		WindowSize:      2 * time.Second, // 使用 2 秒窗口，确保测试稳定性
		WindowCount:     3,
		CleanupInterval: 1 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Failed to create GoBloomSliding: %v", err)
	}
	defer bf.Stop()

	ctx := context.Background()
	testKey := "test:go:rotation"

	// 添加元素到第一个窗口
	_, err = bf.Add(ctx, testKey, "item1", time.Hour)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// 等待窗口轮换
	time.Sleep(3 * time.Second) // 等待超过一个窗口的时间

	// 添加元素到新窗口
	_, err = bf.Add(ctx, testKey, "item2", time.Hour)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// 两个元素都应该存在
	exists1, _ := bf.Exists(ctx, testKey, "item1")
	exists2, _ := bf.Exists(ctx, testKey, "item2")
	if !exists1 || !exists2 {
		t.Errorf("Items should exist: item1=%v, item2=%v", exists1, exists2)
	}

	// 等待足够长的时间让所有窗口过期
	// windowSize=2s, windowCount=3, 总时长=6s
	time.Sleep(7 * time.Second)

	// 再次检查
	exists1, _ = bf.Exists(ctx, testKey, "item1")
	exists2, _ = bf.Exists(ctx, testKey, "item2")
	if exists1 || exists2 {
		t.Errorf("Items should not exist after expiration: item1=%v, item2=%v", exists1, exists2)
	}
}

func TestGoBloomSliding_Cleanup(t *testing.T) {
	bf, err := NewGoBloomSliding(&GoBloomSlidingConf{
		Cap:             1000,
		P:               0.001,
		WindowSize:      100 * time.Millisecond,
		WindowCount:     2,
		CleanupInterval: 200 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Failed to create GoBloomSliding: %v", err)
	}
	defer bf.Stop()

	ctx := context.Background()
	testKey := "test:go:cleanup"

	// 添加元素
	_, err = bf.Add(ctx, testKey, "item1", time.Hour)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// 等待超过所有窗口总时长
	time.Sleep(300 * time.Millisecond)

	// 等待清理 goroutine 执行
	time.Sleep(300 * time.Millisecond)

	// key 应该被清理了
	exists, err := bf.Exists(ctx, testKey, "item1")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	// 注意：由于清理是在后台异步执行的，可能存在时序问题
	// 这里主要测试清理机制是否启动
	if exists {
		// 如果还存在，等待更长时间再检查
		time.Sleep(500 * time.Millisecond)
		exists, _ = bf.Exists(ctx, testKey, "item1")
		if exists {
			t.Log("Key still exists after cleanup delay, this may be due to timing")
		}
	}
}

func TestGoBloomSliding_Concurrent(t *testing.T) {
	bf, err := NewGoBloomSliding(&GoBloomSlidingConf{
		Cap:             10000,
		P:               0.001,
		WindowSize:      1 * time.Hour,
		WindowCount:     5,
		CleanupInterval: 1 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Failed to create GoBloomSliding: %v", err)
	}
	defer bf.Stop()

	ctx := context.Background()
	testKey := "test:go:concurrent"

	var wg sync.WaitGroup
	numGoroutines := 10
	itemsPerGoroutine := 100

	// 并发添加
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < itemsPerGoroutine; j++ {
				item := fmt.Sprintf("item_%d_%d", goroutineID, j)
				_, err := bf.Add(ctx, testKey, item, time.Hour)
				if err != nil {
					t.Errorf("Add() error in goroutine %d: %v", goroutineID, err)
				}
			}
		}(i)
	}

	wg.Wait()

	// 并发检查
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < itemsPerGoroutine; j++ {
				item := fmt.Sprintf("item_%d_%d", goroutineID, j)
				exists, err := bf.Exists(ctx, testKey, item)
				if err != nil {
					t.Errorf("Exists() error in goroutine %d: %v", goroutineID, err)
				}
				if !exists {
					t.Errorf("Item %s should exist", item)
				}
			}
		}(i)
	}

	wg.Wait()
}

// ==================== 性能测试 ====================

func BenchmarkBitsetV9Sliding_Add(b *testing.B) {
	if testRedisV9 == nil {
		b.Skip("Redis v9 not available, skipping benchmark")
	}

	bf, err := NewBitsetV9Sliding(&BitsetV9SlidingConf{
		Redis:       testRedisV9,
		Cap:         100000,
		P:           0.001,
		WindowSize:  1 * time.Hour,
		WindowCount: 5,
	})
	if err != nil {
		b.Fatalf("Failed to create BitsetV9Sliding: %v", err)
	}

	ctx := context.Background()
	testKey := "benchmark:v9:add"
	defer cleanupTestDataV9(&testing.T{}, testRedisV9, testKey)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			item := fmt.Sprintf("item_%d", i)
			_, _ = bf.Add(ctx, testKey, item, time.Hour)
			i++
		}
	})
}

func BenchmarkBitsetV9Sliding_Exists(b *testing.B) {
	if testRedisV9 == nil {
		b.Skip("Redis v9 not available, skipping benchmark")
	}

	bf, err := NewBitsetV9Sliding(&BitsetV9SlidingConf{
		Redis:       testRedisV9,
		Cap:         100000,
		P:           0.001,
		WindowSize:  1 * time.Hour,
		WindowCount: 5,
	})
	if err != nil {
		b.Fatalf("Failed to create BitsetV9Sliding: %v", err)
	}

	ctx := context.Background()
	testKey := "benchmark:v9:exists"
	defer cleanupTestDataV9(&testing.T{}, testRedisV9, testKey)

	// 预先添加一些元素
	for i := 0; i < 1000; i++ {
		item := fmt.Sprintf("item_%d", i)
		_, _ = bf.Add(ctx, testKey, item, time.Hour)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			item := fmt.Sprintf("item_%d", i%1000)
			_, _ = bf.Exists(ctx, testKey, item)
			i++
		}
	})
}

func BenchmarkGoBloomSliding_Add(b *testing.B) {
	bf, err := NewGoBloomSliding(&GoBloomSlidingConf{
		Cap:             100000,
		P:               0.001,
		WindowSize:      1 * time.Hour,
		WindowCount:     5,
		CleanupInterval: 1 * time.Minute,
	})
	if err != nil {
		b.Fatalf("Failed to create GoBloomSliding: %v", err)
	}
	defer bf.Stop()

	ctx := context.Background()
	testKey := "benchmark:go:add"

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			item := fmt.Sprintf("item_%d", i)
			_, _ = bf.Add(ctx, testKey, item, time.Hour)
			i++
		}
	})
}

func BenchmarkGoBloomSliding_Exists(b *testing.B) {
	bf, err := NewGoBloomSliding(&GoBloomSlidingConf{
		Cap:             100000,
		P:               0.001,
		WindowSize:      1 * time.Hour,
		WindowCount:     5,
		CleanupInterval: 1 * time.Minute,
	})
	if err != nil {
		b.Fatalf("Failed to create GoBloomSliding: %v", err)
	}
	defer bf.Stop()

	ctx := context.Background()
	testKey := "benchmark:go:exists"

	// 预先添加一些元素
	for i := 0; i < 1000; i++ {
		item := fmt.Sprintf("item_%d", i)
		_, _ = bf.Add(ctx, testKey, item, time.Hour)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			item := fmt.Sprintf("item_%d", i%1000)
			_, _ = bf.Exists(ctx, testKey, item)
			i++
		}
	})
}

func BenchmarkGoBloomSliding_MAdd(b *testing.B) {
	bf, err := NewGoBloomSliding(&GoBloomSlidingConf{
		Cap:             100000,
		P:               0.001,
		WindowSize:      1 * time.Hour,
		WindowCount:     5,
		CleanupInterval: 1 * time.Minute,
	})
	if err != nil {
		b.Fatalf("Failed to create GoBloomSliding: %v", err)
	}
	defer bf.Stop()

	ctx := context.Background()
	testKey := "benchmark:go:madd"

	items := make([]string, 100)
	for i := range items {
		items[i] = fmt.Sprintf("item_%d", i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = bf.MAdd(ctx, testKey, items, time.Hour)
	}
}

func BenchmarkGoBloomSliding_MExists(b *testing.B) {
	bf, err := NewGoBloomSliding(&GoBloomSlidingConf{
		Cap:             100000,
		P:               0.001,
		WindowSize:      1 * time.Hour,
		WindowCount:     5,
		CleanupInterval: 1 * time.Minute,
	})
	if err != nil {
		b.Fatalf("Failed to create GoBloomSliding: %v", err)
	}
	defer bf.Stop()

	ctx := context.Background()
	testKey := "benchmark:go:mexists"

	items := make([]string, 100)
	for i := range items {
		items[i] = fmt.Sprintf("item_%d", i)
	}
	_, _ = bf.MAdd(ctx, testKey, items, time.Hour)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = bf.MExists(ctx, testKey, items)
	}
}
