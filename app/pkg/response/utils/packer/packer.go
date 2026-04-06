package packer

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	jsoniter "github.com/json-iterator/go"

	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/cache/v3"
)

// type Cache interface {
// 	Set(ctx context.Context, k string, v interface{}, ttl time.Duration, opt ...*cache.StringSetOption) (ok bool, err error)
// 	//Del(ctx context.Context, k string) (ok bool, err error)
// 	Get(ctx context.Context, k string) (val interface{}, err error)
// 	HGet(ctx context.Context, k string, field string) (val string, err error)
// 	HGetAll(ctx context.Context, k string) (val map[string]string, err error)
// 	HMSet(ctx context.Context, k string, fields map[string]string, ttl time.Duration) (rs bool, err error)
// }

type StringData interface {
	CacheString() string
}

const EmptyString = "_empty_"

var ErrCacheTypeNotMatch = errors.New("cache data type not match")
var ErrEmptyData = errors.New("empty data")

type StringOption struct {
	EmptyTtl time.Duration
	// ? 空值, 默认
	IsEmptyCache func(v string) bool
}

func (s *StringOption) Apply(opt *StringOption) {
	if opt == nil {
		return
	}
	if opt.EmptyTtl > 0 {
		s.EmptyTtl = opt.EmptyTtl
	}
	if opt.IsEmptyCache != nil {
		s.IsEmptyCache = opt.IsEmptyCache
	}
}

var dftStringOption = func() *StringOption {
	return &StringOption{
		EmptyTtl: 0,
		IsEmptyCache: func(v string) bool {
			return v == EmptyString
		},
	}
}

type Result struct {
	err  error
	data interface{}
}

func (r *Result) Err() error {
	return r.err
}

func (r *Result) Decode(data interface{}) error {
	if r.err != nil {
		return r.err
	}
	if r.data == nil {
		return nil
	}
	if s, ok := r.data.(string); ok {
		return jsoniter.UnmarshalFromString(s, data)
	}

	return nil
}

func (r *Result) String() string {
	if r.data == nil {
		return ""
	}
	s, ok := r.data.(string)
	if !ok {
		return ""
	}
	return s
}

func (r *Result) Raw() interface{} {
	return r.data
}

func isEmptyErr(err error) bool {
	if err == nil {
		return true
	} else {
		return strings.Contains(err.Error(), "empty") || strings.Contains(err.Error(), "nil")
	}

}

func GetSetString(ctx context.Context, cli redis.UniversalClient, key string, ttl time.Duration, fn func(ctx context.Context) (data StringData, err error), option *StringOption) (rs *Result) {
	opt := dftStringOption()
	opt.Apply(option)
	rs = &Result{}

	s, err := cli.Get(ctx, key).Result()

	if !isEmptyErr(err) {
		rs.err = err
		return
	}

	if opt.IsEmptyCache(s) {
		return rs
	} else if s != "" {
		rs.data = s
		return rs
	} else {
		data, err1 := fn(ctx)
		if err1 != nil {
			rs.err = err1
			return
		}
		var val = data.CacheString()
		if val == "" {
			if opt.EmptyTtl > 0 {
				_, rs.err = cli.Set(ctx, key, EmptyString, opt.EmptyTtl).Result()
			}
		} else {
			_, rs.err = cli.Set(ctx, key, val, ttl).Result()
		}
		rs.data = val
	}

	return
}

/*GetSetString
* @Description: 以string类型存储
* @param ctx
* @param cli
* @param key
* @param ttl
* @param fn
* @param option
* @return rs
 */
func GetSetStringV2(ctx context.Context, cli cache.Rds, key string, ttl time.Duration, fn func(ctx context.Context) (data StringData, err error), option *StringOption) (rs *Result) {
	opt := dftStringOption()
	opt.Apply(option)
	rs = &Result{}

	cacheData, err := cli.Get(ctx, key)

	if !isEmptyErr(err) {
		rs.err = err
		return
	}

	s, ok := cacheData.(string)
	if !ok {
		rs.err = ErrCacheTypeNotMatch
		return
	}

	if opt.IsEmptyCache(s) {
		return rs
	} else if s != "" {
		rs.data = s
		return rs
	} else {
		data, err1 := fn(ctx)
		if err1 != nil {
			rs.err = err1
			return
		}
		var val = data.CacheString()
		if val == "" {
			if opt.EmptyTtl > 0 {
				_, rs.err = cli.Set(ctx, key, EmptyString, opt.EmptyTtl)
			}
		} else {
			_, rs.err = cli.Set(ctx, key, val, ttl)
		}
		rs.data = val
	}

	return
}

