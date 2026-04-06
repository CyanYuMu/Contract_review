package cache

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	jsoniter "github.com/json-iterator/go"
)

type L2 interface {
	Set(ctx context.Context, k string, v interface{}, ttl time.Duration) (ok bool, err error)
	Get(ctx context.Context, k string) (val interface{}, err error)
}

func getDefaultL2Option() *L2Option {
	return &L2Option{}
}

type L2Option struct {
	// Logger 日志记录器
}

func (o *L2Option) Apply(opt *L2Option) {
	if opt == nil {
		return
	}
}

type L2Cache struct {
	// lock sync.Mutex
	opt *L2Option
	l1  L2
	l2  L2
}

func NewL2(l1 L2, l2 L2, opt *L2Option) *L2Cache {
	options := getDefaultL2Option()
	options.Apply(opt)
	return &L2Cache{
		opt: options,
		l1:  l1,
		l2:  l2,
	}
}

type L2GetSetOptions struct {
	// ? default 5min
	L1Ttl time.Duration
	// ? default 10min
	L2Ttl time.Duration
	// ? default 1min
	L1EmptyTtl time.Duration
	// ? default 5min
	L2EmptyTtl time.Duration

	// ? 一级缓存操作, 默认按照string操作
	L1Setter func(ctx context.Context, cacheInst L2, key string, val interface{}, ttl time.Duration) (data interface{}, lerr error)
	// ? 返回的interface{}是给上层使用的
	L1Getter func(ctx context.Context, cacheInst L2, key string) (interface{}, error)

	// ? 二级缓存操作, 默认按照string操作
	L2Getter func(ctx context.Context, cacheInst L2, key string) (interface{}, error)
	L2Setter func(ctx context.Context, cacheInst L2, key string, val interface{}, ttl time.Duration) (data interface{}, err error)
	// ? 二级缓存数据转换成一级数据
	L2Transformer func(val interface{}) (interface{}, error)
}

func (o *L2GetSetOptions) Apply(opt *L2GetSetOptions) {
	if opt == nil {
		return
	}

	if opt.L1Ttl > 0 {
		o.L1Ttl = opt.L1Ttl
	}

	if opt.L2Ttl > 0 {
		o.L2Ttl = opt.L2Ttl
	}

	if opt.L1EmptyTtl > 0 {
		o.L1EmptyTtl = opt.L1EmptyTtl
	} else if opt.L1EmptyTtl < 0 {
		o.L1EmptyTtl = 0
	}

	if opt.L2EmptyTtl > 0 {
		o.L2EmptyTtl = opt.L2EmptyTtl
	} else if opt.L2EmptyTtl < 0 {
		o.L2EmptyTtl = 0
	}

	if opt.L1Setter != nil {
		o.L1Setter = opt.L1Setter
	}

	if opt.L1Getter != nil {
		o.L1Getter = opt.L1Getter // Fixed: Changed opt.L1Getter to o.L1Getter
	}

	if opt.L2Getter != nil {
		o.L2Getter = opt.L2Getter
	}

	if opt.L2Setter != nil {
		o.L2Setter = opt.L2Setter
	}

	if opt.L2Transformer != nil {
		o.L2Transformer = opt.L2Transformer
	}
}

func getDefaultL2GetSetOptions() *L2GetSetOptions {
	return &L2GetSetOptions{
		L1Ttl:      time.Minute * 5,
		L2Ttl:      time.Minute * 10,
		L1EmptyTtl: time.Minute * 1,
		L2EmptyTtl: time.Minute * 5,
		L1Setter: func(ctx context.Context, cacheInst L2, key string, val interface{}, ttl time.Duration) (data interface{}, err error) {
			if val == nil {
				val = EmptyString
			}
			_, err = cacheInst.Set(ctx, key, val, ttl)
			return val, err
		},
		L1Getter: func(ctx context.Context, cacheInst L2, key string) (interface{}, error) {
			valStr, err := cacheInst.Get(ctx, key)
			if err != nil {
				return nil, err
			}
			return valStr, nil
		},
		L2Getter: func(ctx context.Context, cacheInst L2, key string) (interface{}, error) {
			valStr, err := cacheInst.Get(ctx, key)
			if err != nil {
				return nil, err
			}
			return valStr, nil
		},
		L2Setter: func(ctx context.Context, cacheInst L2, key string, val interface{}, ttl time.Duration) (data interface{}, err error) {
			var valStr string
			if val == nil {
				valStr = EmptyString
			} else {
				valStr, err = ToString(val)
				if err != nil {
					return nil, err
				}
			}

			_, err = cacheInst.Set(ctx, key, valStr, ttl)
			return valStr, err
		},
		L2Transformer: func(val interface{}) (interface{}, error) {
			return val, nil
		},
	}
}

func ToString(val interface{}) (string, error) {
	if val == nil {
		return "", nil
	}

	if s, ok := val.(string); ok {
		return s, nil
	}

	dataStr, err := jsoniter.MarshalToString(val)
	if err != nil {
		return "", err
	}

	return dataStr, nil
}

