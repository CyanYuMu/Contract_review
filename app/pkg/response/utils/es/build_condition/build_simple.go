package build_condition

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
)

var simpleOnce sync.Once
var simple *Simple

func NewSimple() *Simple {
	simpleOnce.Do(func() {
		simple = &Simple{}
	})
	return simple
}

type Simple struct{}

var queryKeyMap = map[string]string{
	"=":            "term",
	"==":           "term",
	"!=":           "term",
	"in":           "terms",
	"not_in":       "terms",
	"match":        "match",
	"match_phrase": "match_phrase",
	"like":         "wildcard",
	"not_exists":   "exists",
	"exists":       "exists",
}

func (s Simple) Build(item *WhereItem) string {
	key := queryKeyMap[item.Condition]

	// Handle in/not_in with single value using reflection
	if item.Condition == "in" || item.Condition == "not_in" {
		if v := reflect.ValueOf(item.Val); v.Kind() == reflect.Slice && v.Len() == 1 {
			key = "term"
			item.Condition = "="
			item.Val = v.Index(0).Interface()
		}
	}

	// 构建字段值对象
	var fieldVal interface{}

	// 如果设置了 boost 参数，需要将字段值包装成包含 boost 的对象
	if item.Boost != nil {
		// 对于 term 查询，需要特殊处理 boost
		if key == "term" {
			fieldVal = map[string]interface{}{
				"value": item.Val,
				"boost": *item.Boost,
			}
		} else {
			// 对于其他查询类型，boost 直接添加到字段值中
			fieldVal = map[string]interface{}{
				item.Field: item.Val,
				"boost":    *item.Boost,
			}
		}
	} else {
		fieldVal = item.Val
	}

	// 构建最终的查询对象
	var itemVal map[string]interface{}
	if item.Boost != nil && key == "term" {
		// term 查询的 boost 已经在 fieldVal 中处理
		itemVal = map[string]interface{}{
			item.Field: fieldVal,
		}
	} else if item.Boost != nil {
		// 其他查询类型的 boost 已经在 fieldVal 中处理
		itemVal = fieldVal.(map[string]interface{})
	} else {
		// 没有 boost 的情况
		itemVal = map[string]interface{}{
			item.Field: item.Val,
		}
	}

	itemValString, _ := json.Marshal(itemVal)
	return fmt.Sprintf(`{"%s":%s}`, key, itemValString)
}