type HashOption struct {
	EmptyTtl     time.Duration
	IsEmptyCache func(data interface{}) bool
}

func (h *HashOption) Apply(opt *HashOption) {
	if opt == nil {
		return
	}
	if opt.EmptyTtl > 0 {
		h.EmptyTtl = opt.EmptyTtl
	}
	if opt.IsEmptyCache != nil {
		h.IsEmptyCache = opt.IsEmptyCache
	}
}

func dftHashOption() *HashOption {
	return &HashOption{
		EmptyTtl: 0,
		IsEmptyCache: func(data interface{}) bool {
			if data != nil {
				if v, ok := data.(map[string]string); ok {
					return v[EmptyString] == "1" && len(v) == 1
				}
			}

			return false
		},
	}
}

//type HashData interface {
//	MapStringInterface() map[string]interface{}
//}

type RawOption struct {
	EmptyTtl     time.Duration
	IsEmptyCache func(data interface{}) bool
}

func (h *RawOption) Apply(opt *RawOption) {
	if opt == nil {
		return
	}
	if opt.EmptyTtl > 0 {
		h.EmptyTtl = opt.EmptyTtl
	}
	if opt.IsEmptyCache != nil {
		h.IsEmptyCache = opt.IsEmptyCache
	}
}

func dftRawOption() *RawOption {
	return &RawOption{
		EmptyTtl: 0,
		IsEmptyCache: func(data interface{}) bool {
			if v, ok := data.(string); ok {
				if v == EmptyString {
					return true
				}
			}

			return false
		},
	}
}

// func GetSetRaw(ctx context.Context, goCli Cache, key string, ttl time.Duration, fn func(ctx context.Context) (i interface{}, err error), option *RawOption) (rs *Result) {
// 	opt := dftRawOption()
// 	opt.Apply(option)

// 	rs = &Result{}
// 	val, err := goCli.Get(ctx, key)
// 	if !isEmptyErr(err) {
// 		rs.err = err
// 		return
// 	}
// 	if opt.IsEmptyCache(val) {
// 		return rs
// 	} else if val != nil {
// 		rs.data = val
// 		return rs
// 	} else {
// 		data, err1 := fn(ctx)
// 		if err1 != nil {
// 			rs.err = err1
// 			return
// 		}
// 		if data == nil {
// 			if opt.EmptyTtl > 0 {
// 				_, rs.err = goCli.Set(ctx, key, EmptyString, opt.EmptyTtl)
// 			}
// 		} else {
// 			_, rs.err = goCli.Set(ctx, key, data, ttl)
// 		}
// 		rs.data = data
// 	}

// 	return rs
// }

type Item interface {
	CacheString() string
	//CacheItem() string
}

type StringRs struct {
	Items map[string]string
}

func (r *StringRs) Get(k string) string {
	if r.Items == nil {
		return ""
	}

	return r.Items[k]
}

func (r *StringRs) Decode(k string, data interface{}) error {
	d := r.Get(k)
	err := jsoniter.UnmarshalFromString(d, &data)

	return err
}

type PipeStringGetSetOption struct {
	GetCacheKey func(k string) string
	Ttl         time.Duration
	// 当 > 0时, 会进行防止穿透处理
	EmptyTtl time.Duration
	// 锁定条件
	LockOption *LockOption
	// 自定义ttl,  会调用该函数
	CustomerTtl func(k string, isEmpty bool) time.Duration
}

