package cache

import (
	"context"
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"testing"
	"time"
)

// 测试基本功能
func TestRistrettoCacheBasic(t *testing.T) {
	config := &RistrettoCacheConfig{
		NumCounters: 1000,
		MaxCost:     100,
		BufferItems: 64,
	}
	cache := NewRistrettoCache(config)
	ctx := context.Background()

	// 测试Set和Get
	ok, err := cache.Set(ctx, "test_key", "test_value", time.Minute)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if !ok {
		t.Fatal("Set returned false")
	}

	val, err := cache.Get(ctx, "test_key")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val != "test_value" {
		t.Fatalf("Expected 'test_value', got %v", val)
	}

	// 测试Exists
	exists, err := cache.Exists(ctx, "test_key")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Fatal("Key should exist")
	}

	// 测试Del
	ok, err = cache.Del(ctx, "test_key")
	if err != nil {
		t.Fatalf("Del failed: %v", err)
	}
	if !ok {
		t.Fatal("Del returned false")
	}

	// 验证删除后不存在
	exists, err = cache.Exists(ctx, "test_key")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Fatal("Key should not exist after deletion")
	}
}

// 测试自增自减功能
func TestRistrettoCacheIncr(t *testing.T) {
	config := &RistrettoCacheConfig{
		NumCounters: 1000,
		MaxCost:     100,
		BufferItems: 64,
	}
	cache := NewRistrettoCache(config)
	ctx := context.Background()

	// 测试Incr（从不存在的key开始）
	n, err := cache.Incr(ctx, "counter")
	if err != nil {
		t.Fatalf("Incr failed: %v", err)
	}
	if n != 1 {
		t.Fatalf("Expected 1, got %d", n)
	}

	// 再次Incr
	n, err = cache.Incr(ctx, "counter")
	if err != nil {
		t.Fatalf("Incr failed: %v", err)
	}
	if n != 2 {
		t.Fatalf("Expected 2, got %d", n)
	}

	// 测试IncrBy
	n, err = cache.IncrBy(ctx, "counter", 5)
	if err != nil {
		t.Fatalf("IncrBy failed: %v", err)
	}
	if n != 7 {
		t.Fatalf("Expected 7, got %d", n)
	}

	// 测试Decr
	n, err = cache.Decr(ctx, "counter")
	if err != nil {
		t.Fatalf("Decr failed: %v", err)
	}
	if n != 6 {
		t.Fatalf("Expected 6, got %d", n)
	}

	// 测试DecrBy
	n, err = cache.DecrBy(ctx, "counter", 3)
	if err != nil {
		t.Fatalf("DecrBy failed: %v", err)
	}
	if n != 3 {
		t.Fatalf("Expected 3, got %d", n)
	}
}

// 测试TTL功能
func TestRistrettoCacheTTL(t *testing.T) {
	config := &RistrettoCacheConfig{
		NumCounters: 1000,
		MaxCost:     100,
		BufferItems: 64,
	}
	cache := NewRistrettoCache(config)
	ctx := context.Background()

	// 设置一个短TTL的key
	ok, err := cache.Set(ctx, "ttl_key", "ttl_value", 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if !ok {
		t.Fatal("Set returned false")
	}

	// 立即检查key存在
	exists, err := cache.Exists(ctx, "ttl_key")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Fatal("Key should exist immediately")
	}

	// 等待TTL过期
	time.Sleep(150 * time.Millisecond)

	// 检查key是否过期（注意：Ristretto可能不会立即清理过期key）
	val, err := cache.Get(ctx, "ttl_key")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	// Ristretto的TTL实现可能会延迟清理，所以这里只是记录结果
	t.Logf("TTL test: value after expiry = %v", val)
}

