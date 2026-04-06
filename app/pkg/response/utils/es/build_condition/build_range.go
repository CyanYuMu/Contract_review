package build_condition

import (
	"fmt"
	"reflect"
	"sync"
)

var rangeObj *Range
var rangeOnce sync.Once

var simpleWhereObj *SimpleWhere
var simpleWhereOnce sync.Once

func NewSimpleWhere() *SimpleWhere {
	simpleWhereOnce.Do(func() {
		simpleWhereObj = &SimpleWhere{}
	})
	return simpleWhereObj
}

func NewRange() *Range {
	rangeOnce.Do(func() {
		rangeObj = &Range{}
	})
	return rangeObj
}

type SimpleWhere struct {
}

func (w *SimpleWhere) Build(item *WhereItem) string {
	return item.Val.(string)
}

type Range struct{}

var rangeKey = map[string]string{
	">":  "gt",
	">=": "gte",
	"<":  "lt",
	"<=": "lte",
}

func (t *Range) Build(item *WhereItem) string {
	//var valInt int64
	//var valArr []int64
	//
	//if v, ok := item.Val.(int64); ok {
	//	valInt = v
	//} else if v, ok := item.Val.(int32); ok {
	//	valInt = int64(v)
	//} else if v, ok := item.Val.(int8); ok {
	//	valInt = int64(v)
	//} else if v, ok := item.Val.(float32); ok {
	//	valInt = int64(v)
	//} else if v, ok := item.Val.(float64); ok {
	//	valInt = int64(v)
	//} else if v, ok := item.Val.([]int64); ok {
	//	valArr = v
	//} else if v, ok := item.Val.(int); ok {
	//	valInt = int64(v)
	//} else if v, ok := item.Val.([]int); ok {
	//	valArr = []int64{}
	//	for _, v1 := range v {
	//		valArr = append(valArr, int64(v1))
	//	}
	//}
	//
	//rangeStr := ""
	//if len(valArr) == 2 {
	//	rangeStr = fmt.Sprintf(`{"range":{"%s":{"gte":%v, "lte":%v}}}`, item.Field, valArr[0], valArr[1])
	//} else {
	//	k := rangeKey[item.Condition]
	//	rangeStr = fmt.Sprintf(`{"range":{"%s":{"%s":%v}}}`, item.Field, k, valInt)
	//}

	rangeStr := ""
	if reflect.TypeOf(item.Val).Kind() == reflect.Slice {
		arr := item.Val.([]any)
		if item.Condition == "left_between" {
			// 左开右闭
			rangeStr = fmt.Sprintf(`{"range":{"%s":{"gt":%v, "lte":%v}}}`, item.Field, arr[0], arr[1])
		} else if item.Condition == "right_between" {
			// 左闭右开
			rangeStr = fmt.Sprintf(`{"range":{"%s":{"gte":%v, "lt":%v}}}`, item.Field, arr[0], arr[1])
		} else {
			rangeStr = fmt.Sprintf(`{"range":{"%s":{"gte":%v, "lte":%v}}}`, item.Field, arr[0], arr[1])
		}
	} else {
		k := rangeKey[item.Condition]
		rangeStr = fmt.Sprintf(`{"range":{"%s":{"%s":%v}}}`, item.Field, k, item.Val)
	}

	return rangeStr
}