func (p *PipeStringGetSetOption) Apply(opt *PipeStringGetSetOption) {
	if opt == nil {
		return
	}
	if opt.GetCacheKey != nil {
		p.GetCacheKey = opt.GetCacheKey
	}

	if opt.Ttl > 0 {
		p.Ttl = opt.Ttl
	}

	if opt.EmptyTtl > 0 {
		p.EmptyTtl = opt.EmptyTtl
	}

	if opt.LockOption != nil {
		if opt.LockOption.LockFailedRetryInterval > 0 {
			p.LockOption.LockFailedRetryInterval = opt.LockOption.LockFailedRetryInterval
		}

		if opt.LockOption.MaxRetryTimes > 0 {
			p.LockOption.MaxRetryTimes = opt.LockOption.MaxRetryTimes
		}

		if opt.LockOption.LockTtl > 0 {
			p.LockOption.LockTtl = opt.LockOption.LockTtl
		}

		if opt.LockOption.LockKey != "" {
			p.LockOption.LockKey = opt.LockOption.LockKey
		}
	}

	if opt.CustomerTtl != nil {
		p.CustomerTtl = opt.CustomerTtl
	}
}

func getDftPipeStringGetSet() *PipeStringGetSetOption {
	return &PipeStringGetSetOption{
		GetCacheKey: func(k string) string {
			return k
		},
		Ttl: time.Minute * 30,
		LockOption: &LockOption{
			LockKey:                 "",
			LockTtl:                 0,
			LockFailedRetryInterval: time.Millisecond * 200,
			MaxRetryTimes:           0,
			curTimes:                0,
		},
	}
}

type BatchGetter func(ctx context.Context, keys []string) (items map[string]Item, err error)

func PipeStringGetSet(ctx context.Context, cli redis.UniversalClient, keys []string, batchGetter BatchGetter, option *PipeStringGetSetOption) (rs *StringRs, err error) {
	opt := getDftPipeStringGetSet()
	opt.Apply(option)

	if opt.LockOption.LockKey != "" && opt.LockOption.LockTtl > 0 {
		lockOk, _ := cli.SetNX(ctx, opt.LockOption.LockKey, 1, opt.LockOption.LockTtl).Result()
		if !lockOk {
			if opt.LockOption.MaxRetryTimes > 0 && opt.LockOption.curTimes < opt.LockOption.MaxRetryTimes {
				// 上锁失败, 进行重试
				opt.LockOption.curTimes++
				time.Sleep(opt.LockOption.LockFailedRetryInterval)
				return PipeStringGetSet(ctx, cli, keys, batchGetter, opt)
			} else {
				return nil, ErrFailedToGetLock
			}
		} else {
			// 上锁成功, 注册删除锁
			defer cli.Del(ctx, opt.LockOption.LockKey)
		}
	}

	pipe := cli.Pipeline()
	for i := range keys {
		cacheKey := opt.GetCacheKey(keys[i])
		pipe.Get(ctx, cacheKey)
	}

	cmder, err := pipe.Exec(ctx)

	if err != nil && err != redis.Nil {
		return nil, err
	}

	rs = &StringRs{
		Items: make(map[string]string, len(keys)),
	}

	var missKeys = make([]string, 0)

	for i := range cmder {
		cacheData, err1 := cmder[i].(*redis.StringCmd).Result()
		if err1 != nil && err1 != redis.Nil {
			return nil, cmder[i].Err()
		} else if cacheData == "" || err1 == redis.Nil {
			missKeys = append(missKeys, keys[i])
		} else if cacheData == EmptyString {
			// continue
		} else {
			rs.Items[keys[i]] = cacheData
		}
	}

	if len(missKeys) > 0 {
		dbItems, err := batchGetter(ctx, missKeys)
		if err != nil {
			return nil, err
		}
		nPipe := cli.Pipeline()

		for i := range missKeys {
			if dbItems[missKeys[i]] == nil {
				// empty
				if opt.EmptyTtl > 0 {
					emptyTtl := opt.EmptyTtl
					if opt.CustomerTtl != nil {
						emptyTtl = opt.CustomerTtl(missKeys[i], true)
					}
					nPipe.Set(ctx, opt.GetCacheKey(missKeys[i]), EmptyString, emptyTtl)
				}
				continue
			}
			cacheData := dbItems[missKeys[i]].CacheString()
			cacheKey := opt.GetCacheKey(missKeys[i])
			if cacheData == "" {
				if opt.EmptyTtl > 0 {
					emptyTtl := opt.EmptyTtl
					if opt.CustomerTtl != nil {
						emptyTtl = opt.CustomerTtl(missKeys[i], true)
					}
					nPipe.Set(ctx, cacheKey, EmptyString, emptyTtl)
				}
			} else {
				ttl := opt.Ttl
				if opt.CustomerTtl != nil {
					ttl = opt.CustomerTtl(missKeys[i], false)
				}
				nPipe.Set(ctx, cacheKey, cacheData, ttl)
				rs.Items[missKeys[i]] = cacheData
			}
		}

		_, err = nPipe.Exec(ctx)

		if err != nil {
			return nil, err
		}
	}

	return rs, nil
}

