package hotspotperception

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestNewTinyLFUHotspot(t *testing.T) {
	// 测试创建热点感知器
	hotspot := NewTinyLFUHotspot(1000, 10, 1*time.Minute)

	if hotspot == nil {
		t.Fatal("NewTinyLFUHotspot should not return nil")
	}

	if hotspot.width <= 0 {
		t.Errorf("width should be positive, got %d", hotspot.width)
	}

	if hotspot.depth <= 0 {
		t.Errorf("depth should be positive, got %d", hotspot.depth)
	}

	if len(hotspot.sketches) != hotspot.depth {
		t.Errorf("sketches length should equal depth, got %d, expected %d", len(hotspot.sketches), hotspot.depth)
	}

	if len(hotspot.bloomFilter) <= 0 {
		t.Errorf("bloomFilter should not be empty")
	}
}

func TestMarkAndGet(t *testing.T) {
	hotspot := NewTinyLFUHotspot(1000, 5, 1*time.Minute)
	ctx := context.Background()

	// 测试单个key的访问
	key := "test_key"

	// 初始状态应该为0
	if count := hotspot.Get(ctx, key); count != 0 {
		t.Errorf("initial count should be 0, got %d", count)
	}

	// 标记访问
	hotspot.Mark(ctx, key, 1)

	// 检查访问计数
	if count := hotspot.Get(ctx, key); count < 1 {
		t.Errorf("count should be at least 1, got %d", count)
	}

	// 多次访问
	hotspot.Mark(ctx, key, 2)
	hotspot.Mark(ctx, key, 3)

	// 检查累计计数
	if count := hotspot.Get(ctx, key); count < 6 {
		t.Errorf("count should be at least 6, got %d", count)
	}
}

func TestIsHotspot(t *testing.T) {
	threshold := int64(5)
	hotspot := NewTinyLFUHotspot(1000, threshold, 1*time.Minute)
	ctx := context.Background()

	key := "test_key"

	// 初始状态不应该热点
	if hotspot.IsHotspot(ctx, key) {
		t.Error("key should not be hotspot initially")
	}

	// 访问次数达到阈值
	for i := int64(0); i < threshold; i++ {
		hotspot.Mark(ctx, key, 1)
	}

	// 现在应该是热点
	if !hotspot.IsHotspot(ctx, key) {
		t.Error("key should be hotspot after reaching threshold")
	}

	// 测试另一个key
	key2 := "test_key2"
	if hotspot.IsHotspot(ctx, key2) {
		t.Error("key2 should not be hotspot")
	}
}

func TestMultipleKeys(t *testing.T) {
	hotspot := NewTinyLFUHotspot(1000, 10, 1*time.Minute)
	ctx := context.Background()

	// 为每个key设置不同的访问次数
	expectedCounts := map[string]int64{
		"key1": 1,
		"key2": 5,
		"key3": 10,
		"key4": 15,
		"key5": 20,
	}

	// 标记访问
	for key, count := range expectedCounts {
		hotspot.Mark(ctx, key, count)
	}

	// 验证每个key的计数
	for key, expectedCount := range expectedCounts {
		actualCount := hotspot.Get(ctx, key)
		if actualCount < expectedCount {
			t.Errorf("key %s: expected at least %d, got %d", key, expectedCount, actualCount)
		}
	}
}

func TestBloomFilter(t *testing.T) {
	hotspot := NewTinyLFUHotspot(1000, 5, 1*time.Minute)
	ctx := context.Background()

	// 测试bloom filter的假阳性
	// 生成一些随机key
	keys := make([]string, 100)
	for i := 0; i < 100; i++ {
		keys[i] = fmt.Sprintf("key_%d", i)
	}

	// 只访问前50个key
	for i := 0; i < 50; i++ {
		hotspot.Mark(ctx, keys[i], 1)
	}

	// 检查未访问的key应该返回0
	for i := 50; i < 100; i++ {
		if count := hotspot.Get(ctx, keys[i]); count != 0 {
			t.Errorf("unaccessed key %s should return 0, got %d", keys[i], count)
		}
	}
}

func TestConcurrentAccess(t *testing.T) {
	hotspot := NewTinyLFUHotspot(1000, 10, 1*time.Minute)
	ctx := context.Background()

	var wg sync.WaitGroup
	numGoroutines := 10
	accessesPerGoroutine := 100

	// 启动多个goroutine并发访问
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < accessesPerGoroutine; j++ {
				key := fmt.Sprintf("key_%d_%d", id, j)
				hotspot.Mark(ctx, key, 1)
				hotspot.Get(ctx, key)
			}
		}(i)
	}

	wg.Wait()

	// 验证统计信息
	stats := hotspot.GetStats()
	if stats.TotalAccess < int64(numGoroutines*accessesPerGoroutine) {
		t.Errorf("total access should be at least %d, got %d",
			numGoroutines*accessesPerGoroutine, stats.TotalAccess)
	}
}

func TestReset(t *testing.T) {
	hotspot := NewTinyLFUHotspot(100, 5, 1*time.Minute) // 较小的resetPeriod
	ctx := context.Background()

	key := "test_key"

	// 访问key
	hotspot.Mark(ctx, key, 10)
	initialCount := hotspot.Get(ctx, key)

	// 触发重置（通过访问次数）
	for i := 0; i < 1000; i++ { // 确保触发重置
		hotspot.Mark(ctx, "trigger_key", 1)
	}

	// 重置后计数应该减少
	afterResetCount := hotspot.Get(ctx, key)
	if afterResetCount >= initialCount {
		t.Errorf("count should decrease after reset, got %d, was %d", afterResetCount, initialCount)
	}
}

