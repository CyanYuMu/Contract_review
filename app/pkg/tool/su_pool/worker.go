package su_pool

import (
	"context"
	"fmt"
)

// worker represents a worker in the pool.
type worker struct {
	taskQueue chan Task
}

func newWorker() *worker {
	return &worker{
		taskQueue: make(chan Task, 1),
	}
}

// start starts the worker in a separate goroutine.
// The worker will run tasks from its taskQueue until the taskQueue is closed.
// For the length of the taskQueue is 1, the worker will be pushed back to the pool after executing 1 Task.
func (w *worker) start(pool *goPool, workerIndex int) {
	go func() {
		for t := range w.taskQueue {
			if t != nil {
				err := w.executeTask(t, pool)
				w.handleResult(err, pool)
			}
			pool.pushWorker(workerIndex)
		}
	}()
}

// executeTask executes a Task and returns the result and error.
// If the Task fails, it will be retried according to the retryCount of the pool.
func (w *worker) executeTask(t Task, pool *goPool) (err error) {
	for i := 0; i <= pool.retryCount; i++ {
		if pool.timeout > 0 {
			err = w.executeTaskWithTimeout(t, pool)
		} else {
			err = w.executeTaskWithoutTimeout(t, pool)
		}
		if err == nil || i == pool.retryCount {
			return err
		}
	}
	return
}

// executeTaskWithTimeout executes a Task with a timeout and returns the result and error.
func (w *worker) executeTaskWithTimeout(t Task, pool *goPool) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), pool.timeout)
	defer cancel()

	errChan := make(chan error, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				if pool.panicCallback != nil {
					pool.panicCallback(r)
				}
				select {
				case errChan <- fmt.Errorf("task panic: %v", r):
				default:
				}
			}
		}()
		e := t()
		select {
		case errChan <- e:
		case <-ctx.Done():
			return
		}
	}()

	select {
	case e := <-errChan:
		return e
	case <-ctx.Done():
		return fmt.Errorf("task timed out")
	}
}

func (w *worker) executeTaskWithoutTimeout(t Task, pool *goPool) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if pool.panicCallback != nil {
				pool.panicCallback(r)
			}
			err = fmt.Errorf("task panic: %v", r)
		}
	}()
	return t()
}

func (w *worker) handleResult(err error, pool *goPool) {
	if err != nil && pool.errorCallback != nil {
		pool.errorCallback(err)
	}
}
