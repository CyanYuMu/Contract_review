package cache

import (
	"container/list"
	"context"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/cast"
	// "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
)

// 缓存条目结构
type cacheItem struct {
	value  interface{} // 存储任意类型
	expire int64       // 过期时间戳（秒）
}

// 热度检测器
type hotDetector struct {
	counter    uint32 // 短期访问计数器
	ewma       uint32 // 指数加权移动平均值，float32bits 存储
	lastAccess int64  // 最后访问时间（纳秒）
}

// GoHotSpot 核心结构
type GoHotSpot struct {
	sizeLimit     int           // 普通缓存容量限制
	hotThreshold  uint32        // 热点阈值
	checkInterval time.Duration // 热度检查间隔

	// 普通缓存（LRU实现）
	cacheMu   sync.Mutex
	cache     map[string]*list.Element
	cacheList *list.List

	// 热点存储（无锁）
	hotData sync.Map

	// 热度追踪
	detectors sync.Map // key -> *hotDetector

	// 控制通道
	closeChan chan struct{}
}

// NewGoHotSpot 创建缓存实例
func NewGoHotSpot(sizeLimit int, hotThreshold uint32, checkInterval time.Duration) *GoHotSpot {
	h := &GoHotSpot{
		sizeLimit:     sizeLimit,
		hotThreshold:  hotThreshold,
		checkInterval: checkInterval,
		cache:         make(map[string]*list.Element, sizeLimit>>1),
		cacheList:     list.New(),
		closeChan:     make(chan struct{}),
	}

	go h.startDecayLoop()
	return h
}

// 启动热度衰减循环
func (h *GoHotSpot) startDecayLoop() {
	ticker := time.NewTicker(h.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.checkHotDataDecay()
		case <-h.closeChan:
			return
		}
	}
}

// 检查热点数据是否需要降级
func (h *GoHotSpot) checkHotDataDecay() {
	now := time.Now().Unix()
	h.hotData.Range(func(key, value interface{}) bool {
		det, exists := h.detectors.Load(key)
		if !exists {
			return true
		}

		detector := det.(*hotDetector)
		lastAccess := atomic.LoadInt64(&detector.lastAccess)

		// 牛顿冷却定律计算衰减系数
		elapsed := float64(now-lastAccess) / 1e9
		decayFactor := math.Exp(-elapsed / 60) // 60秒半衰期

		// 原子读取当前EWMA
		currentEWMA := math.Float32frombits(
			atomic.LoadUint32(&detector.ewma),
		)

		// 计算当前热度
		currentHotness := currentEWMA * float32(decayFactor)

		// 降级条件
		if currentHotness < float32(h.hotThreshold)/2 {
			h.demoteFromHot(key.(string), value.(*cacheItem))
		}
		return true
	})
}

// 从热点降级
func (h *GoHotSpot) demoteFromHot(key string, item *cacheItem) {
	h.hotData.Delete(key)
	h.detectors.Delete(key)

	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()

	// 如果key已存在则更新
	if elem, exists := h.cache[key]; exists {
		elem.Value.(*cacheItem).value = item.value
		elem.Value.(*cacheItem).expire = item.expire
		h.cacheList.MoveToFront(elem)
		return
	}

	// 淘汰最久未使用的条目
	if h.cacheList.Len() >= h.sizeLimit {
		tail := h.cacheList.Back()
		if tail != nil {
			var delKey string
			for k, v := range h.cache {
				if v == tail {
					delKey = k
					break
				}
			}
			if delKey != "" {
				delete(h.cache, delKey)
				h.detectors.Delete(delKey)
			}
			h.cacheList.Remove(tail)
		}
	}

	elem := h.cacheList.PushFront(item)
	h.cache[key] = elem
}

// 关闭缓存
func (h *GoHotSpot) Close() {
	close(h.closeChan)
}

// Get 获取缓存值
func (h *GoHotSpot) Get(ctx context.Context, key string) (val interface{}, err error) {
	if item, ok := h.getItem(key); ok {
		return item.value, nil
	}
	return nil, nil
}

// Set 设置缓存值
func (h *GoHotSpot) Set(ctx context.Context, key string, value interface{}, ttl time.Duration, opt ...*StringSetOption) (ok bool, err error) {
	// 处理可选参数
	var option *StringSetOption
	if len(opt) > 0 && opt[0] != nil {
		option = opt[0]
	}

	item := &cacheItem{
		value:  value,
		expire: time.Now().Add(ttl).Unix(),
	}

	// 1. 优先更新热点区
	if val, exists := h.hotData.Load(key); exists {
		// 如果设置了NX选项且key已存在，则不更新
		if option != nil && option.Mode == "NX" {
			return false, nil
		}
		hotItem := val.(*cacheItem)
		hotItem.value = value
		hotItem.expire = item.expire
		return true, nil
	}

	// 2. 更新普通缓存
	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()

	if elem, exists := h.cache[key]; exists {
		// 如果设置了NX选项且key已存在，则不更新
		if option != nil && option.Mode == "NX" {
			return false, nil
		}
		elem.Value.(*cacheItem).value = value
		elem.Value.(*cacheItem).expire = item.expire
		h.cacheList.MoveToFront(elem)
		return true, nil
	}

	// 淘汰最久未使用的条目
	if h.cacheList.Len() >= h.sizeLimit {
		tail := h.cacheList.Back()
		if tail != nil {
			var delKey string
			for k, v := range h.cache {
				if v == tail {
					delKey = k
					break
				}
			}
			if delKey != "" {
				delete(h.cache, delKey)
				h.detectors.Delete(delKey)
			}
			h.cacheList.Remove(tail)
		}
	}

	elem := h.cacheList.PushFront(item)
	h.cache[key] = elem
	return true, nil
}