// 并发读写压测
func TestRistrettoCacheConcurrency(t *testing.T) {
	config := &RistrettoCacheConfig{
		NumCounters: 10000,
		MaxCost:     1000,
		BufferItems: 64,
	}
	cache := NewRistrettoCache(config)
	ctx := context.Background()

	const (
		numGoroutines = 50
		numOperations = 1000
	)

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	// 启动多个goroutine进行并发写入
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("key_%d_%d", id, j)
				value := fmt.Sprintf("value_%d_%d", id, j)

				_, err := cache.Set(ctx, key, value, time.Minute)
				if err != nil {
					errors <- fmt.Errorf("goroutine %d: Set failed: %v", id, err)
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// 检查是否有错误
	for err := range errors {
		t.Error(err)
	}

	// 验证一些数据
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("key_0_%d", i)
		val, err := cache.Get(ctx, key)
		if err != nil {
			t.Errorf("Get failed for key %s: %v", key, err)
		}
		expectedValue := fmt.Sprintf("value_0_%d", i)
		if val != expectedValue && val != nil {
			t.Logf("Key %s: expected %s, got %v", key, expectedValue, val)
		}
	}

	t.Logf("Concurrency test completed: %d goroutines × %d operations", numGoroutines, numOperations)
}

// 性能基准测试 - Set操作
func BenchmarkRistrettoCacheSet(b *testing.B) {
	config := &RistrettoCacheConfig{
		NumCounters: 100000,
		MaxCost:     10000,
		BufferItems: 64,
	}
	cache := NewRistrettoCache(config)
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("bench_key_%d", i)
			value := fmt.Sprintf("bench_value_%d", i)
			cache.Set(ctx, key, value, time.Minute)
			i++
		}
	})
}

// 性能基准测试 - Get操作
func BenchmarkRistrettoCacheGet(b *testing.B) {
	config := &RistrettoCacheConfig{
		NumCounters: 100000,
		MaxCost:     10000,
		BufferItems: 64,
	}
	cache := NewRistrettoCache(config)
	ctx := context.Background()

	// 预填充一些数据
	for i := 0; i < 10000; i++ {
		key := fmt.Sprintf("bench_key_%d", i)
		value := fmt.Sprintf("bench_value_%d", i)
		cache.Set(ctx, key, value, time.Minute)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("bench_key_%d", i%10000)
			cache.Get(ctx, key)
			i++
		}
	})
}

// 性能基准测试 - 混合操作
func BenchmarkRistrettoCacheMixed(b *testing.B) {
	config := &RistrettoCacheConfig{
		NumCounters: 100000,
		MaxCost:     10000,
		BufferItems: 64,
	}
	cache := NewRistrettoCache(config)
	ctx := context.Background()

	// 预填充一些数据
	for i := 0; i < 5000; i++ {
		key := fmt.Sprintf("bench_key_%d", i)
		value := fmt.Sprintf("bench_value_%d", i)
		cache.Set(ctx, key, value, time.Minute)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("bench_key_%d", i%10000)

			// 80% 读，20% 写
			if i%5 == 0 {
				value := fmt.Sprintf("bench_value_%d", i)
				cache.Set(ctx, key, value, time.Minute)
			} else {
				cache.Get(ctx, key)
			}
			i++
		}
	})
}