type LockOption struct {
	LockKey string
	// > 0 开启锁
	LockTtl time.Duration
	// 上锁失败, 重试间隔, 默认 200ms
	LockFailedRetryInterval time.Duration
	// 默认0, 一直重试
	MaxRetryTimes int
	// 当前重试次数
	curTimes int
}

type PipeHashGetSetOption struct {
	GetCacheKey func(k string) string
	Ttl         time.Duration
	// 当 > 0时, 会进行防止穿透处理
	EmptyTtl time.Duration
	// 指定需要获取的字段, 默认全部
	Fields  []string
	Decoder func(map[string]string) interface{}
	// 锁定条件
	LockOption *LockOption
}

/*HashItem
* @description 批量从redis中获取hash数据
 */
type HashItem interface {
	CacheMapStringString() map[string]string
}

func (p *PipeHashGetSetOption) Apply(opt *PipeHashGetSetOption) {
	if opt == nil {
		return
	}
	if opt.GetCacheKey != nil {
		p.GetCacheKey = opt.GetCacheKey
	}

	if opt.Ttl > 0 {
		p.Ttl = opt.Ttl
	}

	if opt.EmptyTtl > 0 {
		p.EmptyTtl = opt.EmptyTtl
	}

	if opt.Decoder != nil {
		p.Decoder = opt.Decoder
	}

	if opt.LockOption != nil {
		if opt.LockOption.LockFailedRetryInterval > 0 {
			p.LockOption.LockFailedRetryInterval = opt.LockOption.LockFailedRetryInterval
		}

		if opt.LockOption.MaxRetryTimes > 0 {
			p.LockOption.MaxRetryTimes = opt.LockOption.MaxRetryTimes
		}

		if opt.LockOption.LockTtl > 0 {
			p.LockOption.LockTtl = opt.LockOption.LockTtl
		}

		if opt.LockOption.LockKey != "" {
			p.LockOption.LockKey = opt.LockOption.LockKey
		}
	}
}

func getDftPipeHashGetSetOption() *PipeHashGetSetOption {
	return &PipeHashGetSetOption{
		GetCacheKey: func(k string) string {
			return k
		},
		Decoder: func(m map[string]string) interface{} {
			return m
		},
		Ttl: time.Minute * 30,
		LockOption: &LockOption{
			LockKey:                 "",
			LockTtl:                 0,
			LockFailedRetryInterval: time.Millisecond * 200,
			MaxRetryTimes:           0,
			curTimes:                0,
		},
	}
}

type HashRs struct {
	decoder func(map[string]string) interface{}
	items   map[string]map[string]string
}

func (h *HashRs) Items() map[string]map[string]string {
	return h.items
}

func (h *HashRs) Set(key string, data map[string]string) {
	if h.items == nil {
		h.items = make(map[string]map[string]string)
	}
	h.items[key] = data
}

func (h *HashRs) MSet(items map[string]map[string]string) {
	if h.items == nil {
		h.items = make(map[string]map[string]string)
	}
	for k, v := range items {
		h.items[k] = v
	}
}

func (h *HashRs) SetDecoder(fn func(m map[string]string) interface{}) {
	h.decoder = fn
}

func (h *HashRs) Get(key string) map[string]string {
	if h.items == nil {
		return nil
	}
	return h.items[key]
}

