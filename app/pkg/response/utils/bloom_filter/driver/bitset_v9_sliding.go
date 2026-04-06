package driver

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

var luaSlidingExistsCheckScriptV9 = redis.NewScript(bloomFilterSlidingExistsCheckScript)
var luaSlidingMSExistsCheckScriptV9 = redis.NewScript(bloomFilterSlidingMSExistsCheckScript)

// BitsetV9Sliding 滑动布隆过滤器
// 使用多个时间窗口来追踪元素，自动清理过期数据
type BitsetV9Sliding struct {
	SlidingWindowBase
	redis redis.UniversalClient
}

// BitsetV9SlidingConf 滑动布隆过滤器配置
type BitsetV9SlidingConf struct {
	// redis cli 实例
	Redis redis.UniversalClient
	// 容量
	Cap uint
	// 误判率, 如: 0.00000001
	P float64
	// 单个窗口的时间长度，例如: 1 * time.Hour
	WindowSize time.Duration
	// 窗口数量，默认5个窗口
	WindowCount int
}

func NewBitsetV9Sliding(conf *BitsetV9SlidingConf) (*BitsetV9Sliding, error) {
	if conf.WindowSize <= 0 {
		return nil, errors.New("window size must be greater than 0")
	}

	windowCount := conf.WindowCount
	if windowCount <= 0 {
		windowCount = 5 // 默认5个窗口
	}

	b := &BitsetV9Sliding{
		SlidingWindowBase: SlidingWindowBase{
			windowSize:  conf.WindowSize,
			windowCount: windowCount,
			currentIdx:  0,
			lastRotate:  time.Now(),
		},
		redis: conf.Redis,
	}

	if b.redis == nil {
		return nil, errors.New("redis instance is nil")
	}

	err := initSlidingWindow(&b.SlidingWindowBase, conf.Cap, conf.P)
	return b, err
}

// getLocation 获取元素的位置索引，使用公共函数 CalculateBloomFilterLocationsWithInterface
func (b *BitsetV9Sliding) getLocation(item string) []interface{} {
	return CalculateBloomFilterLocationsWithInterface(b.cap, b.p, item)
}

// Exists 检查元素是否存在于任何活动窗口中
func (b *BitsetV9Sliding) Exists(ctx context.Context, key string, item string) (exists bool, err error) {
	// 先更新窗口
	redisDel := func(ctx context.Context, key string) {
		_ = b.redis.Del(ctx, key)
	}
	_, err = getCurrentWindow(ctx, &b.SlidingWindowBase, redisDel, key)
	if err != nil {
		return false, err
	}

	locations := b.getLocation(item)
	defer ReleaseLocationSlice(locations) // 使用完后释放回对象池

	activeWindows := getActiveWindows(&b.SlidingWindowBase)

	// 使用 hash tag，所有窗口都在同一个 Redis 分片上，可以使用 Lua 脚本批量检查
	windowKeys := make([]string, 0, len(activeWindows))
	for _, windowIdx := range activeWindows {
		windowKeys = append(windowKeys, getWindowKey(key, windowIdx))
	}

	// 使用 Lua 脚本一次性检查所有窗口
	rs, err := luaSlidingExistsCheckScriptV9.Run(ctx, b.redis, windowKeys, locations...).Result()
	if err != nil {
		return false, err
	}

	v, ok := rs.(int64)
	if !ok {
		return false, errors.New("unexpected response format")
	}

	return v == 1, nil
}

// MExists 批量检查元素是否存在
func (b *BitsetV9Sliding) MExists(ctx context.Context, key string, items []string) (statusList []bool, err error) {
	if len(items) == 0 {
		return []bool{}, nil
	}

	// 先更新窗口
	redisDel := func(ctx context.Context, key string) {
		_ = b.redis.Del(ctx, key)
	}
	_, err = getCurrentWindow(ctx, &b.SlidingWindowBase, redisDel, key)
	if err != nil {
		return nil, err
	}

	// 获取所有活动窗口
	activeWindows := getActiveWindows(&b.SlidingWindowBase)
	windowKeys := make([]string, 0, len(activeWindows))
	for _, windowIdx := range activeWindows {
		windowKeys = append(windowKeys, getWindowKey(key, windowIdx))
	}

	// 从对象池获取大切片，避免频繁内存分配
	requiredCap := len(items)*int(b.k) + 1
	locations := GetLargeLocationSlice(requiredCap)
	defer ReleaseLargeLocationSlice(locations)

	locations = append(locations, b.k) // chunkSize

	// 从对象池获取二维切片来跟踪需要释放的 location 切片
	itemLocationsList := GetItemLocationsListSlice(len(items))
	defer ReleaseItemLocationsListSlice(itemLocationsList)

	// 收集所有 item 的 locations
	for _, item := range items {
		itemLocations := b.getLocation(item)
		itemLocationsList = append(itemLocationsList, itemLocations)
		locations = append(locations, itemLocations...)
	}

	// 使用 defer 确保所有 location 切片被释放
	defer func() {
		for _, itemLocs := range itemLocationsList {
			ReleaseLocationSlice(itemLocs)
		}
	}()

	// 使用批量 Lua 脚本一次性检查所有元素和所有窗口
	rs, err := luaSlidingMSExistsCheckScriptV9.Run(ctx, b.redis, windowKeys, locations...).Result()
	if err != nil {
		return nil, err
	}

	resultItems, ok := rs.([]interface{})
	if !ok {
		return nil, errors.New("unexpected response format")
	}

	statusList = make([]bool, 0, len(items))
	for _, item := range resultItems {
		v, ok := item.(int64)
		if !ok {
			return nil, errors.New("unexpected item format in response")
		}
		statusList = append(statusList, v == 1)
	}

	return statusList, nil
}

