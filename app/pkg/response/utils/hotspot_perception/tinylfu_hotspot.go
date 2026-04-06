package hotspotperception

import (
	"context"
	"hash/fnv"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// TinyLFUHotspot 基于Ristretto TinyLFU算法的热点感知器
// 参考了Ristretto缓存库的admission policy设计
// 具有高性能、低内存占用的特点，适用于大规模热词发现
type TinyLFUHotspot struct {
	// Count-Min Sketch 参数
	width    int64     // sketch宽度
	depth    int       // sketch深度(hash函数数量)
	sketches [][]int64 // count-min sketch 二维数组

	// Bloom Filter 参数
	bloomFilter    []uint64 // bloom filter位数组
	bloomSize      int64    // bloom filter大小
	bloomHashCount int      // bloom filter hash函数数量

	// 热点检测参数
	threshold  int64         // 热点阈值
	windowSize time.Duration // 时间窗口大小
	lastReset  time.Time     // 上次重置时间

	// 并发控制
	mu          sync.RWMutex
	accessCount int64 // 总访问计数
	resetPeriod int64 // 重置周期（访问次数）

	// 性能统计
	stats *TinyLFUStats
}

// TinyLFUStats TinyLFU统计信息
type TinyLFUStats struct {
	TotalAccess    int64 // 总访问次数
	SketchHits     int64 // sketch命中次数
	BloomHits      int64 // bloom命中次数
	FalsePositives int64 // 假阳性次数
	ResetCount     int64 // 重置次数
	LastResetTime  time.Time
}

// NewTinyLFUHotspot 创建TinyLFU热点感知器
// numCounters: sketch计数器总数(建议设为预期key数量的10倍)
// threshold: 热点阈值
// windowSize: 时间窗口大小
func NewTinyLFUHotspot(numCounters int64, threshold int64, windowSize time.Duration) *TinyLFUHotspot {
	// 计算最优的width和depth
	// 使用经验公式: width ≈ numCounters / depth
	depth := 4 // 通常4个hash函数效果较好
	width := numCounters / int64(depth)
	if width < 1024 {
		width = 1024 // 最小宽度
	}

	// bloom filter大小设为sketch大小的1/4，减少内存占用
	bloomSize := width / 4
	bloomHashCount := 2 // bloom filter通常2个hash函数就够了

	t := &TinyLFUHotspot{
		width:          width,
		depth:          depth,
		sketches:       make([][]int64, depth),
		bloomFilter:    make([]uint64, bloomSize/64+1), // 64位为单位
		bloomSize:      bloomSize,
		bloomHashCount: bloomHashCount,
		threshold:      threshold,
		windowSize:     windowSize,
		lastReset:      time.Now(),
		resetPeriod:    numCounters * 10, // 每N次访问重置一次
		stats:          &TinyLFUStats{LastResetTime: time.Now()},
	}

	// 初始化sketch二维数组 - 使用预分配减少内存碎片
	for i := 0; i < depth; i++ {
		t.sketches[i] = make([]int64, width)
	}

	return t
}

// Mark 标记key访问，增加访问计数
func (t *TinyLFUHotspot) Mark(ctx context.Context, key string, count int64) {
	// 原子性增加总访问计数
	totalAccess := atomic.AddInt64(&t.accessCount, count)
	atomic.AddInt64(&t.stats.TotalAccess, count)

	// 检查是否需要重置(freshness机制)
	if totalAccess%t.resetPeriod == 0 {
		t.reset()
	}

	// 检查时间窗口重置
	t.mu.RLock()
	needTimeReset := time.Since(t.lastReset) > t.windowSize
	t.mu.RUnlock()

	if needTimeReset {
		t.reset()
	}

	// 更新bloom filter
	t.updateBloomFilter(key)

	// 更新count-min sketch
	t.updateSketch(key, count)
}

// Get 获取key的估计访问频率
func (t *TinyLFUHotspot) Get(ctx context.Context, key string) int64 {
	// 首先检查bloom filter
	if !t.checkBloomFilter(key) {
		return 0 // bloom filter说没有，那肯定没有
	}

	atomic.AddInt64(&t.stats.BloomHits, 1)

	// 从count-min sketch获取频率估计
	return t.estimateFromSketch(key)
}

// IsHotspot 判断是否为热点
func (t *TinyLFUHotspot) IsHotspot(ctx context.Context, key string) bool {
	return t.Get(ctx, key) >= t.threshold
}

// GetTopHotspots 获取前N个热点(注意：TinyLFU适合判断单个key是否热点，不适合排序)
func (t *TinyLFUHotspot) GetTopHotspots(ctx context.Context, n int) []HotspotItem {
	// TinyLFU主要用于判断单个key的热度，不适合获取排行榜
	// 这里返回空切片，实际应用中建议配合其他数据结构
	return []HotspotItem{}
}

// updateBloomFilter 更新bloom filter
func (t *TinyLFUHotspot) updateBloomFilter(key string) {
	hashes := t.bloomHashes(key)

	for i := 0; i < t.bloomHashCount; i++ {
		pos := hashes[i] % uint64(t.bloomSize)
		wordIndex := pos / 64
		bitIndex := pos % 64

		if wordIndex < uint64(len(t.bloomFilter)) {
			mask := uint64(1) << bitIndex
			// 使用原子操作进行bitwise OR
			for {
				old := atomic.LoadUint64(&t.bloomFilter[wordIndex])
				new := old | mask
				if atomic.CompareAndSwapUint64(&t.bloomFilter[wordIndex], old, new) {
					break
				}
			}
		}
	}
}

// checkBloomFilter 检查bloom filter
func (t *TinyLFUHotspot) checkBloomFilter(key string) bool {
	hashes := t.bloomHashes(key)

	for i := 0; i < t.bloomHashCount; i++ {
		pos := hashes[i] % uint64(t.bloomSize)
		wordIndex := pos / 64
		bitIndex := pos % 64

		if wordIndex >= uint64(len(t.bloomFilter)) {
			return false
		}

		word := atomic.LoadUint64(&t.bloomFilter[wordIndex])
		mask := uint64(1) << bitIndex
		if (word & mask) == 0 {
			return false
		}
	}

	return true
}

// updateSketch 更新count-min sketch
func (t *TinyLFUHotspot) updateSketch(key string, count int64) {
	hashes := t.sketchHashes(key)

	for i := 0; i < t.depth; i++ {
		pos := hashes[i] % uint64(t.width)
		atomic.AddInt64(&t.sketches[i][pos], count)
	}
}

// estimateFromSketch 从count-min sketch估计频率
func (t *TinyLFUHotspot) estimateFromSketch(key string) int64 {
	hashes := t.sketchHashes(key)
	minCount := int64(math.MaxInt64)

	for i := 0; i < t.depth; i++ {
		pos := hashes[i] % uint64(t.width)
		count := atomic.LoadInt64(&t.sketches[i][pos])
		if count < minCount {
			minCount = count
		}
	}

	atomic.AddInt64(&t.stats.SketchHits, 1)

	if minCount == math.MaxInt64 {
		return 0
	}
	return minCount
}

// reset 重置数据结构(freshness机制)
func (t *TinyLFUHotspot) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	// 重置时间
	t.lastReset = time.Now()
	atomic.AddInt64(&t.stats.ResetCount, 1)
	t.stats.LastResetTime = time.Now()

	// 重置bloom filter
	for i := range t.bloomFilter {
		atomic.StoreUint64(&t.bloomFilter[i], 0)
	}

	// 重置count-min sketch (使用衰减而不是完全清零)
	for i := 0; i < t.depth; i++ {
		for j := int64(0); j < t.width; j++ {
			// 将计数减半，保留一些历史信息
			oldVal := atomic.LoadInt64(&t.sketches[i][j])
			atomic.StoreInt64(&t.sketches[i][j], oldVal/2)
		}
	}

	// 重置访问计数器
	atomic.StoreInt64(&t.accessCount, 0)
}