func (h *HashRs) Decode(key string) interface{} {
	if h.items == nil {
		return nil
	}
	if i, ok := h.items[key]; ok {
		return h.decoder(i)
	}

	return nil
}

/*CustDecode
* @Description: 自定义解析
* @param key
* @param decoder
* @return interface{}
 */
func (h *HashRs) CustDecode(key string, decoder func(data map[string]string) interface{}) interface{} {
	if h.items == nil {
		return nil
	}
	if i, ok := h.items[key]; ok {
		return decoder(i)
	}

	return nil
}

type BatchHashGetter func(ctx context.Context, keys []string) (items map[string]HashItem, err error)

var ErrFailedToGetLock = errors.New("failed to get lock")

func PipeHashGetSet(ctx context.Context, cli redis.UniversalClient, keys []string, batchGetter BatchHashGetter, option *PipeHashGetSetOption) (rs *HashRs, err error) {
	opt := getDftPipeHashGetSetOption()
	opt.Apply(option)

	if opt.LockOption.LockKey != "" && opt.LockOption.LockTtl > 0 {
		lockOk, _ := cli.SetNX(ctx, opt.LockOption.LockKey, 1, opt.LockOption.LockTtl).Result()
		if !lockOk {
			if opt.LockOption.MaxRetryTimes > 0 && opt.LockOption.curTimes < opt.LockOption.MaxRetryTimes {
				// 上锁失败, 进行重试
				opt.LockOption.curTimes++
				time.Sleep(opt.LockOption.LockFailedRetryInterval)
				return PipeHashGetSet(ctx, cli, keys, batchGetter, opt)
			} else {
				return nil, ErrFailedToGetLock
			}
		} else {
			// 上锁成功, 注册删除锁
			defer cli.Del(ctx, opt.LockOption.LockKey)
		}
	}

	pipe := cli.Pipeline()
	for i := range keys {
		if len(opt.Fields) > 0 {
			pipe.HMGet(ctx, opt.GetCacheKey(keys[i]), opt.Fields...)
		} else {
			pipe.HGetAll(ctx, opt.GetCacheKey(keys[i]))
		}
	}
	cmders, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, err
	}
	rs = &HashRs{
		decoder: opt.Decoder,
		items:   make(map[string]map[string]string, len(keys)),
	}

	var missKeys = make([]string, 0)
	for i := range cmders {
		if len(opt.Fields) > 0 {
			cacheData, err1 := cmders[i].(*redis.StringSliceCmd).Result()
			if err1 != nil && err1 != redis.Nil {
				return nil, cmders[i].Err()
			} else if len(cacheData) == 0 || err1 == redis.Nil {
				missKeys = append(missKeys, keys[i])
			} else if len(cacheData) <= 2 && cacheData[0] == EmptyString {
				continue
			} else {
				rs.items[keys[i]] = make(map[string]string, len(opt.Fields))

				for j := 0; j < len(cacheData); j += 2 {
					rs.items[keys[i]][cacheData[j]] = cacheData[j+1]
				}
			}
		} else {
			cacheData, err1 := cmders[i].(*redis.StringStringMapCmd).Result()
			if err1 != nil && err1 != redis.Nil {
				return nil, cmders[i].Err()
			} else if len(cacheData) == 0 || err1 == redis.Nil {
				missKeys = append(missKeys, keys[i])
			} else if len(cacheData) == 1 && cacheData[EmptyString] == EmptyString {
				continue
			} else {
				rs.items[keys[i]] = cacheData
			}
		}

	}
	if len(missKeys) > 0 {
		dbItemRef, err := batchGetter(ctx, missKeys)
		if err != nil {
			return rs, err
		}
		nPipe := cli.Pipeline()

		for i := range missKeys {
			if dbItemRef[missKeys[i]] == nil {
				// empty
				if opt.EmptyTtl > 0 {
					nPipe.HSet(ctx, opt.GetCacheKey(missKeys[i]), EmptyString, EmptyString)
					nPipe.Expire(ctx, opt.GetCacheKey(missKeys[i]), opt.EmptyTtl)
				}
				continue
			} else {
				cacheData := dbItemRef[missKeys[i]].CacheMapStringString()
				nPipe.HMSet(ctx, opt.GetCacheKey(missKeys[i]), cacheData)
				if opt.Ttl > 0 {
					nPipe.Expire(ctx, opt.GetCacheKey(missKeys[i]), opt.Ttl)
				}
				rs.items[missKeys[i]] = cacheData
			}
		}

		_, err = nPipe.Exec(ctx)
		if err != nil {
			return rs, err
		}
	}

	return rs, err
}

