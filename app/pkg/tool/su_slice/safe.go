package su_slice

import (
	"sync"
)

type SafeSlice struct {
	sync.Mutex
	data []interface{}
}

func NewSafeSlice() *SafeSlice {
	return &SafeSlice{
		data: make([]interface{}, 0),
	}
}

func (ss *SafeSlice) Append(elem interface{}) {
	ss.Lock()
	defer ss.Unlock()
	ss.data = append(ss.data, elem)
}

func (ss *SafeSlice) Get(index int) (interface{}, bool) {
	ss.Lock()
	defer ss.Unlock()

	if index < 0 || index >= len(ss.data) {
		return nil, false
	}

	return ss.data[index], true
}

func (ss *SafeSlice) Length() int {
	ss.Lock()
	defer ss.Unlock()

	return len(ss.data)
}

// 如果元素不存在, 则进行插入
func AppendIfNotExist[T ~string|~int|~int64|~float64|~float32|~bool](src *[]T, item ...T) {
	for _, v := range item {
		if !InArray(v, *src) {
			*src = append(*src, v)
		}
	}
}

// 如果元素不存在, 则从头部追加元素
func PrependIfNotExist[T ~string](src *[]T, item ...T) {
	for _, v := range item {
		if !InArray(v, *src) {
			*src = append([]T{v}, *src...)
		}
	}
}
