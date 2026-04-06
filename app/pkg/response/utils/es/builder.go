package es

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	es "github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	jsoniter "github.com/json-iterator/go"
	"github.com/tidwall/gjson"

	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/es/build_condition"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
)

func NewBuilder(es *es.Client) *Builder {
	return &Builder{
		buildQuery: &build_condition.QueryString{},
		es:         es,
	}
}

type BuildInterface interface {
	Build(item *WhereItem) string
}

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
	var rangeStr string
	if reflect.TypeOf(item.Val).Kind() == reflect.Slice {
		arr := item.Val.([]any)
		if item.Condition == "left_between" {
			// 左开右闭
			rangeStr = fmt.Sprintf(`{"range":{"%s":{"gt":%v, "lte":%v`, item.Field, arr[0], arr[1])
		} else if item.Condition == "right_between" {
			// 左闭右开
			rangeStr = fmt.Sprintf(`{"range":{"%s":{"gte":%v, "lt":%v`, item.Field, arr[0], arr[1])
		} else {
			rangeStr = fmt.Sprintf(`{"range":{"%s":{"gte":%v, "lte":%v`, item.Field, arr[0], arr[1])
		}
	} else {
		k := rangeKey[item.Condition]
		rangeStr = fmt.Sprintf(`{"range":{"%s":{"%s":%v`, item.Field, k, item.Val)
	}

	// 如果设置了 boost 参数，添加到查询中
	if item.Boost != nil {
		rangeStr += fmt.Sprintf(`, "boost":%v`, *item.Boost)
	}

	rangeStr += "}}"
	return rangeStr
}

func GetBuildHandle(condition string) BuildInterface {
	switch condition {
	case "=", "==", "!=", "in", "not_in", "match", "match_phrase", "like", "not_exists":
		return NewSimple()
	case "between", ">", ">=", "<", "<=", "left_between", "right_between":
		return NewRange()
	case "_where_":
		return NewSimpleWhere()
	}

	return nil
}

type Builder struct {
	query           string
	buildQuery      *build_condition.QueryString
	filter          []*build_condition.WhereItem
	must            []*build_condition.WhereItem
	mustNot         []*build_condition.WhereItem
	should          [][]*build_condition.WhereItem
	sort            []*build_condition.EsSort
	Group           *build_condition.GroupBy
	GroupStr        string
	Aggs            *build_condition.Aggs
	from            int
	size            int
	sizeZero        bool
	searchAfter     []interface{}
	withTotal       bool
	withSum         string
	index           []string
	source          []string
	runtimeMappings string
	es              *es.Client
}

/**
 * @Description: es查询条件
 * 支持 =, >, >=, <, <=, !=, between, in, not_in, like, match, match_phrase
 */
func (o *Builder) Where(field, condition string, val interface{}, boost ...float64) {
	var boostVal *float64
	if len(boost) > 0 {
		boostVal = &boost[0]
	}

	switch condition {
	// filter过滤条件
	case "=", "==", "between", "in", "left_between", "right_between":
		o.filter = append(o.filter, &build_condition.WhereItem{
			Field:     field,
			Condition: condition,
			Val:       val,
			Boost:     boostVal,
		})

	// must查询条件
	case "match", "match_phrase", "like", ">", ">=", "<", "<=":
		o.must = append(o.must, &build_condition.WhereItem{
			Field:     field,
			Condition: condition,
			Val:       val,
			Boost:     boostVal,
		})
	// must not 查询条件
	case "!=", "not_in", "not_exists":
		o.mustNot = append(o.mustNot, &build_condition.WhereItem{
			Field:     field,
			Condition: condition,
			Val:       val,
			Boost:     boostVal,
		})
	}
}

func (o *Builder) OrWhere(items []*build_condition.WhereItem, opt ...*build_condition.Option) {
	if len(opt) > 0 && opt[0] != nil {
		for i := range items {
			items[i].Opt = opt[0]
		}
	}

	o.should = append(o.should, items)
}

type OrderOption struct {
	Missing string
}

func (o *Builder) OrderBy(field, by string, opt ...OrderOption) {
	var missing string
	if len(opt) > 0 {
		missing = opt[0].Missing
	}
	o.sort = append(o.sort, &build_condition.EsSort{
		Field:   field,
		Order:   by,
		Missing: missing,
	})
}

func (o *Builder) GroupBy(field string, groupNum, itemNum int) {
	o.Group = &build_condition.GroupBy{
		Field:    field,
		GroupNum: groupNum,
		ItemNum:  itemNum,
	}
}

func (o *Builder) GroupByString(s string) {
	o.GroupStr = s
}

func (o *Builder) AggsBy(name, aggsType, field string) {
	o.Aggs = &build_condition.Aggs{
		Name:  name,
		Type:  aggsType,
		Field: field,
	}
}

func (o *Builder) Must(field, condition string, val interface{}, boost ...float64) {
	var boostVal *float64
	if len(boost) > 0 {
		boostVal = &boost[0]
	}

	o.must = append(o.must, &build_condition.WhereItem{
		Field:     field,
		Condition: condition,
		Val:       val,
		Boost:     boostVal,
	})
}

