package driver

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/su_json"
	cache "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/cache/v3"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/syncer"
)

// SyncEvent 同步事件，用于节点间同步
type SyncEvent struct {
	Key       string   `json:"k"` // bloom key
	WindowIdx int      `json:"w"` // 窗口索引
	Positions []uint64 `json:"p"` // 位置列表
}

// SyncHook 同步钩子函数类型
// 当有新数据写入时会调用此函数，用于实现自定义的节点间同步逻辑
type SyncHook func(ctx context.Context, event SyncEvent)

// BitsetL2Conf L2 滑动布隆过滤器配置
type BitsetL2Conf struct {
	// Redis 客户端实例（支持 v8/v9）
	Redis cache.Rds
	// 容量
	Cap uint
	// 误判率, 如: 0.001
	P float64
	// 单个窗口的时间长度，例如: 1 * time.Hour
	// 普通模式下此参数作为数据的 TTL 基准
	WindowSize time.Duration
	// 窗口数量:
	//   - WindowCount <= 1: 普通布隆过滤器模式（单窗口，不滑动）
	//   - WindowCount > 1: 滑动窗口布隆过滤器模式
	WindowCount int
	// Buffer flush 间隔，默认 5 秒
	FlushInterval time.Duration
	// Buffer 最大条数，达到后立即 flush，默认 1000
	FlushMaxSize int
	// 同步钩子，可选。当有新数据写入时会调用此函数
	OnSync SyncHook
	// ? 当指定了同步器, 针对分布式场景才算完整
	Syncer *syncer.Syncer
}

// BitsetL2 L2 滑动布隆过滤器
// 结合本地布隆 + Redis 持久化 + 自定义同步 hook
// 支持两种模式：
//   - 普通模式（windowCount <= 1）：单窗口，不滑动
//   - 滑动模式（windowCount > 1）：多窗口滑动
type BitsetL2 struct {
	// 布隆参数
	cap uint
	p   float64
	m   uint // 位数组大小
	k   uint // 哈希函数数量

	// 滑动窗口参数
	windowSize  time.Duration
	windowCount int

	// 是否为滑动模式（windowCount > 1）
	slidingMode bool

	// 本地 bitset 存储: key -> *LocalKeyData
	localData map[string]*LocalKeyData
	localMu   sync.RWMutex

	// Redis 客户端（支持 v8/v9）
	rds cache.Rds

	// 同步钩子（自定义同步逻辑）
	onSync SyncHook

	// 分布式同步器（基于 Redis Pub/Sub）
	syncer *syncer.Syncer

	// 写入 Buffer
	buffer       *WriteBuffer
	flushTicker  *time.Ticker
	flushMaxSize int

	// 停止信号
	stopChan chan struct{}
	stopped  bool
	stopMu   sync.Mutex
}

// LocalKeyData 每个 key 的本地数据
type LocalKeyData struct {
	mu sync.RWMutex

	// 滑动窗口基础信息
	currentIdx int       // 当前窗口索引
	lastRotate time.Time // 上次窗口轮换时间

	// 每个窗口的 bitset: windowIdx -> []byte
	windows [][]byte
}

// WriteBuffer 写入缓冲区
type WriteBuffer struct {
	mu sync.Mutex

	// key -> windowIdx -> []positions
	data map[string]map[int][]uint64

	// 位数组大小，用于边界检查
	m uint

	// 当前 buffer 中的总 position 数量（避免每次遍历统计）
	totalSize int
}