// Del 删除缓存
func (h *GoHotSpot) Del(ctx context.Context, key string) (ok bool, err error) {
	_, hot := h.hotData.Load(key)
	if hot {
		h.hotData.Delete(key)
		h.detectors.Delete(key)
		return true, nil
	}

	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()
	if elem, exists := h.cache[key]; exists {
		delete(h.cache, key)
		h.cacheList.Remove(elem)
		h.detectors.Delete(key)
		return true, nil
	}
	return false, nil
}

// Exists 判断缓存是否存在
func (h *GoHotSpot) Exists(ctx context.Context, key string) (exists bool, err error) {
	if _, ok := h.getItem(key); ok {
		return true, nil
	}
	return false, nil
}

// Expire 设置过期时间
func (h *GoHotSpot) Expire(ctx context.Context, key string, ttl time.Duration) (ok bool, err error) {
	now := time.Now().Add(ttl).Unix()
	if val, ok := h.hotData.Load(key); ok {
		item := val.(*cacheItem)
		item.expire = now
		h.hotData.Store(key, item)
		return true, nil
	}

	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()
	if elem, exists := h.cache[key]; exists {
		elem.Value.(*cacheItem).expire = now
		return true, nil
	}
	return false, nil
}

// Incr 自增
func (h *GoHotSpot) Incr(ctx context.Context, key string) (n int64, err error) {
	return h.IncrBy(ctx, key, 1)
}

// IncrBy 按指定值自增
func (h *GoHotSpot) IncrBy(ctx context.Context, key string, v int64) (n int64, err error) {
	// 先检查热点数据
	if val, ok := h.hotData.Load(key); ok {
		item := val.(*cacheItem)
		ival := cast.ToInt64(item.value)
		ival += v
		item.value = ival
		return ival, nil
	}

	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()
	if elem, ok := h.cache[key]; ok {
		item := elem.Value.(*cacheItem)
		ival := cast.ToInt64(item.value)
		ival += v
		item.value = ival
		return ival, nil
	}

	// 不存在则初始化
	h.cache[key] = h.cacheList.PushFront(&cacheItem{value: v, expire: 0})
	return v, nil
}

// Decr 自减
func (h *GoHotSpot) Decr(ctx context.Context, key string) (n int64, err error) {
	return h.IncrBy(ctx, key, -1)
}

// DecrBy 按指定值自减
func (h *GoHotSpot) DecrBy(ctx context.Context, key string, v int64) (n int64, err error) {
	return h.IncrBy(ctx, key, -v)
}

// TTL 获取剩余过期时间
func (h *GoHotSpot) TTL(ctx context.Context, key string) (ttl time.Duration, err error) {
	now := time.Now().Unix()
	if val, ok := h.hotData.Load(key); ok {
		item := val.(*cacheItem)
		if item.expire == 0 {
			return -1, nil
		}
		if now > item.expire {
			return -1, nil
		}
		return time.Duration(item.expire-now) * time.Second, nil
	}

	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()
	if elem, exists := h.cache[key]; exists {
		item := elem.Value.(*cacheItem)
		if item.expire == 0 {
			return -1, nil
		}
		if now > item.expire {
			return -1, nil
		}
		return time.Duration(item.expire-now) * time.Second, nil
	}
	return -2, nil
}

// getItem 辅助方法，获取未过期的缓存项
func (h *GoHotSpot) getItem(key string) (*cacheItem, bool) {
	now := time.Now().Unix()
	if val, ok := h.hotData.Load(key); ok {
		item := val.(*cacheItem)
		if item.expire == 0 || now <= item.expire {
			h.recordAccess(key, true)
			return item, true
		}
		h.hotData.Delete(key)
		h.detectors.Delete(key)
	}

	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()
	if elem, exists := h.cache[key]; exists {
		item := elem.Value.(*cacheItem)
		if item.expire == 0 || now <= item.expire {
			h.cacheList.MoveToFront(elem)
			h.recordAccess(key, false)
			return item, true
		}
		delete(h.cache, key)
		h.cacheList.Remove(elem)
		h.detectors.Delete(key)
	}
	return nil, false
}

// 记录访问热度
func (h *GoHotSpot) recordAccess(key string, isHot bool) {
	// 原子操作更新计数器
	det, _ := h.detectors.LoadOrStore(key, &hotDetector{})
	detector := det.(*hotDetector)

	// 更新短期计数器
	if !isHot {
		atomic.AddUint32(&detector.counter, 1)
	}

	// 更新EWMA（使用原子操作保证线程安全）
	for {
		oldBits := atomic.LoadUint32(&detector.ewma)
		oldEWMA := math.Float32frombits(oldBits)
		newEWMA := 0.3*float32(atomic.LoadUint32(&detector.counter)) + 0.7*oldEWMA
		newBits := math.Float32bits(newEWMA)
		if atomic.CompareAndSwapUint32(&detector.ewma, oldBits, newBits) {
			break
		}
	}

	// 更新最后访问时间
	atomic.StoreInt64(&detector.lastAccess, time.Now().Unix())

	// 检查是否升级为热点
	if !isHot && atomic.LoadUint32(&detector.counter) >= h.hotThreshold {
		h.promoteToHot(key)
	}
}

// 提升为热点数据
func (h *GoHotSpot) promoteToHot(key string) {
	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()

	if elem, exists := h.cache[key]; exists {
		item := elem.Value.(*cacheItem)
		h.hotData.Store(key, item)
		delete(h.cache, key)
		h.cacheList.Remove(elem)
	}
}
