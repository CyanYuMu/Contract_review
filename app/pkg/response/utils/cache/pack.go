package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	jsoniter "github.com/json-iterator/go"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/mapping"
	error2 "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/error"
)

var ErrorNil = errors.New("cache: nil")
var ErrorEmpty = errors.New("cache: empty")

// 存储类型
type StoreType string
type MapType int

const (
	MapTypeStringString    MapType = 1
	MapTypeStringInterface MapType = 2
	// 不确定的数据类型, 会进行自动转换
	MapTypeGuess MapType = 3
)

const (
	// 默认
	StoreTypeString StoreType = "string"
	StoreTypeHash   StoreType = "hash"
	// 原始数据, 仅适用于gocache模式
	StoreTypeGoCacheRaw StoreType = "gocache_raw"
)

const EmptyStrValue = "_empty_"

type IsEmpty func(data interface{}) bool
type pack struct {
	inst Cache
	// 可选, 不指定则基于EmpStrValue进行判断
	isEmpty   IsEmpty
	emptyTTL  time.Duration
	storeType StoreType
	custom    *Handler
}

// Pack
// @description 获取实例
// inst - 缓存实例
func Pack(inst Cache) *pack {
	return &pack{
		inst: inst,
	}
}

type Handler struct {
	// 自定义获取数据的方式
	Get func(cache Cache, key string) (data interface{}, err error)
	// 自定义设置数据的方式
	Set func(cache Cache, cacheKey string, srcData interface{}, ttl time.Duration) (cacheData interface{}, err error)
}

// Custom
// @description 自定义缓存获取/更新方式
func (p *pack) Custom(handler *Handler) *pack {
	p.custom = handler

	return p
}

// Empty
// isEmpty 可选, 手动指定空判断逻辑
// @description 查询不到数据的情况下赋空值, 如果不指定不会有赋空值的操作
func (p *pack) Empty(ttl time.Duration, isEmpty ...IsEmpty) *pack {
	p.emptyTTL = ttl
	if len(isEmpty) > 0 {
		p.isEmpty = isEmpty[0]
	}

	return p
}

// StoreType
// @description 指定数据底层存储格式
func (p *pack) StoreType(st StoreType) *pack {
	p.storeType = st

	return p
}

func (p *pack) stringHandler(key string, ttl time.Duration, getFunc func() (interface{}, error)) (cacheStr string, err error) {
	cacheData, err := p.inst.Get(key)

	if err != nil {
		// 不能使用 redis.Nil 来判断空, 与 redis v8 类型不一致
		if err.Error() == "redis: nil" {
			err = nil
			cacheStr = ""
		} else {
			return
		}
	}
	var ok bool
	if cacheData != nil {
		cacheStr, ok = cacheData.(string)
		if !ok {
			return "", error2.NewSUError(500401, fmt.Sprintf("数据类型转换失败 %+v", cacheData))
		}
	}

	// 判断是否为空
	if cacheStr == "" {
		// 基于getFunc获取
		data, err := getFunc()
		if err != nil {
			return "", err
		}
		cacheStr, err = Data2String(data)
		if err != nil {
			return "", err
		}

		if cacheStr == "" && p.emptyTTL > 0 {
			cacheStr = EmptyStrValue
			ttl = p.emptyTTL
		}

		_, err = p.inst.Set(key, cacheStr, ttl)
		if err != nil {
			return "", err
		}
	}

	if cacheStr == EmptyStrValue {
		return "", nil
	} else {
		return cacheStr, nil
	}
}