// NewBitsetL2 创建 L2 滑动布隆过滤器
// 根据 WindowCount 自动选择模式：
//   - WindowCount <= 1: 普通布隆过滤器模式
//   - WindowCount > 1: 滑动窗口布隆过滤器模式
func NewBitsetL2(conf *BitsetL2Conf) (*BitsetL2, error) {
	if conf.Redis == nil {
		return nil, errors.New("redis instance is nil")
	}
	if conf.WindowSize <= 0 {
		return nil, errors.New("window size must be greater than 0")
	}

	// 根据 WindowCount 决定模式
	windowCount := conf.WindowCount
	slidingMode := windowCount > 1

	// 普通模式下，强制设置 windowCount = 1
	if !slidingMode {
		windowCount = 1
	}

	flushInterval := conf.FlushInterval
	if flushInterval <= 0 {
		flushInterval = 5 * time.Second
	}

	flushMaxSize := conf.FlushMaxSize
	if flushMaxSize <= 0 {
		flushMaxSize = 1000
	}

	// 初始化布隆参数
	normalizedCap := getCap(conf.Cap)
	normalizedP := getErrorRatio(conf.P)
	m, k := calculateBloomParams(normalizedCap, normalizedP)

	b := &BitsetL2{
		cap:          normalizedCap,
		p:            normalizedP,
		m:            m,
		k:            k,
		windowSize:   conf.WindowSize,
		windowCount:  windowCount,
		slidingMode:  slidingMode,
		localData:    make(map[string]*LocalKeyData),
		rds:          conf.Redis,
		onSync:       conf.OnSync,
		syncer:       conf.Syncer,
		flushMaxSize: flushMaxSize,
		buffer: &WriteBuffer{
			data: make(map[string]map[int][]uint64),
			m:    m,
		},
		stopChan: make(chan struct{}),
	}

	// 启动后台任务
	b.startBackgroundTasks(flushInterval)

	// 如果配置了 Syncer，启动分布式同步订阅
	if b.syncer != nil {
		go b.startSyncSubscriber()
	}

	return b, nil
}

// startBackgroundTasks 启动后台任务
func (b *BitsetL2) startBackgroundTasks(flushInterval time.Duration) {
	b.flushTicker = time.NewTicker(flushInterval)
	go b.flushLoop()
}

// flushLoop flush 循环
func (b *BitsetL2) flushLoop() {
	for {
		select {
		case <-b.flushTicker.C:
			b.flushBuffer(context.Background())
		case <-b.stopChan:
			return
		}
	}
}

// startSyncSubscriber 启动分布式同步订阅
func (b *BitsetL2) startSyncSubscriber() {
	ctx := context.Background()
	err := b.syncer.SubscribeBatch(ctx, func(ctx context.Context, data []string) error {
		events := make([]SyncEvent, 0, len(data))
		for _, item := range data {
			var event SyncEvent
			if err := su_json.Unmarshal([]byte(item), &event); err != nil {
				su_logger.Error(ctx, err, "bloom l2 sync unmarshal failed", su_logger.E().String("data", item))
				return err
			}
			events = append(events, event)
		}
		// 批量应用同步事件到本地
		b.ApplySyncBatch(events)
		return nil
	})
	if err != nil {
		su_logger.Error(ctx, err, "bloom l2 sync subscriber failed")
	}
}

// publishSync 发布同步事件到其他节点
func (b *BitsetL2) publishSync(ctx context.Context, event SyncEvent) {
	if b.syncer == nil {
		return
	}

	data, err := su_json.Marshal(event)
	if err != nil {
		su_logger.Error(ctx, err, "bloom l2 sync marshal failed")
		return
	}

	if err := b.syncer.Publish(ctx, string(data)); err != nil {
		su_logger.Error(ctx, err, "bloom l2 sync publish failed")
	}
}

