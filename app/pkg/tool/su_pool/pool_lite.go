package su_pool

import (
	"context"
	"sort"
	"sync"
	"time"
)

// goPool 表示一个工作者池的实现
type goPool struct {
	workers     []*worker
	workerStack []int
	maxWorkers  int
	// 通过 WithMinWorkers() 设置，用于调整工作者数量。默认等于 maxWorkers
	minWorkers int
	// 任务首先添加到此通道，然后分发给工作者。默认缓冲区大小为100万
	taskQueue chan Task
	// 通过 WithTaskQueueSize() 设置，用于设置任务队列大小。默认为1e6
	taskQueueSize int
	// 通过 WithRetryCount() 设置，用于任务失败时重试。默认为0
	retryCount int
	lock       sync.Locker
	cond       *sync.Cond
	// 通过 WithTimeout() 设置，用于设置任务超时时间。默认为0，表示无超时
	timeout time.Duration
	// 通过 WithResultCallback() 设置，用于处理任务的panic。默认为nil
	panicCallback func(interface{})
	// 通过 WithErrorCallback() 设置，用于处理任务的错误。默认为nil
	errorCallback func(error)
	// adjustInterval 是调整工作者数量的间隔。默认为1秒
	adjustInterval time.Duration
	ctx            context.Context
	// cancel 用于取消上下文。在调用 Release() 时被调用
	cancel context.CancelFunc
}

// NewLite 创建一个轻量级的工作池
func NewLite(maxWorkers int, opts ...Option) Pool {
	ctx, cancel := context.WithCancel(context.Background())
	pool := &goPool{
		maxWorkers: maxWorkers,
		// 默认将 minWorkers 设置为 maxWorkers
		minWorkers: maxWorkers,
		// workers 和 workerStack 应在调用 WithMinWorkers() 后初始化
		workers:        nil,
		workerStack:    nil,
		taskQueue:      nil,
		taskQueueSize:  1e6,
		retryCount:     0,
		lock:           new(sync.Mutex),
		timeout:        0,
		adjustInterval: 1 * time.Second,
		ctx:            ctx,
		cancel:         cancel,
	}
	// 应用选项
	for _, opt := range opts {
		opt(pool)
	}

	pool.taskQueue = make(chan Task, pool.taskQueueSize)
	pool.workers = make([]*worker, pool.minWorkers)
	pool.workerStack = make([]int, pool.minWorkers)

	if pool.cond == nil {
		pool.cond = sync.NewCond(pool.lock)
	}
	// 使用最小数量创建工作者。这里不使用 pushWorker()
	for i := 0; i < pool.minWorkers; i++ {
		wkr := newWorker()
		pool.workers[i] = wkr
		pool.workerStack[i] = i
		wkr.start(pool, i)
	}
	go pool.adjustWorkers()
	go pool.dispatch()
	return pool
}

// AddTask 向池中添加任务
func (p *goPool) AddTask(t Task) {
	p.taskQueue <- t
}

// Wait 等待所有任务被分发和完成
func (p *goPool) Wait() {
	for {
		p.lock.Lock()
		workerStackLen := len(p.workerStack)
		p.lock.Unlock()

		if len(p.taskQueue) == 0 && workerStackLen == len(p.workers) {
			break
		}

		time.Sleep(50 * time.Millisecond)
	}
}

// Release 停止所有工作者并释放资源
func (p *goPool) Release() {
	close(p.taskQueue)
	p.cancel()
	p.cond.Broadcast()
	p.cond.L.Lock()
	for len(p.workerStack) != len(p.workers) {
		p.cond.Wait()
	}
	p.cond.L.Unlock()
	for _, worker := range p.workers {
		close(worker.taskQueue)
	}
	p.workers = nil
	p.workerStack = nil
}

// popWorker 从工作者栈中弹出一个工作者索引
func (p *goPool) popWorker() int {
	p.lock.Lock()
	workerIndex := p.workerStack[len(p.workerStack)-1]
	p.workerStack = p.workerStack[:len(p.workerStack)-1]
	p.lock.Unlock()
	return workerIndex
}