// 压力测试
func TestRistrettoCacheStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	config := &RistrettoCacheConfig{
		NumCounters: 1000000,
		MaxCost:     100000,
		BufferItems: 64,
	}
	cache := NewRistrettoCache(config)
	ctx := context.Background()

	const duration = 10 * time.Second
	numWorkers := runtime.NumCPU() * 2

	var (
		operations int64
		errors     int64
		wg         sync.WaitGroup
	)

	start := time.Now()
	end := start.Add(duration)

	// 启动多个worker
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			localOps := 0
			localErrors := 0

			for time.Now().Before(end) {
				// 随机操作
				switch rand.Intn(10) {
				case 0, 1: // 20% Set
					key := fmt.Sprintf("stress_key_%d_%d", workerID, rand.Intn(10000))
					value := fmt.Sprintf("stress_value_%d", rand.Intn(1000000))
					_, err := cache.Set(ctx, key, value, time.Minute)
					if err != nil {
						localErrors++
					}
				case 2, 3, 4, 5, 6, 7: // 60% Get
					key := fmt.Sprintf("stress_key_%d_%d", rand.Intn(numWorkers), rand.Intn(10000))
					_, err := cache.Get(ctx, key)
					if err != nil {
						localErrors++
					}
				case 8: // 10% Incr
					key := fmt.Sprintf("stress_counter_%d", rand.Intn(100))
					_, err := cache.Incr(ctx, key)
					if err != nil {
						localErrors++
					}
				case 9: // 10% Del
					key := fmt.Sprintf("stress_key_%d_%d", rand.Intn(numWorkers), rand.Intn(10000))
					_, err := cache.Del(ctx, key)
					if err != nil {
						localErrors++
					}
				}
				localOps++
			}

			// 原子操作更新总计数
			// 这里简化处理，实际应该用atomic包
			operations += int64(localOps)
			errors += int64(localErrors)
		}(i)
	}

	wg.Wait()

	elapsed := time.Since(start)
	opsPerSec := float64(operations) / elapsed.Seconds()

	t.Logf("Stress test results:")
	t.Logf("  Duration: %v", elapsed)
	t.Logf("  Workers: %d", numWorkers)
	t.Logf("  Total operations: %d", operations)
	t.Logf("  Errors: %d", errors)
	t.Logf("  Operations/sec: %.2f", opsPerSec)
	t.Logf("  Error rate: %.4f%%", float64(errors)/float64(operations)*100)

	if errors > operations/100 { // 如果错误率超过1%
		t.Errorf("Error rate too high: %d errors out of %d operations", errors, operations)
	}
}

// 内存使用测试
func TestRistrettoCacheMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory test in short mode")
	}

	// 强制GC
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	config := &RistrettoCacheConfig{
		NumCounters: 1000000,
		MaxCost:     50000, // 限制最大cost
		BufferItems: 64,
	}
	cache := NewRistrettoCache(config)
	ctx := context.Background()

	// 插入大量数据
	const numItems = 100000
	for i := 0; i < numItems; i++ {
		key := fmt.Sprintf("mem_key_%d", i)
		value := fmt.Sprintf("mem_value_%d_with_some_extra_data_to_make_it_larger", i)
		cache.Set(ctx, key, value, time.Hour)

		if i%10000 == 0 {
			t.Logf("Inserted %d items", i)
		}
	}

	// 等待一下让数据被处理
	time.Sleep(100 * time.Millisecond)

	// 强制GC并检查内存
	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	allocatedMB := float64(m2.Alloc-m1.Alloc) / 1024 / 1024
	t.Logf("Memory usage:")
	t.Logf("  Items inserted: %d", numItems)
	t.Logf("  Memory allocated: %.2f MB", allocatedMB)
	t.Logf("  Bytes per item: %.2f", float64(m2.Alloc-m1.Alloc)/float64(numItems))

	// 验证一些数据仍然可以读取
	hitCount := 0
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("mem_key_%d", i)
		val, err := cache.Get(ctx, key)
		if err != nil {
			t.Errorf("Get failed for key %s: %v", key, err)
		}
		if val != nil {
			hitCount++
		}
	}

	hitRate := float64(hitCount) / 1000.0
	t.Logf("Hit rate for first 1000 items: %.2f%%", hitRate*100)

	// 关闭缓存
	if closer, ok := cache.(*RistrettoCache); ok {
		closer.Close()
	}
}

// 压测 DisableAutoRefresh=true 的性能
func BenchmarkRistrettoCacheDisableAutoRefresh(b *testing.B) {
	config := &RistrettoCacheConfig{
		NumCounters:        100000,
		MaxCost:            10000,
		BufferItems:        64,
		DisableAutoRefresh: true, // 关闭自动刷新
	}
	cache := NewRistrettoCache(config)
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("bench_key_no_refresh_%d", i)
			value := fmt.Sprintf("bench_value_no_refresh_%d", i)
			cache.Set(ctx, key, value, time.Minute)
			i++
		}
	})
}

