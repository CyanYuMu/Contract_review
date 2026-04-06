package driver

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
)

// GoBloomSliding 滑动布隆过滤器（Go本地实现）
// 使用多个时间窗口来追踪元素，自动清理过期数据
type GoBloomSliding struct {
	// 配置信息
	cap         uint
	p           float64
	m           uint
	k           uint
	windowSize  time.Duration // 单个窗口的时间长度
	windowCount int           // 窗口数量

	// 存储每个 key 的数据
	keys map[string]*GoBloomSlidingKeyData
	mu   sync.RWMutex // 保护 keys map

	// 清理相关
	stopChan        chan struct{} // 停止清理 goroutine 的信号
	cleanupInterval time.Duration // 清理间隔
	cleanupTicker   *time.Ticker  // 清理定时器
}

// GoBloomSlidingKeyData 每个 key 的滑动窗口数据
type GoBloomSlidingKeyData struct {
	SlidingWindowBase
	// 每个窗口的布隆过滤器实例
	windows []*bloom.BloomFilter
	// 窗口创建时间，用于 TTL 管理
	windowCreatedAt []time.Time
	// 窗口过期时间，用于 TTL 管理
	windowTTL []time.Duration
	// 最后访问时间，用于清理过期 key
	lastAccess time.Time
}

// GoBloomSlidingConf 滑动布隆过滤器配置
type GoBloomSlidingConf struct {
	// 容量
	Cap uint
	// 误判率, 如: 0.00000001
	P float64
	// 单个窗口的时间长度，例如: 1 * time.Hour
	WindowSize time.Duration
	// 窗口数量，默认5个窗口
	WindowCount int
	// 清理间隔，默认 1 分钟
	CleanupInterval time.Duration
}

// NewGoBloomSliding 创建新的滑动布隆过滤器
func NewGoBloomSliding(conf *GoBloomSlidingConf) (*GoBloomSliding, error) {
	if conf.WindowSize <= 0 {
		return nil, errors.New("window size must be greater than 0")
	}

	windowCount := conf.WindowCount
	if windowCount <= 0 {
		windowCount = 5 // 默认5个窗口
	}

	cleanupInterval := conf.CleanupInterval
	if cleanupInterval <= 0 {
		cleanupInterval = time.Minute // 默认1分钟清理一次
	}

	b := &GoBloomSliding{
		windowSize:      conf.WindowSize,
		windowCount:     windowCount,
		keys:            make(map[string]*GoBloomSlidingKeyData),
		stopChan:        make(chan struct{}),
		cleanupInterval: cleanupInterval,
	}

	err := initSlidingWindow(&SlidingWindowBase{
		cap:         conf.Cap,
		p:           conf.P,
		windowSize:  conf.WindowSize,
		windowCount: windowCount,
	}, conf.Cap, conf.P)
	if err != nil {
		return nil, err
	}

	// 保存计算后的参数
	b.cap = conf.Cap
	b.p = conf.P
	normalizedCap := getCap(conf.Cap)
	normalizedPercent := getErrorRatio(conf.P)
	b.m = uint(math.Ceil(-1 * float64(normalizedCap) * math.Log(normalizedPercent) / math.Pow(math.Log(2), 2)))
	b.k = uint(math.Ceil(math.Log(2) * float64(b.m) / float64(normalizedCap)))

	// 启动清理 goroutine
	b.startCleanup()

	return b, nil
}

// getOrCreateKeyData 获取或创建指定 key 的数据
func (b *GoBloomSliding) getOrCreateKeyData(key string) *GoBloomSlidingKeyData {
	b.mu.Lock()
	defer b.mu.Unlock()

	keyData, exists := b.keys[key]
	if !exists {
		// 创建新的 key 数据
		keyData = &GoBloomSlidingKeyData{
			SlidingWindowBase: SlidingWindowBase{
				cap:         b.cap,
				p:           b.p,
				m:           b.m,
				k:           b.k,
				windowSize:  b.windowSize,
				windowCount: b.windowCount,
				currentIdx:  0,
				lastRotate:  time.Now(),
			},
			windows:         make([]*bloom.BloomFilter, b.windowCount),
			windowCreatedAt: make([]time.Time, b.windowCount),
			windowTTL:       make([]time.Duration, b.windowCount),
			lastAccess:      time.Now(),
		}
		b.keys[key] = keyData
	}

	return keyData
}

// getKeyData 获取指定 key 的数据
func (b *GoBloomSliding) getKeyData(key string) (*GoBloomSlidingKeyData, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	keyData, exists := b.keys[key]
	return keyData, exists
}