// ApplySyncBatch 批量应用同步事件到本地（减少锁竞争）
func (b *BitsetL2) ApplySyncBatch(events []SyncEvent) {
	if len(events) == 0 {
		return
	}

	// 按 key 分组，减少锁的获取次数
	keyEvents := make(map[string][]SyncEvent)
	for _, event := range events {
		if len(event.Positions) == 0 {
			continue
		}
		keyEvents[event.Key] = append(keyEvents[event.Key], event)
	}

	// 批量处理每个 key 的事件
	for key, evts := range keyEvents {
		keyData := b.getOrCreateKeyData(key)

		keyData.mu.Lock()
		for _, event := range evts {
			// 根据模式确定窗口索引
			windowIdx := event.WindowIdx
			if !b.slidingMode {
				// 普通模式：强制使用窗口 0
				windowIdx = 0
			} else {
				// 滑动模式：边界检查
				if windowIdx < 0 || windowIdx >= b.windowCount {
					continue
				}
			}

			// 确保窗口存在
			b.ensureWindowExists(keyData, windowIdx)

			// 设置位
			for _, pos := range event.Positions {
				if pos >= uint64(b.m) {
					continue
				}
				setBitInBytes(keyData.windows[windowIdx], pos)
			}
		}
		keyData.mu.Unlock()
	}
}

// ApplySync 应用同步事件到本地（供外部调用，用于接收其他节点的同步消息）
func (b *BitsetL2) ApplySync(event SyncEvent) {
	if len(event.Positions) == 0 {
		return
	}

	keyData := b.getOrCreateKeyData(event.Key)

	keyData.mu.Lock()
	defer keyData.mu.Unlock()

	// 根据模式确定窗口索引
	windowIdx := event.WindowIdx
	if !b.slidingMode {
		// 普通模式：强制使用窗口 0
		windowIdx = 0
	} else {
		// 滑动模式：边界检查
		if windowIdx < 0 || windowIdx >= b.windowCount {
			return
		}
	}

	// 确保窗口存在
	b.ensureWindowExists(keyData, windowIdx)

	// 设置位
	for _, pos := range event.Positions {
		if pos >= uint64(b.m) {
			continue
		}
		setBitInBytes(keyData.windows[windowIdx], pos)
	}
}

// getOrCreateKeyData 获取或创建 key 的本地数据
func (b *BitsetL2) getOrCreateKeyData(key string) *LocalKeyData {
	b.localMu.RLock()
	keyData, exists := b.localData[key]
	b.localMu.RUnlock()

	if exists {
		return keyData
	}

	b.localMu.Lock()
	defer b.localMu.Unlock()

	// 双重检查
	if keyData, exists = b.localData[key]; exists {
		return keyData
	}

	keyData = &LocalKeyData{
		currentIdx: 0,
		lastRotate: time.Now(),
		windows:    make([][]byte, b.windowCount),
	}
	b.localData[key] = keyData

	return keyData
}

// ensureWindowExists 确保窗口 bitset 存在
func (b *BitsetL2) ensureWindowExists(keyData *LocalKeyData, windowIdx int) {
	if keyData.windows[windowIdx] == nil {
		byteSize := (b.m + 7) / 8
		keyData.windows[windowIdx] = make([]byte, byteSize)
	}
}

// getLocations 计算元素的位置索引
func (b *BitsetL2) getLocations(item string) []uint64 {
	return CalculateBloomFilterLocations(b.cap, b.p, item)
}

// updateCurrentWindow 更新当前窗口，处理窗口轮换
func (b *BitsetL2) updateCurrentWindow(keyData *LocalKeyData) int {
	now := time.Now()
	elapsed := now.Sub(keyData.lastRotate)

	if elapsed >= b.windowSize {
		rotateCount := int(elapsed / b.windowSize)
		if rotateCount > b.windowCount {
			rotateCount = b.windowCount
		}

		// 清理过期窗口
		if rotateCount >= b.windowCount {
			for i := 0; i < b.windowCount; i++ {
				keyData.windows[i] = nil
			}
		}

		keyData.currentIdx = (keyData.currentIdx + rotateCount) % b.windowCount
		keyData.lastRotate = now
	}

	return keyData.currentIdx
}

// getActiveWindows 获取所有活动窗口的索引
func (b *BitsetL2) getActiveWindows(keyData *LocalKeyData) []int {
	windows := make([]int, 0, b.windowCount)
	for i := 0; i < b.windowCount; i++ {
		idx := (keyData.currentIdx - i + b.windowCount) % b.windowCount
		windows = append(windows, idx)
	}
	return windows
}