// sketchHashes 为count-min sketch生成hash值 - 优化版本
func (t *TinyLFUHotspot) sketchHashes(key string) []uint64 {
	// 使用双重hash减少hash计算开销
	h1 := fnv.New64a()
	h1.Write([]byte(key))
	hash1 := h1.Sum64()

	h2 := fnv.New64()
	h2.Write([]byte(key))
	hash2 := h2.Sum64()

	// 确保hash2为奇数，避免周期性
	if hash2%2 == 0 {
		hash2++
	}

	// 预分配切片避免频繁分配
	hashes := make([]uint64, t.depth)
	for i := 0; i < t.depth; i++ {
		hashes[i] = hash1 + uint64(i)*hash2
	}

	return hashes
}

// bloomHashes 为bloom filter生成hash值 - 优化版本
func (t *TinyLFUHotspot) bloomHashes(key string) []uint64 {
	// 使用双重hash减少hash计算开销
	h1 := fnv.New64a()
	h1.Write([]byte(key))
	hash1 := h1.Sum64()

	h2 := fnv.New64()
	h2.Write([]byte(key))
	hash2 := h2.Sum64()

	// 确保hash2为奇数，避免周期性
	if hash2%2 == 0 {
		hash2++
	}

	// 预分配切片避免频繁分配
	hashes := make([]uint64, t.bloomHashCount)
	for i := 0; i < t.bloomHashCount; i++ {
		hashes[i] = hash1 + uint64(i)*hash2
	}

	return hashes
}

// GetStats 获取统计信息
func (t *TinyLFUHotspot) GetStats() *TinyLFUStats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	stats := *t.stats
	stats.TotalAccess = atomic.LoadInt64(&t.stats.TotalAccess)
	stats.SketchHits = atomic.LoadInt64(&t.stats.SketchHits)
	stats.BloomHits = atomic.LoadInt64(&t.stats.BloomHits)
	stats.FalsePositives = atomic.LoadInt64(&t.stats.FalsePositives)
	stats.ResetCount = atomic.LoadInt64(&t.stats.ResetCount)

	return &stats
}

// Cleanup 清理过期数据
func (t *TinyLFUHotspot) Cleanup(ctx context.Context) {
	// 触发一次重置
	t.reset()
}