// pushWorker 将工作者索引压入工作者栈
func (p *goPool) pushWorker(workerIndex int) {
	p.lock.Lock()
	p.workerStack = append(p.workerStack, workerIndex)
	p.lock.Unlock()
	p.cond.Signal()
}

// adjustWorkers 根据任务队列中的任务数量调整工作者数量
func (p *goPool) adjustWorkers() {
	ticker := time.NewTicker(p.adjustInterval)
	defer ticker.Stop()

	var needBroadcast bool

	for {
		needBroadcast = false
		select {
		case <-ticker.C:
			p.cond.L.Lock()
			if len(p.taskQueue) > len(p.workers)*3/4 && len(p.workers) < p.maxWorkers {
				// 扩容：需要 Broadcast 唤醒可能在 cond.Wait() 等待空闲 worker 的 dispatch()
				needBroadcast = true
				// 将工作者数量翻倍，直到达到最大值
				newWorkers := min(len(p.workers)*2, p.maxWorkers) - len(p.workers)
				for i := 0; i < newWorkers; i++ {
					wrk := newWorker()
					p.workers = append(p.workers, wrk)
					// 这里不使用 len(p.workerStack)-1，因为当池很忙时它会小于 len(p.workers)-1
					p.workerStack = append(p.workerStack, len(p.workers)-1)
					wrk.start(p, len(p.workers)-1)
				}
			} else if len(p.taskQueue) == 0 && len(p.workerStack) == len(p.workers) && len(p.workers) > p.minWorkers {
				// 缩容：无需 Broadcast，因为此时 taskQueue 为空，dispatch() 阻塞在 range taskQueue，不在 cond.Wait()
				// 将工作者数量减半，直到达到最小值
				removeWorkers := (len(p.workers) - p.minWorkers + 1) / 2
				// 在移除工作者前对工作者栈排序
				// [1,2,3,4,5] -工作中-> [1,2,3] -扩展-> [1,2,3,6,7] -空闲-> [1,2,3,6,7,4,5]
				sort.Ints(p.workerStack)
				for i := len(p.workers) - removeWorkers; i < len(p.workers); i++ {
					close(p.workers[i].taskQueue)
				}
				p.workers = p.workers[:len(p.workers)-removeWorkers]
				p.workerStack = p.workerStack[:len(p.workerStack)-removeWorkers]
			}
			p.cond.L.Unlock()
			// 只在扩容时 Broadcast：唤醒等待空闲 worker 的 dispatch() 协程，使其能够使用新增的 worker
			if needBroadcast {
				p.cond.Broadcast()
			}
		case <-p.ctx.Done():
			return
		}
	}
}

// dispatch 将任务分发给工作者
func (p *goPool) dispatch() {
	for t := range p.taskQueue {
		p.cond.L.Lock()
		for len(p.workerStack) == 0 {
			select {
			case <-p.ctx.Done():
				p.cond.L.Unlock()
				return
			default:
			}
			p.cond.Wait()
		}
		select {
		case <-p.ctx.Done():
			p.cond.L.Unlock()
			return
		default:
		}
		p.cond.L.Unlock()
		workerIndex := p.popWorker()
		if workerIndex < len(p.workers) && p.workers[workerIndex] != nil {
			p.workers[workerIndex].taskQueue <- t
		}
	}
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Running 返回当前正在工作的工作者数量
func (p *goPool) Running() int {
	p.lock.Lock()
	defer p.lock.Unlock()
	return len(p.workers) - len(p.workerStack)
}

// GetWorkerCount 返回池中的工作者总数
func (p *goPool) GetWorkerCount() int {
	p.lock.Lock()
	defer p.lock.Unlock()
	return len(p.workers)
}

// GetTaskQueueSize 返回任务队列的大小
func (p *goPool) GetTaskQueueSize() int {
	p.lock.Lock()
	defer p.lock.Unlock()
	return p.taskQueueSize
}
