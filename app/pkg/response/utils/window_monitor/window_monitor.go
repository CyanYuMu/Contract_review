package window_monitor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/su_pool"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
)

// 对比模式类型
type CompareMode int

const (
	Sequential   CompareMode = iota // 环比：连续时间窗口对比
	YearOverYear                    // 同比：不同日期同一时间点对比
)

// 对比结果
type CompareResult struct {
	WindowA      int         // 窗口A的计数
	WindowB      int         // 窗口B的计数
	ChangeValue  int         // 变化值 (B - A)
	GrowthRate   float64     // 增长率 ((B - A) / A * 100)
	CompareMode  CompareMode // 对比模式
	WindowAStart time.Time   // 窗口A开始时间
	WindowAEnd   time.Time   // 窗口A结束时间
	WindowBStart time.Time   // 窗口B开始时间
	WindowBEnd   time.Time   // 窗口B结束时间
}

// 窗口数据
type WindowData struct {
	Count     int       // 窗口计数
	StartTime time.Time // 窗口开始时间
	EndTime   time.Time // 窗口结束时间
}

// 定义接口
type Counter interface {
	Increment(ctx context.Context, key string, incrBy int, ttl time.Duration, now time.Time) // 增加计数
	GetCount(ctx context.Context, key string, now time.Time) int                             // 获取当前计数
	Reset(ctx context.Context, key string, now time.Time)                                    // 重置计数
}

// 实现一个简单的计数器
type SimpleCounter struct {
	counts map[string]int
	mu     sync.RWMutex
}

func NewSimpleCounter() *SimpleCounter {
	return &SimpleCounter{
		counts: make(map[string]int),
	}
}

func (c *SimpleCounter) Increment(ctx context.Context, key string, incrBy int, ttl time.Duration, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts[key] += incrBy
}

func (c *SimpleCounter) GetCount(ctx context.Context, key string, now time.Time) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.counts[key]
}

func (c *SimpleCounter) Reset(ctx context.Context, key string, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.counts, key)
}

// 监听规则
type MonitorRule struct {
	WindowSize  time.Duration       // 单个窗口大小
	WindowCount int                 // 窗口数量
	CompareMode CompareMode         // 对比模式
	Counter     Counter             // 计数器
	OnCompare   func(CompareResult) // 对比结果回调
	MinNumber   *int64              // 最小计数, 当计数小于此值时, 使用最小值进行对比
	MaxNumber   *int64              // 最大计数, 当计数大于此值时, 使用最大值进行对比
}

// 单个监听器
type monitorListener struct {
	projectId string
	scene     string
	rule      MonitorRule
	windows   []WindowData // 窗口数据列表
	mu        sync.Mutex   // 保护windows的锁
}

// 窗口监控器
type WindowMonitor struct {
	tickerInterval time.Duration               // ticker间隔
	listeners      map[string]*monitorListener // 监听器映射，key为 projectId:scene
	mu             sync.RWMutex                // 保护listeners的锁
	ctx            context.Context             // 上下文
	cancel         context.CancelFunc          // 取消函数
	ticker         *time.Ticker                // 定时器
	pool           su_pool.Pool                // 协程池
}

type Param struct {
	Interval        time.Duration // 间隔时间
	NumOfGoRoutines int           // 协程数量, 基于协程池进行处理, 缓解阻塞
}

// 创建新的窗口监控器
func New(param Param) *WindowMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	pool := su_pool.NewLite(param.NumOfGoRoutines, su_pool.WithPanicCallback(func(err interface{}) {
		su_logger.ErrorWithNotify(ctx, nil, "panic in go routine", su_logger.E().Any("err", err))
	}))
	return &WindowMonitor{
		tickerInterval: param.Interval,
		listeners:      make(map[string]*monitorListener),
		ctx:            ctx,
		cancel:         cancel,
		pool:           pool,
	}
}

// 生成监听器的唯一键
func getListenerKey(projectId, scene string) string {
	return fmt.Sprintf("%s:%s", projectId, scene)
}