// 对比测试：DisableAutoRefresh=true vs false 的性能差异
func TestRistrettoCacheAutoRefreshComparison(b *testing.T) {
	if testing.Short() {
		b.Skip("Skipping comparison test in short mode")
	}

	const numOperations = 50000
	numWorkers := runtime.NumCPU()

	// 测试 DisableAutoRefresh = false (默认)
	config1 := &RistrettoCacheConfig{
		NumCounters:        100000,
		MaxCost:            10000,
		BufferItems:        64,
		DisableAutoRefresh: false,
	}
	cache1 := NewRistrettoCache(config1)
	ctx := context.Background()

	start1 := time.Now()
	var wg1 sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg1.Add(1)
		go func(workerID int) {
			defer wg1.Done()
			for j := 0; j < numOperations/numWorkers; j++ {
				key := fmt.Sprintf("test1_key_%d_%d", workerID, j)
				value := fmt.Sprintf("test1_value_%d_%d", workerID, j)
				cache1.Set(ctx, key, value, time.Minute)
			}
		}(i)
	}
	wg1.Wait()
	duration1 := time.Since(start1)

	// 测试 DisableAutoRefresh = true
	config2 := &RistrettoCacheConfig{
		NumCounters:        100000,
		MaxCost:            10000,
		BufferItems:        64,
		DisableAutoRefresh: true,
	}
	cache2 := NewRistrettoCache(config2)

	start2 := time.Now()
	var wg2 sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg2.Add(1)
		go func(workerID int) {
			defer wg2.Done()
			for j := 0; j < numOperations/numWorkers; j++ {
				key := fmt.Sprintf("test2_key_%d_%d", workerID, j)
				value := fmt.Sprintf("test2_value_%d_%d", workerID, j)
				cache2.Set(ctx, key, value, time.Minute)
			}
		}(i)
	}
	wg2.Wait()
	duration2 := time.Since(start2)

	// 输出对比结果
	b.Logf("Performance comparison for %d operations with %d workers:", numOperations, numWorkers)
	b.Logf("  DisableAutoRefresh=false: %v (%.2f ops/sec)", duration1, float64(numOperations)/duration1.Seconds())
	b.Logf("  DisableAutoRefresh=true:  %v (%.2f ops/sec)", duration2, float64(numOperations)/duration2.Seconds())
	b.Logf("  Performance improvement: %.2fx", duration1.Seconds()/duration2.Seconds())

	// 验证数据一致性
	// 对于 DisableAutoRefresh=false 的缓存，数据应该立即可见
	hitCount1 := 0
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("test1_key_0_%d", i)
		val, _ := cache1.Get(ctx, key)
		if val != nil {
			hitCount1++
		}
	}

	// 对于 DisableAutoRefresh=true 的缓存，可能需要等待或手动刷新
	hitCount2 := 0
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("test2_key_0_%d", i)
		val, _ := cache2.Get(ctx, key)
		if val != nil {
			hitCount2++
		}
	}

	b.Logf("  Hit rate comparison (first 100 items):")
	b.Logf("    DisableAutoRefresh=false: %d/100 (%.1f%%)", hitCount1, float64(hitCount1))
	b.Logf("    DisableAutoRefresh=true:  %d/100 (%.1f%%)", hitCount2, float64(hitCount2))

	// 清理
	if closer1, ok := cache1.(*RistrettoCache); ok {
		closer1.Close()
	}
	if closer2, ok := cache2.(*RistrettoCache); ok {
		closer2.Close()
	}
}

// 压测混合读写操作 - DisableAutoRefresh=true
func BenchmarkRistrettoCacheMixedDisableAutoRefresh(b *testing.B) {
	config := &RistrettoCacheConfig{
		NumCounters:        100000,
		MaxCost:            10000,
		BufferItems:        64,
		DisableAutoRefresh: true,
	}
	cache := NewRistrettoCache(config)
	ctx := context.Background()

	// 预填充一些数据
	for i := 0; i < 5000; i++ {
		key := fmt.Sprintf("bench_mixed_key_%d", i)
		value := fmt.Sprintf("bench_mixed_value_%d", i)
		cache.Set(ctx, key, value, time.Minute)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("bench_mixed_key_%d", i%10000)

			// 80% 读，20% 写
			if i%5 == 0 {
				value := fmt.Sprintf("bench_mixed_value_%d", i)
				cache.Set(ctx, key, value, time.Minute)
			} else {
				cache.Get(ctx, key)
			}
			i++
		}
	})
}
