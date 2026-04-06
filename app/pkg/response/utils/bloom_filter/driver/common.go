package driver

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
)

// locationPool 用于复用 []interface{} 切片，避免高频内存分配
var locationPool = sync.Pool{
	New: func() interface{} {
		// 预分配容量为 20，适配大多数场景 (k 通常在 10-15 之间)
		return make([]interface{}, 0, 20)
	},
}

// largeLocationPool 用于复用大容量切片（用于 MExists 批量操作）
// 使用分级池策略：5 个级别，更精确匹配不同场景，减少内存浪费
var (
	// 微批量：容量 128，适用于 10 个 item 以内（k=10）
	tinyLocationPool = sync.Pool{
		New: func() interface{} {
			return make([]interface{}, 0, 128)
		},
	}

	// 迷你批量：容量 256，适用于 25 个 item 以内（k=10）
	miniLocationPool = sync.Pool{
		New: func() interface{} {
			return make([]interface{}, 0, 256)
		},
	}

	// 小批量：容量 512，适用于 50 个 item 以内（k=10）
	smallLocationPool = sync.Pool{
		New: func() interface{} {
			return make([]interface{}, 0, 512)
		},
	}

	// 中批量：容量 4096，适用于 400 个 item 以内（k=10）
	mediumLocationPool = sync.Pool{
		New: func() interface{} {
			return make([]interface{}, 0, 4096)
		},
	}

	// 大批量：容量 16384，适用于 1600 个 item 以内（k=10）
	largeLocationPool = sync.Pool{
		New: func() interface{} {
			return make([]interface{}, 0, 16384)
		},
	}
)

// itemLocationsListPool 用于复用二维切片（跟踪需要释放的 location 切片）
var itemLocationsListPool = sync.Pool{
	New: func() interface{} {
		return make([][]interface{}, 0, 512)
	},
}

// init 预初始化对象池，避免冷启动时的频繁分配
func init() {
	// 预先填充 128 个切片到 locationPool
	for i := 0; i < 128; i++ {
		locationPool.Put(make([]interface{}, 0, 20))
	}

	// 预先填充微批量池（128 个，容量 128）- 最常用
	for i := 0; i < 128; i++ {
		tinyLocationPool.Put(make([]interface{}, 0, 128))
	}

	// 预先填充迷你批量池（96 个，容量 256）
	for i := 0; i < 96; i++ {
		miniLocationPool.Put(make([]interface{}, 0, 256))
	}

	// 预先填充小批量池（64 个，容量 512）
	for i := 0; i < 64; i++ {
		smallLocationPool.Put(make([]interface{}, 0, 512))
	}

	// 预先填充中批量池（32 个，容量 4096）
	for i := 0; i < 32; i++ {
		mediumLocationPool.Put(make([]interface{}, 0, 4096))
	}

	// 预先填充大批量池（16 个，容量 16384）
	for i := 0; i < 16; i++ {
		largeLocationPool.Put(make([]interface{}, 0, 16384))
	}

	// 预先填充 128 个二维切片到 itemLocationsListPool
	for i := 0; i < 128; i++ {
		itemLocationsListPool.Put(make([][]interface{}, 0, 512))
	}
}

// ErrRecordNotFound
var (
	// 定义的key已存在
	ErrItemAlreadyExists = errors.New("ERR item exists")
	ErrNotExists         = errors.New("not exists")
)

func getErrorRatio(p float64) float64 {
	if p > 0.01 {
		p = 0.001
	} else if p <= 0 {
		p = 0.001
	}

	return p
}

func getCap(cap uint) uint {
	if cap == 0 {
		cap = 2 << 16
	}

	return cap
}

// Location returns the ith hashed location using the four base hash values.
// This is the v9 location calculation method.
// h: array of 4 uint64 hash values
// i: index of the hash function (0 to k-1)
func Location(h [4]uint64, i uint) uint64 {
	ii := uint64(i)
	return h[ii%2] + ii*h[2+(((ii+(ii%2))%4)/2)]
}