// Exists 检查元素是否存在
func (b *BitsetL2) Exists(ctx context.Context, key string, item string) (bool, error) {
	keyData := b.getOrCreateKeyData(key)
	locations := b.getLocations(item)

	if b.slidingMode {
		// 滑动窗口模式：单次加锁完成更新和查询
		keyData.mu.Lock()
		b.updateCurrentWindow(keyData)
		activeWindows := b.getActiveWindows(keyData)

		for _, windowIdx := range activeWindows {
			if keyData.windows[windowIdx] == nil {
				continue
			}

			allBitsSet := true
			for _, pos := range locations {
				if pos >= uint64(b.m) || !getBitInBytes(keyData.windows[windowIdx], pos) {
					allBitsSet = false
					break
				}
			}

			if allBitsSet {
				keyData.mu.Unlock()
				return true, nil
			}
		}

		keyData.mu.Unlock()
		return false, nil
	}

	// 普通模式：只检查单个窗口（index 0）
	keyData.mu.RLock()
	defer keyData.mu.RUnlock()

	if keyData.windows[0] == nil {
		return false, nil
	}

	for _, pos := range locations {
		if pos >= uint64(b.m) || !getBitInBytes(keyData.windows[0], pos) {
			return false, nil
		}
	}

	return true, nil
}

// MExists 批量检查元素是否存在（优化：单次加锁批量检查）
func (b *BitsetL2) MExists(ctx context.Context, key string, items []string) ([]bool, error) {
	if len(items) == 0 {
		return []bool{}, nil
	}

	keyData := b.getOrCreateKeyData(key)
	results := make([]bool, len(items))

	// 预计算所有 items 的 locations
	allLocations := make([][]uint64, len(items))
	for i, item := range items {
		allLocations[i] = b.getLocations(item)
	}

	if b.slidingMode {
		// 滑动窗口模式：更新窗口并获取活动窗口列表
		keyData.mu.Lock()
		b.updateCurrentWindow(keyData)
		activeWindows := b.getActiveWindows(keyData)
		keyData.mu.Unlock()

		keyData.mu.RLock()
		defer keyData.mu.RUnlock()

		// 批量检查每个 item
		for i, locations := range allLocations {
			for _, windowIdx := range activeWindows {
				if keyData.windows[windowIdx] == nil {
					continue
				}

				allBitsSet := true
				for _, pos := range locations {
					if pos >= uint64(b.m) || !getBitInBytes(keyData.windows[windowIdx], pos) {
						allBitsSet = false
						break
					}
				}

				if allBitsSet {
					results[i] = true
					break
				}
			}
		}
	} else {
		// 普通模式：只检查单个窗口
		keyData.mu.RLock()
		defer keyData.mu.RUnlock()

		if keyData.windows[0] != nil {
			for i, locations := range allLocations {
				allBitsSet := true
				for _, pos := range locations {
					if pos >= uint64(b.m) || !getBitInBytes(keyData.windows[0], pos) {
						allBitsSet = false
						break
					}
				}
				results[i] = allBitsSet
			}
		}
	}

	return results, nil
}