func PipeHashGetSetV2(ctx context.Context, rds cache.Rds, keys []string, batchGetter BatchHashGetter, option *PipeHashGetSetOption) (rs *HashRs, err error) {
	opt := getDftPipeHashGetSetOption()
	opt.Apply(option)

	if opt.LockOption.LockKey != "" && opt.LockOption.LockTtl > 0 {
		lockOk, _ := rds.Set(ctx, opt.LockOption.LockKey, 1, opt.LockOption.LockTtl, &cache.StringSetOption{
			Mode:      "NX",
			ExtendTtl: false,
		})
		if !lockOk {
			if opt.LockOption.MaxRetryTimes > 0 && opt.LockOption.curTimes < opt.LockOption.MaxRetryTimes {
				// 上锁失败, 进行重试
				opt.LockOption.curTimes++
				time.Sleep(opt.LockOption.LockFailedRetryInterval)
				return PipeHashGetSetV2(ctx, rds, keys, batchGetter, opt)
			} else {
				return nil, ErrFailedToGetLock
			}
		} else {
			// 上锁成功, 注册删除锁
			defer rds.Del(ctx, opt.LockOption.LockKey)
		}
	}

	pipe := rds.Pipeline()
	for i := range keys {
		if len(opt.Fields) > 0 {
			pipe.HMGet(ctx, opt.GetCacheKey(keys[i]), opt.Fields...)
		} else {
			pipe.HGetAll(ctx, opt.GetCacheKey(keys[i]))
		}
	}
	cmders, err := pipe.Exec(ctx)
	if err != nil && !IsNilError(err) {
		return nil, err
	}
	rs = &HashRs{
		decoder: opt.Decoder,
		items:   make(map[string]map[string]string, len(keys)),
	}

	var missKeys = make([]string, 0)
	for i := range cmders {
		if len(opt.Fields) > 0 {
			cacheData, err1 := cmders[i].(cache.StringSliceCmd).Result()
			if err1 != nil && err1 != redis.Nil {
				return nil, cmders[i].Err()
			} else if len(cacheData) == 0 || err1 == redis.Nil {
				missKeys = append(missKeys, keys[i])
			} else if len(cacheData) == 0 && cacheData[0] == EmptyString {
				continue
			} else {
				rs.items[keys[i]] = make(map[string]string, len(opt.Fields))

				for j := 0; j < len(cacheData); j += 2 {
					rs.items[keys[i]][cacheData[j]] = cacheData[j+1]
				}
			}
		} else {
			cacheData, err1 := cmders[i].(cache.StringStringMapCmd).Result()
			if err1 != nil && err1 != redis.Nil {
				return nil, cmders[i].Err()
			} else if len(cacheData) == 0 || err1 == redis.Nil {
				missKeys = append(missKeys, keys[i])
			} else if len(cacheData) == 1 && cacheData[EmptyString] == EmptyString {
				continue
			} else {
				rs.items[keys[i]] = cacheData
			}
		}

	}
	if len(missKeys) > 0 {
		dbItemRef, err := batchGetter(ctx, missKeys)
		if err != nil {
			return rs, err
		}
		nPipe := rds.Pipeline()

		for i := range missKeys {
			if dbItemRef[missKeys[i]] == nil {
				// empty
				if opt.EmptyTtl > 0 {
					nPipe.HSet(ctx, opt.GetCacheKey(missKeys[i]), EmptyString, EmptyString)
					nPipe.Expire(ctx, opt.GetCacheKey(missKeys[i]), opt.EmptyTtl)
				}
				continue
			} else {
				cacheData := dbItemRef[missKeys[i]].CacheMapStringString()
				nPipe.HMSet(ctx, opt.GetCacheKey(missKeys[i]), cacheData)
				if opt.Ttl > 0 {
					nPipe.Expire(ctx, opt.GetCacheKey(missKeys[i]), opt.Ttl)
				}
				rs.items[missKeys[i]] = cacheData
			}
		}

		_, err = nPipe.Exec(ctx)
		if err != nil {
			return rs, err
		}
	}

	return rs, err
}

