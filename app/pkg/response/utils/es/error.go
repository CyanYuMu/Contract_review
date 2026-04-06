package es

import (
	"strings"

	"github.com/tidwall/gjson"
)

type FailedItem struct {
	Key   string
	Error string
}

type BulkError struct {
	Items []FailedItem
}

func (b *BulkError) IsAllMiss() bool {
	if b == nil {
		return false
	}
	for i := range b.Items {
		if !strings.Contains(b.Items[i].Error, "missing") {
			return false
		}
	}

	return true
}

func (b *BulkError) String() string {
	if b == nil {
		return ""
	}
	s := strings.Builder{}
	for i := range b.Items {
		s.WriteString(b.Items[i].Key)
		s.WriteString(": ")
		s.WriteString(b.Items[i].Error)
		s.WriteString("  ")
	}

	return s.String()
}

// 获取批量操作失败的错误信息
func GetBulkErrorMsg(s string) (rs *BulkError) {
	if strings.HasPrefix(s, "[") {
		s = s[strings.Index(s, "{"):]
	}
	rs = &BulkError{}
	if !gjson.Get(s, "errors").Bool() {
		return nil
	}
	gjson.Get(s, "items").ForEach(func(_, item gjson.Result) bool {
		item.ForEach(func(key, value gjson.Result) bool {
			if value.Get("error").Exists() {
				//fmt.Printf("Operation %s failed: %v\n", key.String(), value.Get("error").String())
				rs.Items = append(rs.Items, FailedItem{
					Key:   key.String(),
					Error: value.Get("error").String(),
				})
			}
			return true // continue iterating
		})
		return true // continue iterating
	})

	return rs
}