// Add 添加元素
func (b *BitsetL2) Add(ctx context.Context, key string, item string, ttl time.Duration) (bool, error) {
	keyData := b.getOrCreateKeyData(key)
	locations := b.getLocations(item)

	var currentIdx int

	if b.slidingMode {
		// 滑动窗口模式
		keyData.mu.Lock()
		currentIdx = b.updateCurrentWindow(keyData)
		b.ensureWindowExists(keyData, currentIdx)

		// 检查是否已存在（在当前窗口）
		allBitsSet := true
		for _, pos := range locations {
			if pos >= uint64(b.m) {
				continue
			}
			if !getBitInBytes(keyData.windows[currentIdx], pos) {
				allBitsSet = false
				break
			}
		}

		if allBitsSet {
			keyData.mu.Unlock()
			return false, nil // 已存在
		}

		// 设置位
		for _, pos := range locations {
			if pos >= uint64(b.m) {
				continue
			}
			setBitInBytes(keyData.windows[currentIdx], pos)
		}
		keyData.mu.Unlock()
	} else {
		// 普通模式：使用单个窗口（index 0）
		currentIdx = 0

		keyData.mu.Lock()
		b.ensureWindowExists(keyData, 0)

		// 检查是否已存在
		allBitsSet := true
		for _, pos := range locations {
			if pos >= uint64(b.m) {
				continue
			}
			if !getBitInBytes(keyData.windows[0], pos) {
				allBitsSet = false
				break
			}
		}

		if allBitsSet {
			keyData.mu.Unlock()
			return false, nil // 已存在
		}

		// 设置位
		for _, pos := range locations {
			if pos >= uint64(b.m) {
				continue
			}
			setBitInBytes(keyData.windows[0], pos)
		}
		keyData.mu.Unlock()
	}

	// 构建同步事件
	syncEvent := SyncEvent{
		Key:       key,
		WindowIdx: currentIdx,
		Positions: locations,
	}

	// 调用同步 hook（自定义同步逻辑）
	if b.onSync != nil {
		b.onSync(ctx, syncEvent)
	}

	// 通过 Syncer 发布同步事件到其他节点
	b.publishSync(ctx, syncEvent)

	// 加入写入 buffer
	b.addToBuffer(key, currentIdx, locations)

	return true, nil
}

// MAdd 批量添加元素（优化：单次加锁批量添加）
func (b *BitsetL2) MAdd(ctx context.Context, key string, items []string, ttl time.Duration) ([]bool, error) {
	if len(items) == 0 {
		return []bool{}, nil
	}

	keyData := b.getOrCreateKeyData(key)
	results := make([]bool, len(items))

	// 预计算所有 items 的 locations
	allLocations := make([][]uint64, len(items))
	for i, item := range items {
		allLocations[i] = b.getLocations(item)
	}

	var currentIdx int
	var addedPositions []uint64 // 收集所有新增的 positions

	keyData.mu.Lock()

	if b.slidingMode {
		currentIdx = b.updateCurrentWindow(keyData)
	} else {
		currentIdx = 0
	}
	b.ensureWindowExists(keyData, currentIdx)

	// 批量检查和添加
	for i, locations := range allLocations {
		// 检查是否已存在
		allBitsSet := true
		for _, pos := range locations {
			if pos >= uint64(b.m) {
				continue
			}
			if !getBitInBytes(keyData.windows[currentIdx], pos) {
				allBitsSet = false
				break
			}
		}

		if allBitsSet {
			results[i] = false // 已存在
			continue
		}

		// 设置位
		for _, pos := range locations {
			if pos >= uint64(b.m) {
				continue
			}
			setBitInBytes(keyData.windows[currentIdx], pos)
		}
		results[i] = true
		addedPositions = append(addedPositions, locations...)
	}

	keyData.mu.Unlock()

	// 如果有新增数据，处理同步和 buffer
	if len(addedPositions) > 0 {
		// 构建同步事件（合并所有新增的 positions）
		syncEvent := SyncEvent{
			Key:       key,
			WindowIdx: currentIdx,
			Positions: addedPositions,
		}

		// 调用同步 hook
		if b.onSync != nil {
			b.onSync(ctx, syncEvent)
		}

		// 发布同步事件
		b.publishSync(ctx, syncEvent)

		// 加入写入 buffer
		b.addToBuffer(key, currentIdx, addedPositions)
	}

	return results, nil
}

// Insert 插入元素（会延长过期时间）
func (b *BitsetL2) Insert(ctx context.Context, key string, item string, ttl time.Duration) (bool, error) {
	return b.Add(ctx, key, item, ttl)
}

