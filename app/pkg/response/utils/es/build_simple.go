package es

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
	"=":          "term",
	"==":         "term",
	"!=":         "term",
	"in":         "terms",
	"not_in":     "terms",
	"match":      "match",
	"like":       "wildcard",
	"not_exists": "exists",
	"exists":     "exists",
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

	itemVal := map[string]interface{}{
		item.Field: item.Val,
	}
	itemValString, _ := json.Marshal(itemVal)
	return fmt.Sprintf(`{"%s":%s}`, key, itemValString)
}
