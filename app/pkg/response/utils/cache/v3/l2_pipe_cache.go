package cache

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func L2GetSet(ctx context.Context, l1Cache GoCacheCmd, l2Rds Rds, key string, sourceGetter func(ctx context.Context) (interface{}, error), opt *L2GetSetOptions) (rs *L2Result) {
	if ctx == nil {
		return &L2Result{Err: errors.New("context cannot be nil")}
	}

	options := getDefaultL2GetSetOptions()
	options.Apply(opt)

	rs = &L2Result{}

	l1Val, err := options.L1Getter(ctx, l1Cache, key)
	if err != nil {
		rs.Err = err
		return rs
	}

	if l1Val == nil {
		// 从二级缓存查询
		l2Val, err1 := options.L2Getter(ctx, l2Rds, key)
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
				l2Val, err = options.L2Setter(ctx, l2Rds, key, sourceVal, l2Ttl)
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
			_, err = options.L1Setter(ctx, l1Cache, key, l1Val, l1Ttl)
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
	}

	return rs
}

type L2PipeResult struct {
	Err   error
	Items map[string]interface{}
	// 对 empty 值进行忽略
	notIgnoreEmpty bool
}

func (l *L2PipeResult) NotIgnoreEmpty(b bool) *L2PipeResult {
	if l == nil {
		return l
	}
	l.notIgnoreEmpty = b
	return l
}

func (l *L2PipeResult) Get(key string) interface{} {
	if l == nil {
		return nil
	}
	v := l.Items[key]
	if !l.notIgnoreEmpty && IsEmptyValue(v) {
		return nil
	}
	return v
}

type L2PipeGetSetOptions struct {
	// * 获取缓存key
	GetCacheKey func(key string) string
	// ? default 5min
	L1Ttl time.Duration
	// ? default 10min
	L2Ttl time.Duration
	// ? default 1min
	L1EmptyTtl time.Duration
	// ? default 5min
	L2EmptyTtl time.Duration
	// ? 一级缓存操作, 默认按照string操作
	L1Setter func(ctx context.Context, cacheInst GoCacheCmd, items map[string]interface{}, ttl time.Duration, emptyTtl time.Duration, customerTtl func(k string, isEmpty bool) time.Duration, getCacheKey GetCacheKey) (err error)
	// ? 返回的interface{}是给上层使用的
	L1Getter func(ctx context.Context, cacheInst GoCacheCmd, keys []string, getCacheKey GetCacheKey) (rs *L2PipeResult, err error)

	// ? 二级缓存操作, 默认按照string操作
	L2Getter func(ctx context.Context, cacheInst Rds, keys []string, getCacheKey GetCacheKey) (rs *L2PipeResult, err error)
	L2Setter func(ctx context.Context, cacheInst Rds, items map[string]interface{}, ttl time.Duration, emptyTtl time.Duration, customerTtl func(k string, isEmpty bool) time.Duration, getCacheKey GetCacheKey) (rs map[string]interface{}, err error)
	// ? 二级缓存数据转换成一级数据
	L2Transformer func(val interface{}) (interface{}, error)
	// 自定义ttl,  会调用该函数
	L1CustomerTtl func(k string, isEmpty bool) time.Duration
	L2CustomerTtl func(k string, isEmpty bool) time.Duration
}