type L2Result struct {
	Data interface{}
	Err  error
}

func (l *L2Result) Decode(data interface{}) error {
	if l.Err != nil {
		return l.Err
	}

	if l.Data == nil {
		return nil
	}

	if data == nil {
		return errors.New("target data cannot be nil")
	}

	dataVal, okData := data.(*string)
	cacheStr, okCache := l.Data.(string)

	// If val is a string type and Data is also a string, do direct assignment
	// 都是字符串的场景, 直接赋值
	if okData && okCache {
		*dataVal = cacheStr
		return nil
	} else if okCache && !okData {
		return jsoniter.UnmarshalFromString(cacheStr, data)
	} else if !okCache && !okData {
		// 通过反射赋值
		rv := reflect.ValueOf(data)
		if rv.Kind() != reflect.Ptr {
			return errors.New("target must be a pointer")
		}

		if rv.IsNil() {
			return errors.New("target pointer cannot be nil")
		}

		// 对目标类型和源类型进行转换
		srcVal := reflect.ValueOf(l.Data)
		if srcVal.Kind() == reflect.Ptr && rv.Elem().Kind() != reflect.Ptr {
			srcVal = srcVal.Elem()
		} else if srcVal.Kind() != reflect.Ptr && rv.Elem().Kind() == reflect.Ptr {
			// 创建新的指针
			ptr := reflect.New(srcVal.Type())
			ptr.Elem().Set(srcVal)
			srcVal = ptr
		}

		// Check if target is addressable and settable
		if !rv.Elem().CanSet() {
			return errors.New("target value is not settable")
		}

		// Handle special case for maps
		if srcVal.Kind() == reflect.Map && rv.Elem().Kind() == reflect.Map {
			// Create new map if target is nil
			if rv.Elem().IsNil() {
				rv.Elem().Set(reflect.MakeMap(rv.Elem().Type()))
			}

			// Clear existing map
			for _, key := range rv.Elem().MapKeys() {
				rv.Elem().SetMapIndex(key, reflect.Value{})
			}

			// Copy all key-value pairs from source to target map
			for _, key := range srcVal.MapKeys() {
				rv.Elem().SetMapIndex(key, srcVal.MapIndex(key))
			}
		} else {
			// Check type compatibility before setting
			if !srcVal.Type().AssignableTo(rv.Elem().Type()) {
				return errors.New("source type is not assignable to target type")
			}
			rv.Elem().Set(srcVal)
		}
	}

	return nil
}

func (l *L2Cache) isEmpty(val interface{}) bool {
	if val == nil {
		return true
	}

	if s, ok := val.(string); ok {
		return s == EmptyString
	}

	return false
}

func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	errS := strings.ToLower(err.Error())
	return strings.Contains(errS, "not found") || strings.Contains(errS, "not exist") || strings.Contains(errS, "no document") || strings.Contains(errS, "redis: nil")
}

func (l *L2Cache) GetSet(ctx context.Context, key string, sourceGetter func(ctx context.Context) (interface{}, error), opt *L2GetSetOptions) (rs *L2Result) {
	if ctx == nil {
		return &L2Result{Err: errors.New("context cannot be nil")}
	}

	options := getDefaultL2GetSetOptions()
	options.Apply(opt)

	rs = &L2Result{}

	l1Val, err := options.L1Getter(ctx, l.l1, key)
	if err != nil {
		rs.Err = err
		return rs
	}

	if l1Val == nil {
		// 从二级缓存查询
		l2Val, err1 := options.L2Getter(ctx, l.l2, key)
		if err1 != nil {
			if !IsNotFoundError(err1) {
				rs.Err = err1
				return rs
			}
		}

		l2Ttl := options.L2Ttl
		l1Ttl := options.L1Ttl

		if l2Val == nil {
			// 从源获取
			sourceVal, err1 := sourceGetter(ctx)
			if err1 != nil {
				rs.Err = err1
				return rs
			}
			if sourceVal == nil {
				l2Ttl = options.L2EmptyTtl
				l1Ttl = options.L1EmptyTtl
			}
			if l2Ttl > 0 {
				// 设置二级缓存
				l2Val, err = options.L2Setter(ctx, l.l2, key, sourceVal, l2Ttl)
				if err != nil {
					rs.Err = err
					return rs
				}
			}
		}

		rs.Data = l2Val

		if l1Ttl > 0 {
			if l2Val == nil {
				l1Val = EmptyString
			} else {
				l1Val, err = options.L2Transformer(l2Val)
				if err != nil {
					rs.Err = err
					return rs
				}
				rs.Data = l1Val
			}
			_, err = options.L1Setter(ctx, l.l1, key, l1Val, l1Ttl)
			if err != nil {
				rs.Err = err
				return rs
			}
		}
	} else {
		if l.isEmpty(l1Val) {
			return rs
		} else {
			rs.Data = l1Val
		}
	}

	return rs
}