// updateCurrentWindow 更新当前窗口，如果需要则进行轮换
// 注意：调用此方法前需要确保已持有 keyData.mu 锁
func (b *GoBloomSliding) updateCurrentWindow(ctx context.Context, keyData *GoBloomSlidingKeyData, key string) error {
	now := time.Now()
	elapsed := now.Sub(keyData.lastRotate)

	// 如果超过窗口时间，需要轮换到下一个窗口
	if elapsed >= keyData.windowSize {
		// 计算需要轮换的窗口数
		rotateCount := int(elapsed / keyData.windowSize)
		if rotateCount > keyData.windowCount {
			rotateCount = keyData.windowCount // 最多轮换所有窗口
		}

		// 清理过期窗口（从当前窗口开始，轮换 rotateCount 个窗口）
		for i := 0; i < rotateCount; i++ {
			oldIdx := (keyData.currentIdx + i) % keyData.windowCount
			// 清空过期窗口的布隆过滤器
			keyData.windows[oldIdx] = nil
			keyData.windowCreatedAt[oldIdx] = time.Time{}
			keyData.windowTTL[oldIdx] = 0
		}

		// 更新当前窗口索引
		keyData.currentIdx = (keyData.currentIdx + rotateCount) % keyData.windowCount
		keyData.lastRotate = now
	}

	return nil
}

// getCurrentWindow 获取当前窗口的布隆过滤器实例
func (b *GoBloomSliding) getCurrentWindow(ctx context.Context, keyData *GoBloomSlidingKeyData, key string) (*bloom.BloomFilter, error) {
	keyData.mu.Lock()
	defer keyData.mu.Unlock()

	// 更新窗口
	now := time.Now()
	elapsed := now.Sub(keyData.lastRotate)

	// 如果超过窗口时间，需要轮换到下一个窗口
	if elapsed >= keyData.windowSize {
		// 计算需要轮换的窗口数
		rotateCount := int(elapsed / keyData.windowSize)
		if rotateCount > keyData.windowCount {
			rotateCount = keyData.windowCount // 最多轮换所有窗口
		}

		// 清理过期窗口（从当前窗口开始，轮换 rotateCount 个窗口）
		for i := 0; i < rotateCount; i++ {
			oldIdx := (keyData.currentIdx + i) % keyData.windowCount
			// 清空过期窗口的布隆过滤器
			keyData.windows[oldIdx] = nil
			keyData.windowCreatedAt[oldIdx] = time.Time{}
			keyData.windowTTL[oldIdx] = 0
		}

		// 更新当前窗口索引
		keyData.currentIdx = (keyData.currentIdx + rotateCount) % keyData.windowCount
		keyData.lastRotate = now
	}

	// 如果当前窗口不存在，创建一个新的
	if keyData.windows[keyData.currentIdx] == nil {
		normalizedCap := getCap(keyData.cap)
		normalizedPercent := getErrorRatio(keyData.p)
		keyData.windows[keyData.currentIdx] = bloom.NewWithEstimates(normalizedCap, normalizedPercent)
		keyData.windowCreatedAt[keyData.currentIdx] = time.Now()
	}

	return keyData.windows[keyData.currentIdx], nil
}

// Exists 检查元素是否存在于任何活动窗口中
func (b *GoBloomSliding) Exists(ctx context.Context, key string, item string) (exists bool, err error) {
	keyData := b.getOrCreateKeyData(key)

	keyData.mu.Lock()
	// 更新最后访问时间
	keyData.lastAccess = time.Now()
	// 更新窗口
	_ = b.updateCurrentWindow(ctx, keyData, key)
	// 获取活动窗口索引
	activeIndices := make([]int, 0, keyData.windowCount)
	for i := 0; i < keyData.windowCount; i++ {
		idx := (keyData.currentIdx - i + keyData.windowCount) % keyData.windowCount
		if keyData.windows[idx] != nil {
			// 检查窗口是否过期（如果有 TTL）
			if keyData.windowTTL[idx] > 0 {
				elapsed := time.Since(keyData.windowCreatedAt[idx])
				if elapsed >= keyData.windowTTL[idx] {
					continue // 窗口已过期，跳过
				}
			}
			activeIndices = append(activeIndices, idx)
		}
	}
	windows := make([]*bloom.BloomFilter, len(activeIndices))
	for i, idx := range activeIndices {
		windows[i] = keyData.windows[idx]
	}
	keyData.mu.Unlock()

	// 检查所有活动窗口（不需要持有锁）
	for _, window := range windows {
		if window.TestString(item) {
			return true, nil
		}
	}

	return false, nil
}

// MExists 批量检查元素是否存在
func (b *GoBloomSliding) MExists(ctx context.Context, key string, items []string) (statusList []bool, err error) {
	statusList = make([]bool, 0, len(items))
	for _, item := range items {
		exists, err := b.Exists(ctx, key, item)
		if err != nil {
			return nil, err
		}
		statusList = append(statusList, exists)
	}

	return statusList, nil
}

