package build_condition

type BuildInterface interface {
	Build(item *WhereItem) string
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