func (o *Builder) MustWhere(tpl string) {
	o.must = append(o.must, &build_condition.WhereItem{
		Condition: "_where_",
		Val:       tpl,
	})
}

func (o *Builder) Filter(field, condition string, val interface{}, boost ...float64) {
	var boostVal *float64
	if len(boost) > 0 {
		boostVal = &boost[0]
	}

	o.filter = append(o.filter, &build_condition.WhereItem{
		Field:     field,
		Condition: condition,
		Val:       val,
		Boost:     boostVal,
	})
}

func (o *Builder) MustNot(field, condition string, val interface{}, boost ...float64) {
	var boostVal *float64
	if len(boost) > 0 {
		boostVal = &boost[0]
	}

	o.mustNot = append(o.mustNot, &build_condition.WhereItem{
		Field:     field,
		Condition: condition,
		Val:       val,
		Boost:     boostVal,
	})
}

func (o *Builder) Paging(page, pageSize int) {
	if page == 0 {
		page = 1
	}
	o.from = (page - 1) * pageSize
	o.size = pageSize
}

func (o *Builder) SetFrom(from int) {
	o.from = from
}

func (o *Builder) Size(size int) {
	o.size = size
	if size == 0 {
		o.sizeZero = true
	}
}

func (o *Builder) SearchAfter(val []interface{}) {
	o.searchAfter = val
}

func (o *Builder) BuildWhere() string {
	filter := o.buildQuery.Build(o.filter, nil)
	must := o.buildQuery.Build(o.must, o.should)
	mustNot := o.buildQuery.Build(o.mustNot, nil)

	q := fmt.Sprintf(`{"filter": %s,"must": %s,"must_not": %s}`, filter, must, mustNot)

	return q
}

func (o *Builder) BuildQuery() string {
	filter := o.buildQuery.Build(o.filter, nil)
	must := o.buildQuery.Build(o.must, o.should)
	mustNot := o.buildQuery.Build(o.mustNot, nil)
	runtimeMappings := o.buildQuery.BuildRuntimeMappings(o.runtimeMappings)
	sort := o.buildQuery.BuildSort(o.sort)
	paging := o.buildQuery.BuildPaging(o.from, o.size, o.sizeZero)
	searchAfter := o.buildQuery.BuildSearchAfter(o.searchAfter)
	groupBy := o.buildQuery.BuildGroupBy(o.Group, o.GroupStr)
	aggs := o.buildQuery.BuildAggs(o.Aggs)

	o.query = fmt.Sprintf(
		`{"query":{"bool":{"filter": %s,"must": %s,"must_not": %s}}%s%s%s%s%s%s}`,
		filter, must, mustNot, runtimeMappings, sort, paging, searchAfter, groupBy, aggs,
	)
	return o.query
}

func (o *Builder) WithTotal() *Builder {
	o.withTotal = true
	return o
}

func (o *Builder) WithSum(field string) *Builder {
	o.withSum = field
	return o
}

func (o *Builder) Index(index string) *Builder {
	o.index = []string{index}
	return o
}

func (o *Builder) Indexs(indexs []string) *Builder {
	o.index = indexs
	return o
}

func (o *Builder) Source(source []string) *Builder {
	o.source = source
	return o
}

func (o *Builder) RuntimeMappings(mappings string) {
	o.runtimeMappings = mappings
}

type SelectRs struct {
	Total int64
	Sum   float64
}

func (o *Builder) Select(ctx context.Context, ref interface{}) (*SelectRs, error) {
	query := o.BuildQuery()
	su_logger.Debug(ctx, "esQuery: "+strings.Join(o.index, ",")+" --- "+query)
	request := esapi.SearchRequest{
		Index:          o.index,
		Body:           strings.NewReader(query),
		TrackTotalHits: o.withTotal,
	}
	if len(o.source) > 0 {
		request.Source = o.source
	}
	resp, err := request.Do(ctx, o.es)
	if err != nil {
		return &SelectRs{}, err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return &SelectRs{}, errors.New("es resp err" + resp.String())
	}
	return o.ParseBody(resp.String(), ref), nil
}

func (o *Builder) ParseBody(body string, ref interface{}) *SelectRs {
	body = body[9:]
	parse := gjson.Parse(body)
	jsonStr := "["
	d := ""
	parse.Get("hits.hits").ForEach(func(key, value gjson.Result) bool {
		jsonStr = jsonStr + d + value.Get("_source").String()
		d = ","
		return true
	})
	jsonStr += "]"
	_ = jsoniter.UnmarshalFromString(jsonStr, ref)
	var total int64
	if o.withTotal {
		total = parse.Get("hits.total.value").Int()
	}

	var sum float64
	if o.withSum != "" {
		sum = parse.Get("aggregations." + o.withSum + ".value").Float()
	}

	return &SelectRs{
		Total: total,
		Sum:   sum,
	}
}
