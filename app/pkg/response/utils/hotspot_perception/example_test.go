package hotspotperception

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ExampleBasicUsage 演示基本使用方法
func ExampleBasicUsage() {
	// 创建热点感知器
	hotspot := NewTinyLFUHotspot(1000, 5, 1*time.Minute)
	ctx := context.Background()

	// 标记一些访问
	hotspot.Mark(ctx, "user_123", 1)
	hotspot.Mark(ctx, "user_456", 2)
	hotspot.Mark(ctx, "user_789", 3)

	// 检查是否为热点
	if hotspot.IsHotspot(ctx, "user_123") {
		fmt.Println("user_123 是热点")
	} else {
		fmt.Println("user_123 不是热点")
	}

	// 获取访问次数
	count := hotspot.Get(ctx, "user_456")
	fmt.Printf("user_456 的访问次数: %d\n", count)

	// Output:
	// user_123 不是热点
	// user_456 的访问次数: 2
}

// ExampleConcurrentUsage 演示并发使用
func ExampleConcurrentUsage() {
	hotspot := NewTinyLFUHotspot(10000, 10, 1*time.Minute)
	ctx := context.Background()

	// 模拟并发访问
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("key_%d", id)
			hotspot.Mark(ctx, key, 1)
		}(i)
	}
	wg.Wait()

	// 检查结果
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("key_%d", i)
		count := hotspot.Get(ctx, key)
		fmt.Printf("%s: %d\n", key, count)
	}

	// Output:
	// key_0: 1
	// key_1: 1
	// key_2: 1
	// key_3: 1
	// key_4: 1
}

// ExampleStats 演示统计信息
func ExampleStats() {
	hotspot := NewTinyLFUHotspot(1000, 5, 1*time.Minute)
	ctx := context.Background()

	// 进行一些访问
	hotspot.Mark(ctx, "key1", 1)
	hotspot.Mark(ctx, "key2", 2)
	hotspot.Get(ctx, "key1")

	// 获取统计信息
	stats := hotspot.GetStats()
	fmt.Printf("总访问次数: %d\n", stats.TotalAccess)
	fmt.Printf("Sketch命中次数: %d\n", stats.SketchHits)
	fmt.Printf("Bloom命中次数: %d\n", stats.BloomHits)

	// Output:
	// 总访问次数: 3
	// Sketch命中次数: 1
	// Bloom命中次数: 1
}