// CalculateBloomFilterLocations calculates bloom filter location indices for the given key.
// cap: capacity (expected number of items)
// percent: false positive rate (e.g., 0.001 for 0.1%)
// key: the key string to calculate locations for
// Returns: slice of location indices (uint64 values)
func CalculateBloomFilterLocations(cap uint, percent float64, key string) []uint64 {
	// Normalize cap and percent
	normalizedCap := getCap(cap)
	normalizedPercent := getErrorRatio(percent)

	// Calculate m (bit array size) and k (number of hash functions)
	m := uint(math.Ceil(-1 * float64(normalizedCap) * math.Log(normalizedPercent) / math.Pow(math.Log(2), 2)))
	k := uint(math.Ceil(math.Log(2) * float64(m) / float64(normalizedCap)))

	// Calculate hash values using murmur3
	var d digest128 // murmur hashing
	hash1, hash2, hash3, hash4 := d.sum256([]byte(key))
	hashList := [4]uint64{hash1, hash2, hash3, hash4}

	// Calculate all location indices
	locations := make([]uint64, k)
	for i := uint(0); i < k; i++ {
		locations[i] = Location(hashList, i) % uint64(m)
	}

	return locations
}

// CalculateBloomFilterLocationsWithInterface calculates bloom filter location indices for the given key.
// capacity: capacity (expected number of items)
// percent: false positive rate (e.g., 0.001 for 0.1%)
// key: the key string to calculate locations for
// Returns: slice of location indices as []interface{} (for direct use with Redis Lua scripts)
//
// IMPORTANT: 返回的切片是从 sync.Pool 中获取的，调用者必须在使用完毕后调用 ReleaseLocationSlice 释放
func CalculateBloomFilterLocationsWithInterface(capacity uint, percent float64, key string) []interface{} {
	// Normalize cap and percent
	normalizedCap := getCap(capacity)
	normalizedPercent := getErrorRatio(percent)

	// Calculate m (bit array size) and k (number of hash functions)
	m := uint(math.Ceil(-1 * float64(normalizedCap) * math.Log(normalizedPercent) / math.Pow(math.Log(2), 2)))
	k := uint(math.Ceil(math.Log(2) * float64(m) / float64(normalizedCap)))

	// Calculate hash values using murmur3
	var d digest128 // murmur hashing
	hash1, hash2, hash3, hash4 := d.sum256([]byte(key))
	hashList := [4]uint64{hash1, hash2, hash3, hash4}

	// 从对象池获取切片，避免频繁分配
	locSlice := locationPool.Get().([]interface{})

	// 检查容量是否足够，不足则重新分配
	sliceCap := cap(locSlice)
	if sliceCap >= int(k) {
		locSlice = locSlice[:k] // 扩展到需要的长度
	} else {
		// 容量不足，重新分配
		locSlice = make([]interface{}, k)
	}

	// Calculate all location indices
	for i := uint(0); i < k; i++ {
		locSlice[i] = Location(hashList, i) % uint64(m)
	}

	return locSlice
}

// ReleaseLocationSlice 将使用完的 location 切片放回对象池
// 调用者在使用完 CalculateBloomFilterLocationsWithInterface 返回的切片后必须调用此函数
func ReleaseLocationSlice(locations []interface{}) {
	if locations != nil && cap(locations) <= 50 { // 只回收容量合理的切片，避免池中积累过大切片
		locationPool.Put(locations)
	}
}

// GetLargeLocationSlice 从对象池获取大容量切片，用于批量操作
// 使用分级池策略，根据需要的容量选择合适的池
// 返回的切片容量至少为 minCap，调用者使用完后必须调用 ReleaseLargeLocationSlice
func GetLargeLocationSlice(minCap int) []interface{} {
	var slice []interface{}

	// 根据所需容量选择合适的池（5 级容量）
	switch {
	case minCap <= 128:
		slice = tinyLocationPool.Get().([]interface{})
	case minCap <= 256:
		slice = miniLocationPool.Get().([]interface{})
	case minCap <= 512:
		slice = smallLocationPool.Get().([]interface{})
	case minCap <= 4096:
		slice = mediumLocationPool.Get().([]interface{})
	case minCap <= 16384:
		slice = largeLocationPool.Get().([]interface{})
	default:
		// 超大批量直接分配，不使用池（避免池中积累过大切片）
		return make([]interface{}, 0, minCap)
	}

	// 检查容量是否足够
	if cap(slice) < minCap {
		// 容量不足，分配新的更大切片
		slice = make([]interface{}, 0, minCap)
	} else {
		// 重置长度
		slice = slice[:0]
	}

	return slice
}

// ReleaseLargeLocationSlice 将大切片放回对应的对象池
func ReleaseLargeLocationSlice(slice []interface{}) {
	if slice == nil {
		return
	}

	sliceCap := cap(slice)
	slice = slice[:0] // 重置长度

	// 根据容量放回对应的池（只回收标准容量的切片，避免池污染）
	switch {
	case sliceCap <= 128 && sliceCap >= 64:
		tinyLocationPool.Put(slice)
	case sliceCap <= 256 && sliceCap >= 128:
		miniLocationPool.Put(slice)
	case sliceCap <= 512 && sliceCap >= 256:
		smallLocationPool.Put(slice)
	case sliceCap <= 4096 && sliceCap >= 2048:
		mediumLocationPool.Put(slice)
	case sliceCap <= 16384 && sliceCap >= 8192:
		largeLocationPool.Put(slice)
	// 其他容量的切片不放回池，让 GC 回收
	}
}