func (p *pack) goCacheHandler(key string, ttl time.Duration, getFunc func() (interface{}, error)) (rs interface{}, err error) {
	cacheData, err := p.inst.Get(key)
	if err != nil {
		return nil, err
	}

	if cacheData == nil {
		dbData, err := getFunc()
		if err != nil {
			return nil, err
		}

		if isNilInterface(dbData) {
			// 空值情况, 按照空值策略进行赋值
			if p.emptyTTL > 0 {
				cacheData = EmptyStrValue
				ttl = p.emptyTTL
			} else {
				return nil, err
			}
		}

		_, err = p.inst.Set(key, dbData, ttl)

		return dbData, err
	} else {
		// 判断是否为空值
		if cacheStr, ok := cacheData.(string); ok {
			if cacheStr == EmptyStrValue {
				return nil, nil
			}
		}

		return cacheData, nil
	}
}

func (p *pack) customHandler(key string, ttl time.Duration, getFunc func() (interface{}, error), handler *Handler) (cacheData interface{}, t MapType, err error) {
	cacheData, err = handler.Get(p.inst, key)
	t = MapTypeGuess
	if err != nil {
		if err.Error() == "redis: nil" {
			err = nil
		} else {
			return nil, t, err
		}
	}
	if isNilInterface(cacheData) {
		var srcData interface{}
		srcData, err = getFunc()
		if err != nil {
			return nil, t, err
		}
		if isNilInterface(srcData) {
			// 空值情况, 按照空值策略进行赋值
			if p.emptyTTL > 0 {
				//cacheData = EmptyStrValue
				ttl = p.emptyTTL
			} else {
				return nil, t, ErrorNil
			}
		}
		cacheData, err = handler.Set(p.inst, key, srcData, ttl)
	}
	// 基于getFunc获取
	if err != nil {
		return
	}

	if p.determineEmpty(cacheData) {
		return nil, t, ErrorEmpty
	}

	return
}

func (p *pack) determineEmpty(v interface{}) bool {
	if p.isEmpty != nil {
		return p.isEmpty(v)
	} else {
		switch nv := v.(type) {
		case string:
			return nv == EmptyStrValue
		case bool, float64, float32, int, int32, int64, uint, uint32, uint64:
			return false
		case map[string]string:
			if _, exists := nv[EmptyStrValue]; exists {
				return true
			}
			return false
		case map[string]interface{}:
			if _, exists := nv[EmptyStrValue]; exists {
				return true
			}
			return false
		default:
			tmpS, _ := jsoniter.MarshalToString(v)
			if strings.Contains(tmpS, EmptyStrValue) {
				return true
			}

			return false
		}
	}
}

func (p *pack) hashHandler(key string, ttl time.Duration, getFunc func() (interface{}, error)) (rs interface{}, t MapType, err error) {
	// hgetall 读出来的数据类型是 map[string]string
	cacheData, err := p.inst.HGetAll(key)
	if err != nil {
		if err.Error() == "redis: nil" {
			err = nil
		} else {
			return nil, MapTypeStringString, err
		}
	}

	if len(cacheData) == 0 {
		var newCacheData map[string]interface{}
		data, err := getFunc()
		if err != nil {
			return nil, MapTypeStringInterface, err
		}
		newCacheData, err = Data2Map(data)
		if err != nil {
			return nil, MapTypeStringInterface, err
		}
		var isEmpty bool
		// 空值情况下, 如果未开启赋空值, 则直接跳过
		if isNilInterface(newCacheData) || len(newCacheData) == 0 {
			if p.emptyTTL > 0 {
				isEmpty = true
				ttl = p.emptyTTL
				newCacheData = map[string]interface{}{EmptyStrValue: "x"}
			} else {
				return nil, MapTypeStringInterface, err
			}
		}

		if err = p.inst.HMSet(key, newCacheData); err != nil {
			return nil, MapTypeStringInterface, err
		}

		if ttl > 0 {
			p.inst.Expire(key, ttl)
		}

		if isEmpty {
			return nil, MapTypeStringInterface, nil
		} else {
			return newCacheData, MapTypeStringInterface, nil
		}
	} else {
		// 空值判断
		if _, exists := cacheData[EmptyStrValue]; exists {
			if len(cacheData) == 1 {
				return nil, MapTypeStringString, nil
			}
		}

		return cacheData, MapTypeStringString, nil
	}
}

