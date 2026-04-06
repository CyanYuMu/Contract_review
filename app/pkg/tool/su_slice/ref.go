package su_slice

import (
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
)

type StringRef struct {
	ref    map[string]int
	inited atomic.Bool
	mu     sync.Mutex
}

var ErrNilData = errors.New("nil data")
var ErrNotSlice = errors.New("input value is not a slice")
var ErrNilGetKey = errors.New("nil getKey")

func (r *StringRef) Init(data interface{}, GetKey func(v interface{}) string) error {
	// 快速路径：避免重复初始化
	if r.inited.Load() {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.inited.Load() {
		return nil
	}

	err := r.init(data, GetKey)
	if err == nil {
		r.inited.Store(true)
	}
	return err
}

func (r *StringRef) init(data interface{}, getKey func(v interface{}) string) error {
	if data == nil {
		return ErrNilData
	}
	if getKey == nil {
		return ErrNilGetKey
	}

	rv := reflect.ValueOf(data)
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return ErrNilData
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Slice {
		return ErrNotSlice
	}

	n := rv.Len()
	ref := make(map[string]int, n)
	for i := 0; i < n; i++ {
		ref[getKey(rv.Index(i).Interface())] = i
	}
	r.ref = ref
	return nil
}

/*Pos
* @Description: get pos by key
* @param key
* @return pos -1 => key不存在
* @return exists
 */
func (r *StringRef) Pos(key string) (pos int) {
	pos = -1
	if !r.inited.Load() {
		return
	}

	pos, exists := r.ref[key]
	if !exists {
		pos = -1
	}

	return
}
