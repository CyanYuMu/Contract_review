package su_pool

// Pool 表示一个工作池接口
type Pool interface {
	// AddTask 向池中添加任务
	AddTask(Task)
	// Wait 等待所有任务被分发和完成
	Wait()
	// Release 释放池和所有工作者
	Release()
	// Running 返回当前正在运行的工作者数量
	Running() int
	// GetWorkerCount 返回工作者总数
	GetWorkerCount() int
	// GetTaskQueueSize 返回任务队列大小
	GetTaskQueueSize() int
}

// Task 表示将由工作者执行的函数
// 返回一个错误
type Task func() error
