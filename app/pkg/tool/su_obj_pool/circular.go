package su_obj_pool

import (
	"errors"
	"sync/atomic"
)

var ErrQueueFull = errors.New("queue is full")
var ErrQueueEmpty = errors.New("queue is empty")

/*
* @description 循环队列, 定长, 复用
 */
type Circular struct {
	data       []interface{}
	head, tail int32
	capacity   int32
}

func NewCircularQueue(capacity int32) *Circular {
	return &Circular{
		data:     make([]interface{}, capacity),
		capacity: capacity,
	}
}

func (q *Circular) Enqueue(item interface{}) error {
	for {
		head := atomic.LoadInt32(&q.head)
		tail := atomic.LoadInt32(&q.tail)

		nextTail := (tail + 1) % q.capacity

		if nextTail == head {
			return ErrQueueFull
		}

		if atomic.CompareAndSwapInt32(&q.tail, tail, nextTail) {
			q.data[tail] = item
			return nil
		}
	}
}

func (q *Circular) Dequeue() (interface{}, error) {
	for {
		head := atomic.LoadInt32(&q.head)
		tail := atomic.LoadInt32(&q.tail)

		if head == tail {
			return nil, ErrQueueEmpty
		}

		nextHead := (head + 1) % q.capacity

		if atomic.CompareAndSwapInt32(&q.head, head, nextHead) {
			item := q.data[head]
			q.data[head] = nil // Remove reference for GC
			return item, nil
		}
	}
}

func (q *Circular) IsEmpty() bool {
	head := atomic.LoadInt32(&q.head)
	tail := atomic.LoadInt32(&q.tail)
	return head == tail
}

func (q *Circular) IsFull() bool {
	head := atomic.LoadInt32(&q.head)
	tail := atomic.LoadInt32(&q.tail)
	return (tail+1)%q.capacity == head
}