// 注册监听策略
func (m *WindowMonitor) Register(ctx context.Context, projectId string, scene string, rule MonitorRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := getListenerKey(projectId, scene)

	// 检查是否已存在，如果存在则覆盖
	listener := &monitorListener{
		projectId: projectId,
		scene:     scene,
		rule:      rule,
		windows:   make([]WindowData, 0, rule.WindowCount),
	}

	// 初始化第一个窗口
	now := time.Now()
	firstWindow := WindowData{
		Count:     0,
		StartTime: now,
		EndTime:   now.Add(rule.WindowSize),
	}
	listener.windows = append(listener.windows, firstWindow)

	m.listeners[key] = listener
	return nil
}

// 取消注册监听策略
func (m *WindowMonitor) UnRegister(ctx context.Context, projectId string, scene string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := getListenerKey(projectId, scene)
	listener, exists := m.listeners[key]
	if !exists {
		return fmt.Errorf("listener not found: %s", key)
	}

	// 清理计数器中的数据（可选）
	// 这里可以根据需要清理计数器中的相关数据
	if listener.rule.Counter != nil {
		listener.mu.Lock()
		for i, window := range listener.windows {
			windowKey := listener.getWindowKey(i, window.StartTime)
			listener.rule.Counter.Reset(ctx, windowKey, time.Now())
		}
		listener.mu.Unlock()
	}

	// 从监听器列表中删除
	delete(m.listeners, key)
	return nil
}

// 判断监听器是否存在
func (m *WindowMonitor) IsRegistered(projectId string, scene string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := getListenerKey(projectId, scene)
	_, exists := m.listeners[key]
	return exists
}

// 生成窗口的key
func (l *monitorListener) getWindowKey(windowIndex int, windowStart time.Time) string {
	if l.rule.CompareMode == YearOverYear {
		// 同比模式：使用时间点（小时:分钟）作为key，相同时间点使用相同key
		return fmt.Sprintf("%s:%s:%s", l.projectId, l.scene, windowStart.Format("15:04"))
	}
	// 环比模式：使用窗口索引，每个窗口有独立的key
	return fmt.Sprintf("%s:%s:%d", l.projectId, l.scene, windowIndex)
}

// 启动监控
func (m *WindowMonitor) Start() {
	m.ticker = time.NewTicker(m.tickerInterval)
	go m.run()
}

// 运行监控循环
func (m *WindowMonitor) run() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.ticker.C:
			m.checkAndRotateWindows()
		}
	}
}

// 检查并切换窗口
func (m *WindowMonitor) checkAndRotateWindows() {
	now := time.Now()
	m.mu.RLock()
	listeners := make([]*monitorListener, 0, len(m.listeners))
	for _, listener := range m.listeners {
		listeners = append(listeners, listener)
	}
	m.mu.RUnlock()

	// 批量处理所有监听器
	for _, listener := range listeners {
		m.pool.AddTask(func() error {
			m.processListener(listener, now)
			return nil
		})
	}
	m.pool.Wait()
}

// 处理单个监听器
func (m *WindowMonitor) processListener(listener *monitorListener, now time.Time) {
	listener.mu.Lock()
	defer listener.mu.Unlock()

	if len(listener.windows) == 0 {
		return
	}

	currentWindow := &listener.windows[len(listener.windows)-1]

	// 检查当前窗口是否到期
	if now.Before(currentWindow.EndTime) {
		return // 窗口未到期，不处理
	}

	// 获取当前窗口的计数
	key := listener.getWindowKey(len(listener.windows)-1, currentWindow.StartTime)
	currentWindow.Count = listener.rule.Counter.GetCount(m.ctx, key, now)

	// 创建新窗口
	newWindow := WindowData{
		Count:     0,
		StartTime: now,
		EndTime:   now.Add(listener.rule.WindowSize),
	}
	listener.windows = append(listener.windows, newWindow)

	// 如果窗口数量超过限制，移除最旧的窗口
	if len(listener.windows) > listener.rule.WindowCount {
		listener.windows = listener.windows[1:]
	}

	// 执行对比
	m.performCompare(listener, now)
}