func (o *L2PipeGetSetOptions) Apply(opt *L2PipeGetSetOptions) {
	if opt == nil {
		return
	}
	if opt.L1Ttl != 0 {
		o.L1Ttl = opt.L1Ttl
	}
	if opt.L2Ttl != 0 {
		o.L2Ttl = opt.L2Ttl
	}
	if opt.L1EmptyTtl != 0 {
		o.L1EmptyTtl = opt.L1EmptyTtl
	}
	if opt.L2EmptyTtl != 0 {
		o.L2EmptyTtl = opt.L2EmptyTtl
	}
	if opt.L1Setter != nil {
		o.L1Setter = opt.L1Setter
	}
	if opt.L1Getter != nil {
		o.L1Getter = opt.L1Getter
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

	if opt.GetCacheKey != nil {
		o.GetCacheKey = opt.GetCacheKey
	}

	if opt.L1CustomerTtl != nil {
		o.L1CustomerTtl = opt.L1CustomerTtl
	}
	if opt.L2CustomerTtl != nil {
		o.L2CustomerTtl = opt.L2CustomerTtl
	}
}

type GetCacheKey func(key string) string

func getDefaultL2PipeGetSetOptions() *L2PipeGetSetOptions {
	o := &L2PipeGetSetOptions{
		L1Ttl:      time.Minute * 5,
		L2Ttl:      time.Minute * 10,
		L1EmptyTtl: time.Minute * 1,
		L2EmptyTtl: time.Minute * 5,
		GetCacheKey: func(key string) string {
			return "cache_" + key
		},
		L2Transformer: func(val interface{}) (interface{}, error) {
			return val, nil
		},
	}

	o.L1Getter = func(ctx context.Context, cacheInst GoCacheCmd, ids []string, getCacheKey GetCacheKey) (rs *L2PipeResult, err error) {
		rs = &L2PipeResult{
			Items: make(map[string]interface{}),
		}
		for _, id := range ids {
			ck := getCacheKey(id)
			val, err := cacheInst.Get(ctx, ck)
			if err != nil {
				if IsNotFoundError(err) {
					continue
				}
				return nil, err
			}
			if val == nil {
				continue
			}
			rs.Items[id] = val
		}
		return rs, nil
	}

	o.L1Setter = func(ctx context.Context, cacheInst GoCacheCmd, items map[string]interface{}, ttl time.Duration, emptyTtl time.Duration, customerTtl func(k string, isEmpty bool) time.Duration, getCacheKey GetCacheKey) (err error) {
		for k, v := range items {
			isEmpty := IsEmptyValue(v)
			if isEmpty {
				if emptyTtl == 0 {
					continue
				}
				v = EmptyString
			}
			ck := getCacheKey(k)
			if !isEmpty && customerTtl != nil {
				ttl = customerTtl(k, isEmpty)
			}
			_, err = cacheInst.Set(ctx, ck, v, ttl)
			if err != nil {
				return err
			}
		}
		return nil
	}

	o.L2Getter = func(ctx context.Context, cacheInst Rds, ids []string, getCacheKey GetCacheKey) (rs *L2PipeResult, err error) {
		rs = &L2PipeResult{
			Items: make(map[string]interface{}),
		}
		p := cacheInst.Pipeline()
		for _, id := range ids {
			ck := getCacheKey(id)
			p.Get(ctx, ck)
		}
		cmders, err := p.Exec(ctx)
		if len(cmders) == 0 {
			return nil, fmt.Errorf("no cmders %v", err)
		}
		for i, cmd := range cmders {
			if cmd.Err() != nil {
				if IsNotFoundError(cmd.Err()) {
					continue
				}
				return nil, cmd.Err()
			}
			if v, ok := cmd.(StringCmd); ok {
				cacheStr, _ := v.Result()
				if cacheStr != "" {
					rs.Items[ids[i]] = cacheStr
				}
			}
		}
		return rs, nil
	}

	o.L2Setter = func(ctx context.Context, cacheInst Rds, items map[string]interface{}, ttl time.Duration, emptyTtl time.Duration, customerTtl func(k string, isEmpty bool) time.Duration, getCacheKey GetCacheKey) (rs map[string]interface{}, err error) {
		p := cacheInst.Pipeline()
		rs = make(map[string]interface{})
		for k, v := range items {
			ck := getCacheKey(k)
			if IsEmptyValue(v) {
				if emptyTtl == 0 {
					continue
				}
				p.Set(ctx, ck, EmptyString, emptyTtl)
				rs[k] = EmptyString
			} else {
				cv, err := ToString(v)
				if err != nil {
					return nil, err
				}
				if customerTtl != nil {
					ttl = customerTtl(k, false)
				}
				p.Set(ctx, ck, cv, ttl)
				rs[k] = cv
			}
		}
		_, err = p.Exec(ctx)
		if err != nil {
			return nil, err
		}
		return rs, nil
	}

	return o
}

func L2PipeGetSet(ctx context.Context, l1Cache GoCacheCmd, l2Rds Rds, keys []string, sourceGetter func(ctx context.Context, keys []string) (map[string]interface{}, error), opt *L2PipeGetSetOptions) (rs *L2PipeResult) {
	if ctx == nil {
		return &L2PipeResult{
			Err: errors.New("context cannot be nil"),
		}
	}

	options := getDefaultL2PipeGetSetOptions()
	options.Apply(opt)

	rs = &L2PipeResult{
		Items: make(map[string]interface{}),
	}

	// 先从一级缓存批量获取
	l1Val, err := options.L1Getter(ctx, l1Cache, keys, options.GetCacheKey)
	if err != nil {
		rs.Err = err
		return rs
	}
	l1Val.NotIgnoreEmpty(true)

	notHitKeys := make([]string, 0)
	for _, key := range keys {
		cv := l1Val.Get(key)
		if cv == nil {
			notHitKeys = append(notHitKeys, key)
		} else {
			// 如果一级缓存有值,  直接赋值给结果
			if IsEmptyValue(cv) {
				continue
			}
			rs.Items[key] = cv
		}
	}

	if len(notHitKeys) == 0 {
		return rs
	}

	// 从二级缓存批量获取
	l2Val, err := options.L2Getter(ctx, l2Rds, notHitKeys, options.GetCacheKey)
	if err != nil && !IsNotFoundError(err) {
		rs.Err = err
		return rs
	}
	l2Val.NotIgnoreEmpty(true)

	l2NotHitKeys := make([]string, 0)
	var goCacheValue interface{}
	for _, key := range notHitKeys {
		cv := l2Val.Get(key)
		if cv == nil {
			l2NotHitKeys = append(l2NotHitKeys, key)
		} else {
			// 二级缓存有值, 转换成一级缓存
			if IsEmptyValue(cv) {
				goCacheValue = EmptyString
			} else {
				goCacheValue, err = options.L2Transformer(cv)
				if err != nil {
					rs.Err = err
					return rs
				}
			}

			rs.Items[key] = goCacheValue
			// 缓存到一级缓存
			options.L1Setter(ctx, l1Cache, map[string]interface{}{key: goCacheValue}, options.L1Ttl, options.L2EmptyTtl, options.L1CustomerTtl, options.GetCacheKey)
		}
	}

	if len(l2NotHitKeys) == 0 {
		return rs
	}

	sourceRef, err := sourceGetter(ctx, l2NotHitKeys)
	if err != nil {
		rs.Err = err
		return rs
	}
	l2CacheItems := make(map[string]interface{})
	for _, k := range l2NotHitKeys {
		if v, ok := sourceRef[k]; ok {
			//rs.Items[k] = v
			l2CacheItems[k] = v
		} else {
			l2CacheItems[k] = nil
		}
	}

	if len(l2CacheItems) == 0 {
		return rs
	}

	l2Rs, err := options.L2Setter(ctx, l2Rds, l2CacheItems, options.L2Ttl, options.L2EmptyTtl, options.L2CustomerTtl, options.GetCacheKey)
	if err != nil {
		rs.Err = err
		return rs
	}
	for k := range l2Rs {
		if IsEmptyValue(l2Rs[k]) {
			continue
		}
		l2Rs[k], err = options.L2Transformer(l2Rs[k])
		if err != nil {
			rs.Err = err
			return rs
		}
		rs.Items[k] = l2Rs[k]
	}
	options.L1Setter(ctx, l1Cache, l2Rs, options.L1Ttl, options.L1EmptyTtl, options.L1CustomerTtl, options.GetCacheKey)

	return rs
}
