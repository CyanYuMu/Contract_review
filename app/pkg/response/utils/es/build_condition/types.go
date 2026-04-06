package build_condition

type WhereItem struct {
	Field string
	// between: 左右闭合, left_between: 左开右闭, right_between: 左闭右开
	Condition string
	Val       interface{}
	Opt       *Option
	Boost     *float64 // 新增：boost 参数
}

// "missing": "_last"//
type EsSort struct {
	Field   string
	Order   string
	Missing string
}

type GroupBy struct {
	Field    string // 分组字段
	GroupNum int    // 获取分组的数量
	ItemNum  int    // 每个分组的item数量
}

type Aggs struct {
	Name  string // 聚合后字段名
	Type  string // 聚合类型
	Field string // 聚合字段
}

type Option struct {
	// 最小匹配数量, 服务于should场景
	MinimumShouldMatch *int
	// 新增：boost 参数
	Boost *float64
}