// MInsert 批量插入元素
func (b *BitsetL2) MInsert(ctx context.Context, key string, items []string, ttl time.Duration) ([]bool, error) {
	return b.MAdd(ctx, key, items, ttl)
}

// Info 返回布隆过滤器信息
func (b *BitsetL2) Info(ctx context.Context, key string) (map[string]interface{}, error) {
	keyData := b.getOrCreateKeyData(key)

	keyData.mu.RLock()
	currentIdx := keyData.currentIdx
	keyData.mu.RUnlock()

	info := map[string]interface{}{
		"cap":         b.cap,
		"p":           b.p,
		"m":           b.m,
		"k":           b.k,
		"slidingMode": b.slidingMode,
		"windowCount": b.windowCount,
		"syncEnabled": b.syncer != nil,
	}

	if b.slidingMode {
		info["windowSize"] = b.windowSize.String()
		info["currentIdx"] = currentIdx
	}

	return info, nil
}

// Clear 清空指定 key 的数据
func (b *BitsetL2) Clear(ctx context.Context, key string) error {
	b.localMu.Lock()
	delete(b.localData, key)
	b.localMu.Unlock()

	if b.slidingMode {
		// 滑动模式：删除所有窗口
		keys := make([]string, 0, b.windowCount)
		for i := 0; i < b.windowCount; i++ {
			keys = append(keys, getWindowKey(key, i))
		}
		if len(keys) > 0 {
			b.rds.MDel(ctx, keys...)
		}
		return nil
	} else {
		// 普通模式：只删除单个窗口
		_, err := b.rds.Del(ctx, getWindowKey(key, 0))
		return err
	}
}

// addToBuffer 添加到写入 buffer
func (b *BitsetL2) addToBuffer(key string, windowIdx int, positions []uint64) {
	b.buffer.mu.Lock()

	// 普通模式下强制使用窗口 0
	if !b.slidingMode {
		windowIdx = 0
	}

	if b.buffer.data[key] == nil {
		b.buffer.data[key] = make(map[int][]uint64)
	}

	// 合并位置（去重），同时更新计数器
	existing := b.buffer.data[key][windowIdx]
	oldSize := len(existing)

	posSet := make(map[uint64]struct{}, oldSize+len(positions))
	for _, pos := range existing {
		posSet[pos] = struct{}{}
	}
	for _, pos := range positions {
		posSet[pos] = struct{}{}
	}

	merged := make([]uint64, 0, len(posSet))
	for pos := range posSet {
		merged = append(merged, pos)
	}
	b.buffer.data[key][windowIdx] = merged

	// 更新计数器：新增的数量 = 合并后数量 - 原有数量
	b.buffer.totalSize += len(merged) - oldSize

	// 检查是否需要立即 flush
	needFlush := b.buffer.totalSize >= b.flushMaxSize
	b.buffer.mu.Unlock()

	if needFlush {
		go b.flushBuffer(context.Background())
	}
}

// flushBuffer 将 buffer 数据 flush 到 Redis
func (b *BitsetL2) flushBuffer(ctx context.Context) {
	b.buffer.mu.Lock()
	if len(b.buffer.data) == 0 {
		b.buffer.mu.Unlock()
		return
	}

	data := b.buffer.data
	b.buffer.data = make(map[string]map[int][]uint64)
	b.buffer.totalSize = 0 // 重置计数器
	b.buffer.mu.Unlock()

	pipe := b.rds.Client().V9.Pipeline()
	ttlSeconds := int64(b.windowSize.Seconds())

	for key, windows := range data {
		for windowIdx, positions := range windows {
			// 边界检查
			if windowIdx < 0 || windowIdx >= b.windowCount {
				continue
			}

			// 过滤有效的 positions
			validPositions := make([]interface{}, 0, len(positions)+1)
			validPositions = append(validPositions, ttlSeconds) // ARGV[1] = TTL
			for _, pos := range positions {
				if pos < uint64(b.m) {
					validPositions = append(validPositions, pos)
				}
			}

			// 跳过没有有效位置的窗口
			if len(validPositions) <= 1 {
				continue
			}

			windowKey := getWindowKey(key, windowIdx)
			// 使用 Lua 脚本批量设置位并设置 TTL
			cache.SetBitsScriptV9.Run(ctx, pipe, []string{windowKey}, validPositions...)
		}
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		su_logger.Error(ctx, err, "bloom l2 flush buffer failed")
	}
}