func PipeStringGetSetV2(ctx context.Context, rds cache.Rds, keys []string, batchGetter BatchGetter, option *PipeStringGetSetOption) (rs *StringRs, err error) {
	opt := getDftPipeStringGetSet()
	opt.Apply(option)

	if opt.LockOption.LockKey != "" && opt.LockOption.LockTtl > 0 {
		lockOk, _ := rds.Set(ctx, opt.LockOption.LockKey, 1, opt.LockOption.LockTtl, &cache.StringSetOption{
			Mode:      "NX",
			ExtendTtl: false,
		})
		if !lockOk {
			if opt.LockOption.MaxRetryTimes > 0 && opt.LockOption.curTimes < opt.LockOption.MaxRetryTimes {
				// 上锁失败, 进行重试
				opt.LockOption.curTimes++
				time.Sleep(opt.LockOption.LockFailedRetryInterval)
				return PipeStringGetSetV2(ctx, rds, keys, batchGetter, opt)
			} else {
				return nil, ErrFailedToGetLock
			}
		} else {
			// 上锁成功, 注册删除锁
			defer rds.Del(ctx, opt.LockOption.LockKey)
		}
	}

	pipe := rds.Pipeline()
	for i := range keys {
		cacheKey := opt.GetCacheKey(keys[i])
		pipe.Get(ctx, cacheKey)
	}

	cmder, err := pipe.Exec(ctx)

	if err != nil && !IsNilError(err) {
		return nil, err
	}

	rs = &StringRs{
		Items: make(map[string]string, len(keys)),
	}

	var missKeys = make([]string, 0)

	for i := range cmder {
		cacheData, err1 := cmder[i].(cache.StringCmd).Result()
		if err1 != nil && !IsNilError(err1) {
			return nil, cmder[i].Err()
		} else if cacheData == "" || IsNilError(err1) {
			missKeys = append(missKeys, keys[i])
		} else if cacheData == EmptyString {
			// continue
		} else {
			rs.Items[keys[i]] = cacheData
		}
	}

	if len(missKeys) > 0 {
		dbItems, err := batchGetter(ctx, missKeys)
		if err != nil {
			return nil, err
		}
		nPipe := rds.Pipeline()

		for i := range missKeys {
			if dbItems[missKeys[i]] == nil {
				// empty
				if opt.EmptyTtl > 0 {
					emptyTtl := opt.EmptyTtl
					if opt.CustomerTtl != nil {
						emptyTtl = opt.CustomerTtl(missKeys[i], true)
					}
					nPipe.Set(ctx, opt.GetCacheKey(missKeys[i]), EmptyString, emptyTtl)
				}
				continue
			}
			cacheData := dbItems[missKeys[i]].CacheString()
			cacheKey := opt.GetCacheKey(missKeys[i])
			if cacheData == "" {
				if opt.EmptyTtl > 0 {
					emptyTtl := opt.EmptyTtl
					if opt.CustomerTtl != nil {
						emptyTtl = opt.CustomerTtl(missKeys[i], true)
					}
					nPipe.Set(ctx, cacheKey, EmptyString, emptyTtl)
				}
			} else {
				ttl := opt.Ttl
				if opt.CustomerTtl != nil {
					ttl = opt.CustomerTtl(missKeys[i], false)
				}
				nPipe.Set(ctx, cacheKey, cacheData, ttl)
				rs.Items[missKeys[i]] = cacheData
			}
		}

		_, err = nPipe.Exec(ctx)

		if err != nil {
			return nil, err
		}
	}

	return rs, nil
}

func IsNilError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "redis: nil")
}