// GetItemLocationsListSlice 从对象池获取二维切片
func GetItemLocationsListSlice(minCap int) [][]interface{} {
	// 超大批量（>2000 items）不使用池，直接分配
	if minCap > 2048 {
		return make([][]interface{}, 0, minCap)
	}

	slice := itemLocationsListPool.Get().([][]interface{})

	if cap(slice) < minCap {
		slice = make([][]interface{}, 0, minCap)
	} else {
		slice = slice[:0]
	}

	return slice
}

// ReleaseItemLocationsListSlice 将二维切片放回对象池
func ReleaseItemLocationsListSlice(slice [][]interface{}) {
	if slice != nil && cap(slice) >= 256 && cap(slice) <= 2048 {
		itemLocationsListPool.Put(slice[:0])
	}
}

// RedisDelFunc 用于抽象 Redis 客户端的 Del 方法
// 接受 context 和 key，忽略返回值（因为错误会被忽略）
type RedisDelFunc func(ctx context.Context, key string)

// SlidingWindowBase 滑动窗口布隆过滤器的公共字段
type SlidingWindowBase struct {
	cap         uint
	p           float64
	m           uint
	k           uint
	windowSize  time.Duration // 单个窗口的时间长度
	windowCount int           // 窗口数量
	mu          sync.RWMutex  // 保护当前窗口索引
	currentIdx  int           // 当前窗口索引
	lastRotate  time.Time     // 上次窗口轮换时间
}

// initSlidingWindow 初始化滑动窗口布隆过滤器的参数
func initSlidingWindow(base *SlidingWindowBase, cap uint, p float64) error {
	base.cap = getCap(cap)
	base.p = getErrorRatio(p)

	base.m = uint(math.Ceil(-1 * float64(base.cap) * math.Log(base.p) / math.Pow(math.Log(2), 2)))
	base.k = uint(math.Ceil(math.Log(2) * float64(base.m) / float64(base.cap)))

	return nil
}

// getWindowKey 获取指定窗口的 redis key
func getWindowKey(baseKey string, windowIdx int) string {
	return fmt.Sprintf("{%s}:window:%d", baseKey, windowIdx)
}

// getCurrentWindow 获取当前窗口索引，如果需要则进行轮换
func getCurrentWindow(ctx context.Context, base *SlidingWindowBase, redisDel RedisDelFunc, baseKey string) (int, error) {
	base.mu.Lock()
	defer base.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(base.lastRotate)

	// 如果超过窗口时间，需要轮换到下一个窗口
	if elapsed >= base.windowSize {
		// 计算需要轮换的窗口数
		rotateCount := int(elapsed / base.windowSize)
		if rotateCount > base.windowCount {
			rotateCount = base.windowCount // 最多轮换所有窗口
		}

		// 计算新的当前窗口索引
		newCurrentIdx := (base.currentIdx + rotateCount) % base.windowCount

		// 清理过期窗口：删除超出活动范围的窗口
		// 活动窗口范围：从 newCurrentIdx 往前追溯 windowCount 个窗口
		// 由于只有 windowCount 个窗口，只有当 rotateCount >= windowCount 时才会删除窗口
		if rotateCount >= base.windowCount {
			// 如果轮转数超过等于窗口数，删除所有窗口（所有窗口都已过期）
			for i := 0; i < base.windowCount; i++ {
				oldKey := getWindowKey(baseKey, i)
				redisDel(ctx, oldKey)
			}
		}
		// 如果 rotateCount < windowCount，所有窗口都在活动范围内，不需要删除

		// 更新当前窗口索引
		base.currentIdx = newCurrentIdx
		base.lastRotate = now
	}

	return base.currentIdx, nil
}

// getActiveWindows 获取所有活动窗口的索引
func getActiveWindows(base *SlidingWindowBase) []int {
	base.mu.RLock()
	defer base.mu.RUnlock()

	windows := make([]int, 0, base.windowCount)
	// 从当前窗口往前追溯，最多回溯 windowCount 个窗口
	for i := 0; i < base.windowCount; i++ {
		idx := (base.currentIdx - i + base.windowCount) % base.windowCount
		windows = append(windows, idx)
	}

	return windows
}