// Add 添加元素到当前窗口
func (b *GoBloomSliding) Add(ctx context.Context, key string, item string, ttl time.Duration) (ok bool, err error) {
	keyData := b.getOrCreateKeyData(key)

	keyData.mu.Lock()
	// 更新最后访问时间
	keyData.lastAccess = time.Now()
	// 更新窗口
	now := time.Now()
	elapsed := now.Sub(keyData.lastRotate)

	// 如果超过窗口时间，需要轮换到下一个窗口
	if elapsed >= keyData.windowSize {
		rotateCount := int(elapsed / keyData.windowSize)
		if rotateCount > keyData.windowCount {
			rotateCount = keyData.windowCount
		}
		// 计算新的当前窗口索引
		newCurrentIdx := (keyData.currentIdx + rotateCount) % keyData.windowCount

		// 只有当轮换数量超过或等于窗口总数时，才需要清空所有窗口
		// 否则，旧窗口的数据仍然在活动范围内，应该保留
		if rotateCount >= keyData.windowCount {
			// 清空所有窗口（所有窗口都已过期）
			for i := 0; i < keyData.windowCount; i++ {
				keyData.windows[i] = nil
				keyData.windowCreatedAt[i] = time.Time{}
				keyData.windowTTL[i] = 0
			}
		}
		// 如果 rotateCount < windowCount，不需要清空任何窗口
		// 因为所有窗口都在活动范围内

		keyData.currentIdx = newCurrentIdx
		keyData.lastRotate = now
	}

	// 如果当前窗口不存在，创建一个新的
	if keyData.windows[keyData.currentIdx] == nil {
		normalizedCap := getCap(keyData.cap)
		normalizedPercent := getErrorRatio(keyData.p)
		keyData.windows[keyData.currentIdx] = bloom.NewWithEstimates(normalizedCap, normalizedPercent)
		keyData.windowCreatedAt[keyData.currentIdx] = time.Now()
	}

	currentWindow := keyData.windows[keyData.currentIdx]
	currentIdx := keyData.currentIdx

	// 检查元素是否已存在，如果不存在则添加（原子操作）
	if currentWindow.TestOrAddString(item) {
		keyData.mu.Unlock()
		return false, nil // 元素已存在
	}

	// 设置窗口 TTL（如果提供了）
	if ttl > 0 {
		windowTTL := b.windowSize * time.Duration(b.windowCount)
		if ttl < windowTTL {
			windowTTL = ttl
		}
		keyData.windowTTL[currentIdx] = windowTTL
	}

	keyData.mu.Unlock()
	return true, nil
}

// MAdd 批量添加元素
func (b *GoBloomSliding) MAdd(ctx context.Context, key string, items []string, ttl time.Duration) (statusList []bool, err error) {
	statusList = make([]bool, 0, len(items))
	for _, item := range items {
		ok, err := b.Add(ctx, key, item, ttl)
		if err != nil {
			return nil, err
		}
		statusList = append(statusList, ok)
	}

	return statusList, nil
}

// Insert 插入元素到当前窗口（会延长窗口过期时间）
func (b *GoBloomSliding) Insert(ctx context.Context, key string, item string, ttl time.Duration) (ok bool, err error) {
	keyData := b.getOrCreateKeyData(key)

	keyData.mu.Lock()
	// 更新最后访问时间
	keyData.lastAccess = time.Now()
	// 更新窗口
	now := time.Now()
	elapsed := now.Sub(keyData.lastRotate)

	// 如果超过窗口时间，需要轮换到下一个窗口
	if elapsed >= keyData.windowSize {
		rotateCount := int(elapsed / keyData.windowSize)
		if rotateCount > keyData.windowCount {
			rotateCount = keyData.windowCount
		}
		// 计算新的当前窗口索引
		newCurrentIdx := (keyData.currentIdx + rotateCount) % keyData.windowCount

		// 只有当轮换数量超过或等于窗口总数时，才需要清空所有窗口
		// 否则，旧窗口的数据仍然在活动范围内，应该保留
		if rotateCount >= keyData.windowCount {
			// 清空所有窗口（所有窗口都已过期）
			for i := 0; i < keyData.windowCount; i++ {
				keyData.windows[i] = nil
				keyData.windowCreatedAt[i] = time.Time{}
				keyData.windowTTL[i] = 0
			}
		}
		// 如果 rotateCount < windowCount，不需要清空任何窗口
		// 因为所有窗口都在活动范围内

		keyData.currentIdx = newCurrentIdx
		keyData.lastRotate = now
	}

	// 如果当前窗口不存在，创建一个新的
	if keyData.windows[keyData.currentIdx] == nil {
		normalizedCap := getCap(keyData.cap)
		normalizedPercent := getErrorRatio(keyData.p)
		keyData.windows[keyData.currentIdx] = bloom.NewWithEstimates(normalizedCap, normalizedPercent)
		keyData.windowCreatedAt[keyData.currentIdx] = time.Now()
	}

	currentWindow := keyData.windows[keyData.currentIdx]
	currentIdx := keyData.currentIdx

	// 检查元素是否已存在，如果不存在则添加（原子操作）
	exists := currentWindow.TestOrAddString(item)

	// 设置窗口 TTL（如果提供了）
	if ttl > 0 {
		windowTTL := b.windowSize * time.Duration(b.windowCount)
		if ttl < windowTTL {
			windowTTL = ttl
		}
		keyData.windowTTL[currentIdx] = windowTTL
		keyData.windowCreatedAt[currentIdx] = time.Now() // 重置创建时间，延长过期时间
	}

	keyData.mu.Unlock()
	return !exists, nil
}