// 执行对比
func (m *WindowMonitor) performCompare(listener *monitorListener, now time.Time) {
	if len(listener.windows) < 2 {
		return
	}

	var windowA, windowB WindowData

	if listener.rule.CompareMode == Sequential {
		// 环比模式：使用连续的两个窗口
		windowA = listener.windows[len(listener.windows)-2]
		windowB = listener.windows[len(listener.windows)-1]
	} else {
		// 同比模式：找到不同日期同一时间点的窗口（通常是上一天同一时间）
		currentWindow := listener.windows[len(listener.windows)-1]
		currentTime := currentWindow.StartTime
		windowB = currentWindow

		// 计算目标时间（往前推24小时，保持相同的小时和分钟）
		targetTime := currentTime.Add(-24 * time.Hour)
		targetHour := targetTime.Hour()
		targetMinute := targetTime.Minute()

		// 在历史窗口中查找匹配的窗口（不同日期但相同时间点）
		// 允许窗口大小范围内的误差
		found := false
		bestMatch := -1
		minDiff := time.Duration(0)

		for i := len(listener.windows) - 2; i >= 0; i-- {
			window := listener.windows[i]
			windowTime := window.StartTime

			// 检查是否是不同日期但相同时间点（小时、分钟相同）
			if windowTime.Hour() == targetHour &&
				windowTime.Minute() == targetMinute &&
				windowTime.Day() != currentTime.Day() {
				// 找到最接近目标时间的窗口
				diff := windowTime.Sub(targetTime)
				if diff < 0 {
					diff = -diff
				}
				if bestMatch == -1 || diff < minDiff {
					bestMatch = i
					minDiff = diff
				}
			}
		}

		if bestMatch >= 0 && minDiff < listener.rule.WindowSize {
			windowA = listener.windows[bestMatch]
			found = true
		}

		if !found {
			return // 没有找到匹配的窗口，不进行对比
		}
	}

	// 应用最小值限制，确保windowA的计数达到最小值要求
	// 如果windowA的计数小于最小值，使用最小值作为基准进行对比，避免因基数过小导致误判
	windowACount := windowA.Count
	if listener.rule.MinNumber != nil && int64(windowACount) < *listener.rule.MinNumber {
		windowACount = int(*listener.rule.MinNumber)
	}
	// 应用最大值限制，确保windowA的计数达到最大值要求
	if listener.rule.MaxNumber != nil && int64(windowACount) > *listener.rule.MaxNumber {
		windowACount = int(*listener.rule.MaxNumber)
	}

	// 计算变化值和增长率
	changeValue := windowB.Count - windowACount
	var growthRate float64
	if windowACount > 0 {
		growthRate = float64(changeValue) / float64(windowACount) * 100
	} else if windowB.Count > 0 {
		growthRate = 100.0 // 从0增长到正数，增长率为100%
	}

	result := CompareResult{
		WindowA:      windowA.Count,
		WindowB:      windowB.Count,
		ChangeValue:  changeValue,
		GrowthRate:   growthRate,
		CompareMode:  listener.rule.CompareMode,
		WindowAStart: windowA.StartTime,
		WindowAEnd:   windowA.EndTime,
		WindowBStart: windowB.StartTime,
		WindowBEnd:   windowB.EndTime,
	}

	// 调用回调函数
	if listener.rule.OnCompare != nil {
		listener.rule.OnCompare(result)
	}
}

// 增加计数
func (m *WindowMonitor) Increment(ctx context.Context, projectId string, scene string, incrBy int) {
	now := time.Now()
	key := getListenerKey(projectId, scene)

	m.mu.RLock()
	listener, exists := m.listeners[key]
	m.mu.RUnlock()

	if !exists {
		return
	}

	listener.mu.Lock()
	defer listener.mu.Unlock()

	if len(listener.windows) == 0 {
		return
	}

	currentWindow := listener.windows[len(listener.windows)-1]
	windowKey := listener.getWindowKey(len(listener.windows)-1, currentWindow.StartTime)
	listener.rule.Counter.Increment(ctx, windowKey, incrBy, listener.rule.WindowSize, now)
}

// 停止监控
func (m *WindowMonitor) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	if m.ticker != nil {
		m.ticker.Stop()
	}
}