// LoadFromRedis 从 Redis 加载指定 key 的数据到本地（使用 Pipeline 批量加载）
func (b *BitsetL2) LoadFromRedis(ctx context.Context, key string) error {
	keyData := b.getOrCreateKeyData(key)
	byteSize := (b.m + 7) / 8

	// 确定需要加载的窗口数量
	windowCount := 1
	if b.slidingMode {
		windowCount = b.windowCount
	}

	// 使用 Pipeline 批量获取所有窗口数据
	pipe := b.rds.Client().V9.Pipeline()
	cmds := make([]*redis.StringCmd, windowCount)
	for i := 0; i < windowCount; i++ {
		windowKey := getWindowKey(key, i)
		cmds[i] = pipe.Get(ctx, windowKey)
	}

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		// 检查是否所有命令都是 key 不存在的错误
		allNil := true
		for _, cmd := range cmds {
			if cmd.Err() != nil && cmd.Err() != redis.Nil {
				allNil = false
				break
			}
		}
		if !allNil {
			return fmt.Errorf("failed to load windows: %w", err)
		}
	}

	// 应用数据到本地
	keyData.mu.Lock()
	defer keyData.mu.Unlock()

	for windowIdx, cmd := range cmds {
		data, err := cmd.Bytes()
		if err == redis.Nil {
			continue
		}
		if err != nil {
			continue
		}

		if uint(len(data)) > byteSize {
			data = data[:byteSize]
		}

		if keyData.windows[windowIdx] == nil {
			keyData.windows[windowIdx] = make([]byte, byteSize)
			copy(keyData.windows[windowIdx], data)
		} else {
			for i := 0; i < len(data) && i < len(keyData.windows[windowIdx]); i++ {
				keyData.windows[windowIdx][i] |= data[i]
			}
		}
	}

	return nil
}

// Stop 停止后台任务
func (b *BitsetL2) Stop() {
	b.stopMu.Lock()
	defer b.stopMu.Unlock()

	if b.stopped {
		return
	}
	b.stopped = true

	if b.flushTicker != nil {
		b.flushTicker.Stop()
	}

	close(b.stopChan)

	// 最后一次 flush
	b.flushBuffer(context.Background())
}

// ==================== 辅助函数 ====================

// setBitInBytes 在字节数组中设置指定位置的位（大端序，与 Redis SETBIT 一致）
func setBitInBytes(data []byte, pos uint64) {
	byteIdx := pos / 8
	if byteIdx >= uint64(len(data)) {
		return
	}
	bitIdx := 7 - (pos % 8)
	data[byteIdx] |= 1 << bitIdx
}

// getBitInBytes 在字节数组中获取指定位置的位（大端序，与 Redis GETBIT 一致）
func getBitInBytes(data []byte, pos uint64) bool {
	byteIdx := pos / 8
	if byteIdx >= uint64(len(data)) {
		return false
	}
	bitIdx := 7 - (pos % 8)
	return (data[byteIdx] & (1 << bitIdx)) != 0
}

// calculateBloomParams 计算布隆过滤器参数
func calculateBloomParams(cap uint, p float64) (m uint, k uint) {
	base := &SlidingWindowBase{}
	_ = initSlidingWindow(base, cap, p)
	return base.m, base.k
}