// MInsert 批量插入元素
func (b *GoBloomSliding) MInsert(ctx context.Context, key string, items []string, ttl time.Duration) (statusList []bool, err error) {
	statusList = make([]bool, 0, len(items))
	for _, item := range items {
		ok, err := b.Insert(ctx, key, item, ttl)
		if err != nil {
			return nil, err
		}
		statusList = append(statusList, ok)
	}

	return statusList, nil
}

// Info 返回布隆过滤器的信息
func (b *GoBloomSliding) Info(ctx context.Context, key string) (info map[string]interface{}, err error) {
	keyData, exists := b.getKeyData(key)
	if !exists {
		return map[string]interface{}{
			"cap":         b.cap,
			"k":           b.k,
			"m":           b.m,
			"windowSize":  b.windowSize.String(),
			"windowCount": b.windowCount,
			"currentIdx":  0,
		}, nil
	}

	keyData.mu.Lock()
	// 更新窗口
	now := time.Now()
	elapsed := now.Sub(keyData.lastRotate)

	if elapsed >= keyData.windowSize {
		rotateCount := int(elapsed / keyData.windowSize)
		if rotateCount > keyData.windowCount {
			rotateCount = keyData.windowCount
		}
		for i := 0; i < rotateCount; i++ {
			oldIdx := (keyData.currentIdx + i) % keyData.windowCount
			keyData.windows[oldIdx] = nil
			keyData.windowCreatedAt[oldIdx] = time.Time{}
			keyData.windowTTL[oldIdx] = 0
		}
		keyData.currentIdx = (keyData.currentIdx + rotateCount) % keyData.windowCount
		keyData.lastRotate = now
	}
	keyData.lastAccess = now // 更新最后访问时间
	currentIdx := keyData.currentIdx
	keyData.mu.Unlock()

	return map[string]interface{}{
		"cap":         keyData.cap,
		"k":           keyData.k,
		"m":           keyData.m,
		"windowSize":  keyData.windowSize.String(),
		"windowCount": keyData.windowCount,
		"currentIdx":  currentIdx,
	}, nil
}

// startCleanup 启动清理 goroutine
func (b *GoBloomSliding) startCleanup() {
	b.cleanupTicker = time.NewTicker(b.cleanupInterval)
	ticker := b.cleanupTicker // 保存本地引用，避免并发访问问题
	go func() {
		for {
			select {
			case <-ticker.C:
				b.cleanup()
			case <-b.stopChan:
				return
			}
		}
	}()
}

// cleanup 清理过期的 key 数据
func (b *GoBloomSliding) cleanup() {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	// key 的过期时间：窗口大小 * 窗口数量（所有窗口的总时长）
	keyExpireDuration := b.windowSize * time.Duration(b.windowCount)

	// 收集需要删除的 key
	keysToDelete := make([]string, 0)
	for key, keyData := range b.keys {
		keyData.mu.RLock()
		elapsed := now.Sub(keyData.lastAccess)
		keyData.mu.RUnlock()

		// 如果 key 超过所有窗口总时长未访问，则删除
		if elapsed >= keyExpireDuration {
			keysToDelete = append(keysToDelete, key)
		}
	}

	// 删除过期的 key
	for _, key := range keysToDelete {
		delete(b.keys, key)
	}
}

// Stop 停止清理 goroutine
func (b *GoBloomSliding) Stop() {
	if b.cleanupTicker != nil {
		b.cleanupTicker.Stop()
		b.cleanupTicker = nil
	}
	select {
	case <-b.stopChan:
		// 已经关闭了
	default:
		close(b.stopChan)
	}
}

// Clear 清空指定key的所有窗口数据
func (b *GoBloomSliding) Clear(ctx context.Context, key string) (err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 删除指定key的所有窗口数据
	delete(b.keys, key)
	return nil
}
