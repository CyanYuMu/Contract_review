package su_obj_pool

import (
	"errors"
	"sync"
	"sync/atomic"
)

var ErrNoObject = errors.New("no object")

type Sized struct {
	pool        sync.Pool
	size        int32
	currentSize int32
	newFunc     func() interface{}
}

func NewSizedPool(size int, newFunc func() interface{}) *Sized {
	return &Sized{
		size:    int32(size),
		newFunc: newFunc,
	}
}

func (s *Sized) Put(item interface{}) {
	for {
		currentSize := atomic.LoadInt32(&s.currentSize)
		if currentSize >= s.size {
			// 超过容量，丢弃对象
			return
		}
		if atomic.CompareAndSwapInt32(&s.currentSize, currentSize, currentSize+1) {
			s.pool.Put(item)
			return
		}
	}
}

func (s *Sized) Get() interface{} {
	for {
		currentSize := atomic.LoadInt32(&s.currentSize)
		if currentSize < s.size && s.newFunc != nil {
			if atomic.CompareAndSwapInt32(&s.currentSize, currentSize, currentSize+1) {
				return s.newFunc()
			}
		} else if currentSize > 0 {
			if atomic.CompareAndSwapInt32(&s.currentSize, currentSize, currentSize-1) {
				return s.pool.Get()
			}
		} else {
			return nil
		}
	}
}

/*Try
* @Description: 尝试获取资源, 并自定义回调函数
* @param fn
* @return error
 */
func (s *Sized) Try(fn func(i interface{}) error) error {
	obj := s.Get()
	if obj != nil {
		defer s.Put(obj)
		return fn(obj)
	}

	return ErrNoObject
}