type GetSetHandler = func() (interface{}, error)

// GetSet
// @description 最终的查询
func (p *pack) GetSet(key string, ttl time.Duration, getFunc GetSetHandler) (rs *Result, err error) {
	rs = &Result{
		Type: p.storeType,
	}
	if p.custom != nil {
		rs.Val, rs.mapType, err = p.customHandler(key, ttl, getFunc, p.custom)
		return
	} else if p.storeType == "" || p.storeType == StoreTypeString {
		rs.Val, err = p.stringHandler(key, ttl, getFunc)
		return
	} else if p.storeType == StoreTypeGoCacheRaw {
		rs.Val, err = p.goCacheHandler(key, ttl, getFunc)
		return
	} else {
		rs.Val, rs.mapType, err = p.hashHandler(key, ttl, getFunc)
		return
	}
}

type Result struct {
	Type    StoreType
	Val     interface{}
	mapType MapType
}

func (r *Result) Unmarshal(data interface{}) error {
	if r.Type == StoreTypeString {
		err := jsoniter.UnmarshalFromString(r.Val.(string), data)

		return err
	} else {
		// 这里有个注意点, redis 的HGetAll返回的数据是map[string]string, hGet 返回的同样是string类型, 所以这里需要挨个字段进行转换
		if isNilInterface(r.Val) {
			return ErrorNil
		}
		if r.mapType == MapTypeStringString {
			return mapping.StringString2Struct(r.Val.(map[string]string), data)
		} else {
			return mapping.StringInterface2Struct(r.Val.(map[string]interface{}), data)
		}
	}
}

func (r *Result) String() string {
	if r.Type == StoreTypeString {
		if v, ok := r.Val.(string); ok {
			return v
		}

		return ""
	} else {
		byteData, err := jsoniter.Marshal(r.Val)
		if err != nil {
			return ""
		}

		return string(byteData)
	}
}

func (r *Result) Raw() interface{} {
	return r.Val
}

func Data2String(data interface{}) (string, error) {
	if data == nil {
		return "", ErrorNil
	}
	rt := reflect.TypeOf(data)
	switch rt.Kind() {
	case reflect.String:
		return data.(string), nil
	case reflect.Map, reflect.Slice, reflect.Struct, reflect.Ptr:
		byteData, err := jsoniter.Marshal(data)
		return string(byteData), err
	default:
		return "", error2.NewSUError(500401, "不支持的数据类型 kind:"+rt.Kind().String())
	}
}

func isNilInterface(data interface{}) bool {
	if data == nil {
		return true
	} else {
		rt := reflect.TypeOf(data)
		switch rt.Kind() {
		case reflect.Ptr, reflect.Map, reflect.Slice:
			return reflect.ValueOf(data).IsNil()
		default:
			return false
		}
	}
}

func Data2Map(data interface{}) (mapData map[string]interface{}, err error) {
	if isNilInterface(data) {
		return nil, err
	}
	rt := reflect.TypeOf(data)
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}
	switch rt.Kind() {
	case reflect.String:
		err = json.Unmarshal([]byte(data.(string)), &mapData)
		return
	case reflect.Struct:
		byteData, err := jsoniter.Marshal(data)
		if err != nil {
			return nil, err
		}

		err = jsoniter.Unmarshal(byteData, &mapData)
		return mapData, err
	case reflect.Map:
		mapData, ok := data.(map[string]interface{})
		if ok {
			return mapData, nil
		} else {
			if tmpData, ok := data.(map[string]string); ok {
				mapData = make(map[string]interface{}, len(tmpData))
				for k, v := range tmpData {
					mapData[k] = v
				}
			}
		}

		return mapData, nil
	default:
		return nil, error2.NewSUError(500401, "不支持的数据类型 kind:"+rt.Kind().String())
	}
}