func TestTimeWindowReset(t *testing.T) {
	// 使用很短的时间窗口进行测试
	hotspot := NewTinyLFUHotspot(1000, 5, 10*time.Millisecond)
	ctx := context.Background()

	key := "test_key"

	// 访问key
	hotspot.Mark(ctx, key, 10)
	initialCount := hotspot.Get(ctx, key)

	// 等待时间窗口过期
	time.Sleep(20 * time.Millisecond)

	// 再次访问触发时间窗口重置
	hotspot.Mark(ctx, key, 1)

	// 重置后计数应该减少
	afterResetCount := hotspot.Get(ctx, key)
	if afterResetCount >= initialCount {
		t.Errorf("count should decrease after time window reset, got %d, was %d", afterResetCount, initialCount)
	}
}

func TestGetStats(t *testing.T) {
	hotspot := NewTinyLFUHotspot(1000, 5, 1*time.Minute)
	ctx := context.Background()

	// 初始统计信息
	initialStats := hotspot.GetStats()
	if initialStats.TotalAccess != 0 {
		t.Errorf("initial total access should be 0, got %d", initialStats.TotalAccess)
	}

	// 进行一些访问
	hotspot.Mark(ctx, "key1", 1)
	hotspot.Mark(ctx, "key2", 2)
	hotspot.Get(ctx, "key1")
	hotspot.Get(ctx, "key3") // 这个key没有被访问过

	// 检查统计信息
	stats := hotspot.GetStats()
	if stats.TotalAccess < 3 {
		t.Errorf("total access should be at least 3, got %d", stats.TotalAccess)
	}

	// 由于bloom filter的存在，sketch hits可能少于预期
	if stats.SketchHits < 1 {
		t.Errorf("sketch hits should be at least 1, got %d", stats.SketchHits)
	}

	if stats.BloomHits < 1 {
		t.Errorf("bloom hits should be at least 1, got %d", stats.BloomHits)
	}
}

func TestGetTopHotspots(t *testing.T) {
	hotspot := NewTinyLFUHotspot(1000, 5, 1*time.Minute)
	ctx := context.Background()

	// TinyLFU不适合获取排行榜，应该返回空切片
	hotspots := hotspot.GetTopHotspots(ctx, 10)
	if len(hotspots) != 0 {
		t.Errorf("GetTopHotspots should return empty slice, got %d items", len(hotspots))
	}
}

func TestCleanup(t *testing.T) {
	hotspot := NewTinyLFUHotspot(1000, 5, 1*time.Minute)
	ctx := context.Background()

	key := "test_key"

	// 访问key
	hotspot.Mark(ctx, key, 10)
	initialCount := hotspot.Get(ctx, key)

	// 执行清理
	hotspot.Cleanup(ctx)

	// 清理后计数应该减少
	afterCleanupCount := hotspot.Get(ctx, key)
	if afterCleanupCount >= initialCount {
		t.Errorf("count should decrease after cleanup, got %d, was %d", afterCleanupCount, initialCount)
	}
}

func TestEdgeCases(t *testing.T) {
	hotspot := NewTinyLFUHotspot(1000, 5, 1*time.Minute)
	ctx := context.Background()

	// 测试空字符串
	hotspot.Mark(ctx, "", 1)
	if count := hotspot.Get(ctx, ""); count < 1 {
		t.Errorf("empty string key should work, got %d", count)
	}

	// 测试零计数
	hotspot.Mark(ctx, "zero_key", 0)
	if count := hotspot.Get(ctx, "zero_key"); count != 0 {
		t.Errorf("zero count should result in 0, got %d", count)
	}

	// 测试负数计数（应该被正常处理）
	hotspot.Mark(ctx, "negative_key", -1)
	if count := hotspot.Get(ctx, "negative_key"); count != -1 {
		t.Errorf("negative count should be preserved, got %d", count)
	}
}

func TestMemoryEfficiency(t *testing.T) {
	// 测试内存效率
	hotspot := NewTinyLFUHotspot(10000, 10, 1*time.Minute)
	ctx := context.Background()

	// 大量key访问
	numKeys := 1000
	for i := 0; i < numKeys; i++ {
		key := fmt.Sprintf("key_%d", i)
		hotspot.Mark(ctx, key, int64(i%10+1))
	}

	// 验证所有key都能正确获取
	for i := 0; i < numKeys; i++ {
		key := fmt.Sprintf("key_%d", i)
		if count := hotspot.Get(ctx, key); count < 1 {
			t.Errorf("key %s should have count >= 1, got %d", key, count)
		}
	}
}

// 基准测试
func BenchmarkMark(b *testing.B) {
	hotspot := NewTinyLFUHotspot(10000, 10, 1*time.Minute)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key_%d", i%100)
		hotspot.Mark(ctx, key, 1)
	}
}

func BenchmarkGet(b *testing.B) {
	hotspot := NewTinyLFUHotspot(10000, 10, 1*time.Minute)
	ctx := context.Background()

	// 预先填充一些数据
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key_%d", i)
		hotspot.Mark(ctx, key, 5)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key_%d", i%100)
		hotspot.Get(ctx, key)
	}
}

func BenchmarkConcurrentMark(b *testing.B) {
	hotspot := NewTinyLFUHotspot(10000, 10, 1*time.Minute)
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key_%d", i%100)
			hotspot.Mark(ctx, key, 1)
			i++
		}
	})
}
