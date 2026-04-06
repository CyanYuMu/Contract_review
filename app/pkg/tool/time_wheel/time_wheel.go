package timewheel

import (
	"container/heap"
	"context"
	"math"
	"sync"
	"time"

	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
)

// Config 表示时间轮的配置
type Config struct {
	Interval time.Duration // 时间轮的基本间隔
	MaxDelay time.Duration // 支持的最大延迟时间
	SlotSize int           // 每层的槽位数
	levels   int           // 动态计算的层级数
}

// Task 表示一个计划任务
type Task struct {
	ID       string
	Priority int
	Delay    time.Duration
	Execute  func()
	deadline time.Time
}

// TimeWheel 表示一个分层时间轮
type TimeWheel struct {
	config      Config
	ticker      *time.Ticker
	slots       [][]*Task
	currentPos  int
	taskMap     map[string]*Task
	priorityMap map[int]*PriorityQueue
	mu          sync.RWMutex
	stop        chan struct{}
}

// PriorityQueue 实现了heap.Interface接口并持有Task
type PriorityQueue []*Task

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	return pq[i].Priority > pq[j].Priority
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
}

func (pq *PriorityQueue) Push(x interface{}) {
	*pq = append(*pq, x.(*Task))
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[0 : n-1]
	return item
}

// DefaultConfig returns a default configuration for the time wheel
func DefaultConfig() Config {
	return Config{
		Interval: time.Second, // 100ms interval
		MaxDelay: time.Hour,   // 24 hours max delay
		SlotSize: 60,          // 60 slots per level
	}
}

// calculateLevels 计算需要的时间轮层级数
func calculateLevels(interval, maxDelay time.Duration, slotSize int) int {
	if maxDelay <= interval {
		return 1
	}

	totalSlots := int(maxDelay / interval)
	levels := int(math.Ceil(math.Log(float64(totalSlots)) / math.Log(float64(slotSize))))
	if levels < 1 {
		levels = 1
	}
	return levels
}

// Apply applies the given config options to override default values
func (c *Config) Apply(cfg Config) {
	if c == nil {
		return
	}
	if cfg.Interval > 0 {
		c.Interval = cfg.Interval
	}
	if cfg.MaxDelay > 0 {
		c.MaxDelay = cfg.MaxDelay
	}
	if cfg.SlotSize > 0 {
		c.SlotSize = cfg.SlotSize
	}

	// 计算层级数
	c.levels = calculateLevels(c.Interval, c.MaxDelay, c.SlotSize)
}

// New 创建一个新的时间轮
func New(config Config) *TimeWheel {
	cfg := DefaultConfig()
	cfg.Apply(config)

	tw := &TimeWheel{
		config:      cfg,
		slots:       make([][]*Task, cfg.levels*cfg.SlotSize),
		taskMap:     make(map[string]*Task),
		priorityMap: make(map[int]*PriorityQueue),
		stop:        make(chan struct{}),
	}

	for i := range tw.slots {
		tw.slots[i] = make([]*Task, 0)
	}

	tw.ticker = time.NewTicker(config.Interval)
	go tw.run()
	return tw
}

// AddTask 向时间轮添加新任务
func (tw *TimeWheel) AddTask(task *Task) bool {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	if _, exists := tw.taskMap[task.ID]; exists {
		return false
	}

	// 如果延迟时间超过最大支持的延迟时间，则强制设置为最大延迟时间
	if task.Delay > tw.config.MaxDelay {
		su_logger.Warn(context.Background(), "task delay exceed max delay "+task.ID, su_logger.E().String("max_delay", tw.config.MaxDelay.String()))
		task.Delay = tw.config.MaxDelay
	}

	task.deadline = time.Now().Add(task.Delay)

	tw.taskMap[task.ID] = task

	// 计算槽位位置
	delay := int(task.Delay / tw.config.Interval)
	level := 0
	for delay >= tw.config.SlotSize {
		delay /= tw.config.SlotSize
		level++
	}
	if level >= tw.config.levels {
		level = tw.config.levels - 1
		delay = tw.config.SlotSize - 1
	}

	pos := (tw.currentPos + delay) % len(tw.slots)

	// 添加到优先队列
	if _, exists := tw.priorityMap[pos]; !exists {
		pq := make(PriorityQueue, 0)
		tw.priorityMap[pos] = &pq
		heap.Init(tw.priorityMap[pos])
	}
	heap.Push(tw.priorityMap[pos], task)

	// 添加到对应的时间槽
	slotIndex := level*tw.config.SlotSize + delay
	if slotIndex < len(tw.slots) {
		tw.slots[slotIndex] = append(tw.slots[slotIndex], task)
	}

	return true
}

// RemoveTask 从时间轮中移除任务
func (tw *TimeWheel) RemoveTask(taskID string) bool {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	if _, exists := tw.taskMap[taskID]; exists {
		delete(tw.taskMap, taskID)
		return true
	}
	return false
}

// run 启动时间轮
func (tw *TimeWheel) run() {
	for {
		select {
		case <-tw.ticker.C:
			tw.mu.Lock()
			// 执行当前时间槽的任务,优先执行优先级队列中的任务
			if pq, exists := tw.priorityMap[tw.currentPos]; exists {
				for pq.Len() > 0 {
					task := heap.Pop(pq).(*Task)
					if _, exists := tw.taskMap[task.ID]; exists && time.Now().After(task.deadline) {
						go task.Execute()
						delete(tw.taskMap, task.ID)
					}
				}
				delete(tw.priorityMap, tw.currentPos)
			}

			tw.currentPos = (tw.currentPos + 1) % len(tw.slots)
			tw.mu.Unlock()
		case <-tw.stop:
			tw.ticker.Stop()
			return
		}
	}
}

// Stop 停止时间轮
func (tw *TimeWheel) Stop() {
	close(tw.stop)
}