// Add 添加元素到当前窗口
func (b *BitsetV9Sliding) Add(ctx context.Context, key string, item string, ttl time.Duration) (ok bool, err error) {
	redisDel := func(ctx context.Context, key string) {
		_ = b.redis.Del(ctx, key)
	}
	currentIdx, err := getCurrentWindow(ctx, &b.SlidingWindowBase, redisDel, key)
	if err != nil {
		return false, err
	}

	windowKey := getWindowKey(key, currentIdx)
	locations := make([]interface{}, 0, b.k+1)
	// 使用窗口大小作为 TTL，确保窗口过期后自动清理
	windowTTL := int64(b.windowSize.Seconds() * float64(b.windowCount))
	if ttl > 0 && ttl.Seconds() < float64(windowTTL) {
		windowTTL = int64(ttl.Seconds())
	}
	// 确保 TTL 至少为 5 秒，避免因网络延迟导致数据立即过期
	if windowTTL < 5 {
		windowTTL = 5
	}
	locations = append(locations, windowTTL)

	// 获取位置列表
	itemLocations := b.getLocation(item)
	defer ReleaseLocationSlice(itemLocations) // 使用完后释放回对象池
	locations = append(locations, itemLocations...)

	// 调试日志
	//fmt.Printf("DEBUG Add: windowKey=%s, windowTTL=%d, itemLocations=%v, locations=%v\n",
	//	windowKey, windowTTL, itemLocations, locations)

	rs, err := luaAddScriptV9.Run(ctx, b.redis, []string{windowKey}, locations...).Result()
	if err != nil {
		return false, err
	}

	v, ok := rs.(int64)
	if !ok {
		return false, errors.New("unexpected response format")
	}

	return v == 1, nil
}

// MAdd 批量添加元素
func (b *BitsetV9Sliding) MAdd(ctx context.Context, key string, items []string, ttl time.Duration) (statusList []bool, err error) {
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

// Info 返回布隆过滤器的信息
func (b *BitsetV9Sliding) Info(ctx context.Context, key string) (info map[string]interface{}, err error) {
	redisDel := func(ctx context.Context, key string) {
		_ = b.redis.Del(ctx, key)
	}
	_, err = getCurrentWindow(ctx, &b.SlidingWindowBase, redisDel, key)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"cap":         b.cap,
		"k":           b.k,
		"m":           b.m,
		"windowSize":  b.windowSize.String(),
		"windowCount": b.windowCount,
		"currentIdx":  b.currentIdx,
	}, nil
}

// Insert 插入元素到当前窗口（会延长窗口过期时间）
func (b *BitsetV9Sliding) Insert(ctx context.Context, key string, item string, ttl time.Duration) (ok bool, err error) {
	redisDel := func(ctx context.Context, key string) {
		_ = b.redis.Del(ctx, key)
	}
	currentIdx, err := getCurrentWindow(ctx, &b.SlidingWindowBase, redisDel, key)
	if err != nil {
		return false, err
	}

	windowKey := getWindowKey(key, currentIdx)
	locations := make([]interface{}, 0, b.k+1)
	// 使用窗口大小作为 TTL，确保窗口过期后自动清理
	windowTTL := int64(b.windowSize.Seconds() * float64(b.windowCount))
	if ttl > 0 && ttl.Seconds() < float64(windowTTL) {
		windowTTL = int64(ttl.Seconds())
	}
	// 确保 TTL 至少为 5 秒，避免因网络延迟导致数据立即过期
	if windowTTL < 5 {
		windowTTL = 5
	}
	locations = append(locations, windowTTL)

	itemLocations := b.getLocation(item)
	defer ReleaseLocationSlice(itemLocations) // 使用完后释放回对象池
	locations = append(locations, itemLocations...)

	rs, err := luaInsertScriptV9.Run(ctx, b.redis, []string{windowKey}, locations...).Result()
	if err != nil {
		return false, err
	}

	v, ok := rs.(int64)
	if !ok {
		return false, errors.New("unexpected response format")
	}

	return v == 1, nil
}

// MInsert 批量插入元素
func (b *BitsetV9Sliding) MInsert(ctx context.Context, key string, items []string, ttl time.Duration) (statusList []bool, err error) {
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

// Clear 清空指定key的所有窗口数据
func (b *BitsetV9Sliding) Clear(ctx context.Context, key string) (err error) {
	// 删除所有窗口的Redis key
	keys := make([]string, 0, b.windowCount)
	for i := 0; i < b.windowCount; i++ {
		keys = append(keys, getWindowKey(key, i))
	}

	if len(keys) > 0 {
		return b.redis.Del(ctx, keys...).Err()
	}

	return nil
}
