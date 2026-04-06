package es

import (
	"fmt"
	"strings"

	jsoniter "github.com/json-iterator/go"
)

type QueryString struct{}

func (t *QueryString) Build(items []*WhereItem, shouldItems [][]*WhereItem) string {
	query := ""
	//itemsLen := len(items) - 1
	for i := range items {
		d := ","
		if i == 0 || items[i].Condition == "_where_" {
			d = ""
		}
		buildHandle := GetBuildHandle(items[i].Condition)

		itemQuery := buildHandle.Build(items[i])
		query += fmt.Sprintf(`%s%s`, d, itemQuery)
	}

	// build should
	if shouldItems != nil && len(shouldItems) > 0 {
		siLen := len(shouldItems) - 1
		d1 := ","
		var minShouldMatch int = 1
		if shouldItems[0][0] != nil {
			if shouldItems[0][0].Opt != nil && shouldItems[0][0].Opt.MinimumShouldMatch != nil {
				minShouldMatch = *shouldItems[0][0].Opt.MinimumShouldMatch
			}
		}

		for i1, ites := range shouldItems {
			if siLen == i1 {
				d1 = ""
			}

			should := ""
			itesLen := len(ites) - 1
			for i, item := range ites {
				d := ","
				if itesLen == i {
					d = ""
				}
				buildHandle := GetBuildHandle(item.Condition)
				itemQuery := buildHandle.Build(item)
				should += fmt.Sprintf(`%s%s`, itemQuery, d)
			}
			if strings.HasSuffix(query, "}") || strings.HasPrefix(query, "]") {
				query += ","
			}
			if minShouldMatch > 0 {
				query += fmt.Sprintf(`{"bool":{"should":[%s], "minimum_should_match":1}}%s`, should, d1)
			} else {
				query += fmt.Sprintf(`{"bool":{"should":[%s]}}%s`, should, d1)
			}
		}
	}

	filter := fmt.Sprintf(`[%s]`, query)
	return filter
}

func (t *QueryString) BuildSort(sort []*EsSort) string {
	if len(sort) == 0 {
		return ""
	}

	s, d := "", ","
	sLen := len(sort) - 1
	for i, esSort := range sort {
		if i == sLen {
			d = ""
		}
		if esSort.Missing != "" {
			s += fmt.Sprintf(`{"%s":{"order":"%s", "missing":"%s"}}%s`, esSort.Field, esSort.Order, esSort.Missing, d)
		} else {
			s += fmt.Sprintf(`{"%s":{"order":"%s"}}%s`, esSort.Field, esSort.Order, d)
		}
	}

	sortStr := fmt.Sprintf(`,"sort":[%s]`, s)
	return sortStr
}

func (t *QueryString) BuildPaging(from, size int, sizeZero bool) string {
	if !sizeZero && from == 0 && size == 0 {
		return ""
	}

	s := ""
	if sizeZero {
		s = fmt.Sprintf(`,"size":%d`, size)
	} else {
		s = fmt.Sprintf(`,"from":%d, "size":%d`, from, size)
	}
	return s
}

func (t *QueryString) BuildSearchAfter(val []interface{}) string {
	if len(val) == 0 {
		return ""
	}

	v, _ := jsoniter.MarshalToString(val)
	return fmt.Sprintf(`,"search_after":%v`, v)
}

func (t *QueryString) BuildGroupBy(groupBy *GroupBy, str string) string {
	if str != "" {
		return str
	}
	if groupBy == nil {
		return ""
	}
	return fmt.Sprintf(`,"aggs":{"categories":{"terms":{"field":"%s","size":%d},"aggs":{"top_products":{"top_hits":{"size":%d}}}}}`, groupBy.Field, groupBy.GroupNum, groupBy.ItemNum)
}

func (t *QueryString) BuildAggs(aggs *Aggs) string {
	if aggs == nil {
		return ""
	}
	agg := fmt.Sprintf(`{"%s":{"%s":{"field":"%s"}}}`, aggs.Name, aggs.Type, aggs.Field)
	return fmt.Sprintf(`,"aggs":%s`, agg)
}

func (t *QueryString) BuildRuntimeMappings(mappings string) string {
	if mappings == "" {
		return ""
	}
	return `,"runtime_mappings":` + mappings
}
