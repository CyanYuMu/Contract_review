package cache

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	jsoniter "github.com/json-iterator/go"
	"golang.org/x/sync/singleflight"
)

type L2 interface {
	Set(ctx context.Context, k string, v interface{}, ttl time.Duration, opt ...*StringSetOption) (ok bool, err error)
	Get(ctx context.Context, k string) (val interface{}, err error)
	TTL(ctx context.Context, k string) (ttl time.Duration, err error)
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

	// 用于提前加载，确保同一 key 同一时间只有一个协程在加载
	sf singleflight.Group
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

	// ? 是否开启提前加载机制
	// ? 开启后会在一级缓存即将过期前提前从二级缓存加载数据到一级缓存
	// ? 解决高并发场景下一级缓存过期导致大量请求打到二级缓存的问题
	EnablePreload bool
	// ? 提前加载的时间量, 即在过期前多久开始加载
	// ? 默认为 10s, 如果设置值 < 5s 则不会开启提前加载
	PreloadAdvance time.Duration

	// 是否跳过一级缓存查询，直接从二级缓存获取（内部使用）
	skipL1Get bool
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

	// 提前加载配置
	if opt.EnablePreload {
		o.EnablePreload = opt.EnablePreload
	}
	if opt.PreloadAdvance > 0 {
		o.PreloadAdvance = opt.PreloadAdvance
	}
	if opt.skipL1Get {
		o.skipL1Get = opt.skipL1Get
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

func IsEmptyValue(val interface{}) bool {
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

// minPreloadAdvance 最小提前加载时间量，小于此值则不开启提前加载
const minPreloadAdvance = 5 * time.Second

// defaultPreloadAdvance 默认提前加载时间量
const defaultPreloadAdvance = 10 * time.Second

// getEffectivePreloadAdvance 获取有效的提前加载时间量
func (l *L2Cache) getEffectivePreloadAdvance(options *L2GetSetOptions) time.Duration {
	if !options.EnablePreload {
		return 0
	}

	advance := options.PreloadAdvance
	if advance <= 0 {
		// 未设置则使用默认值
		advance = defaultPreloadAdvance
	}

	// 提前量小于最小值则不开启
	if advance < minPreloadAdvance {
		return 0
	}

	// 提前量不能超过 L1Ttl 的一半
	if advance > options.L1Ttl/2 {
		advance = options.L1Ttl / 2
	}

	return advance
}

// needPreload 检查是否需要提前加载
// 通过 L1 缓存的 TTL 方法获取剩余过期时间
func (l *L2Cache) needPreload(ctx context.Context, key string, preloadAdvance time.Duration) bool {
	if preloadAdvance <= 0 {
		return false
	}

	// 通过 L2 接口的 TTL 方法获取一级缓存的剩余过期时间
	ttl, err := l.l1.TTL(ctx, key)
	if err != nil {
		return false
	}

	// ttl < 0 表示 key 不存在或无过期时间
	if ttl < 0 {
		return false
	}

	// 检查是否在提前加载时间窗口内
	return ttl <= preloadAdvance
}

// triggerPreload 触发提前加载（异步）
func (l *L2Cache) triggerPreload(ctx context.Context, key string, sourceGetter func(ctx context.Context) (interface{}, error), options *L2GetSetOptions) {
	go func() {
		// 使用 singleflight 确保同一 key 同一时间只有一个协程在加载
		_, _, _ = l.sf.Do("preload:"+key, func() (interface{}, error) {
			// 使用新的 context 避免原 context 取消导致加载失败
			preloadCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			// 复用 GetSet 逻辑，跳过 L1 查询直接从 L2/Source 加载
			preloadOpt := &L2GetSetOptions{
				L1Ttl:         options.L1Ttl,
				L2Ttl:         options.L2Ttl,
				L1EmptyTtl:    options.L1EmptyTtl,
				L2EmptyTtl:    options.L2EmptyTtl,
				L1Setter:      options.L1Setter,
				L1Getter:      options.L1Getter,
				L2Getter:      options.L2Getter,
				L2Setter:      options.L2Setter,
				L2Transformer: options.L2Transformer,
				skipL1Get:     true, // 跳过 L1 查询
			}
			l.GetSet(preloadCtx, key, sourceGetter, preloadOpt)

			return nil, nil
		})
	}()
}

func (l *L2Cache) GetSet(ctx context.Context, key string, sourceGetter func(ctx context.Context) (interface{}, error), opt *L2GetSetOptions) (rs *L2Result) {
	if ctx == nil {
		return &L2Result{Err: errors.New("context cannot be nil")}
	}

	options := getDefaultL2GetSetOptions()
	options.Apply(opt)

	rs = &L2Result{}

	// 计算有效的提前加载时间量
	preloadAdvance := l.getEffectivePreloadAdvance(options)

	var l1Val interface{}
	var err error

	// 是否跳过 L1 查询（用于提前加载场景）
	if !options.skipL1Get {
		l1Val, err = options.L1Getter(ctx, l.l1, key)
		if err != nil {
			rs.Err = err
			return rs
		}
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
			sourceVal, err2 := sourceGetter(ctx)
			if err2 != nil {
				rs.Err = err2
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
		if IsEmptyValue(l1Val) {
			return rs
		} else {
			rs.Data = l1Val
		}

		// 检查是否需要提前加载
		if l.needPreload(ctx, key, preloadAdvance) {
			l.triggerPreload(ctx, key, sourceGetter, options)
		}
	}

	return rs
}
