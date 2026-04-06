package gormx

import (
	"database/sql"
	jsoniter "github.com/json-iterator/go"
	gocache "github.com/patrickmn/go-cache"
	"github.com/spf13/cast"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/su_string"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"reflect"
	"regexp"
	"strings"
	"time"
)

type GormX struct {
	*gorm.DB
	cache *gocache.Cache
}

func New(db *gorm.DB) *GormX {
	return &GormX{
		DB:    db,
		cache: gocache.New(time.Minute*30, time.Minute),
	}
}

func (g *GormX) Ignore() *gorm.DB {
	tx := g.Clauses(clause.Insert{Modifier: "IGNORE"})

	return tx
}

// OnDuplicateKeyUpdate
// 支持两种方式:
// 1. upFields: 优先级高, 会将value自动转换成 gorm.Expr 类型, 适用于赋值过程中需要使用函数的情况
// eg:
//
//	map[string]string{
//		"name": "if(values(`refund_at`) = 0, `refund_at`, values(`refund_at`))",
//		"update_at":"values(`update_at`)"
//	}
//
// 2. columns, 简单更新特定的列, 仅读取 index = 0 的数据
// eg: []string{"name", "update"}
func (g *GormX) OnDuplicateKeyUpdate(upFields map[string]string, columns ...[]string) (tx *gorm.DB) {
	if len(upFields) > 0 {
		var assignData = make(map[string]interface{}, len(upFields))
		for field, _ := range upFields {
			if strings.Contains(upFields[field], "(") {
				assignData[field] = gorm.Expr(upFields[field])
			} else {
				assignData[field] = gorm.Expr("values(`" + field + "`)")
			}
		}
		/*
			DoUpdates: clause.Assignments(map[string]interface{}{
				"update_at": gorm.Expr("values(`update_at`)"),
				"refund_at":     gorm.Expr("if(values(`refund_at`) = 0, `refund_at`, values(`refund_at`))"),
			}
		*/
		tx = g.Clauses(clause.OnConflict{
			DoUpdates: clause.Assignments(assignData),
		})
	} else if len(columns) > 0 {
		tx = g.Clauses(clause.OnConflict{
			DoUpdates: clause.AssignmentColumns(columns[0]),
		})
	}

	return tx
}

func isZero(v interface{}, param map[string]interface{}) (b bool) {
	defer func() {
		if err := recover(); err != nil {
			b = true
		}
	}()
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)

	if rv.IsZero() {
		return true
	}
	// 对切片和数组进行长度判断
	if l, ok := v.([]interface{}); ok {
		return len(l) == 0
	}

	return false
}

var gormColumnPattern = regexp.MustCompile(`column\:([a-z_\d]+)`)

func (g *GormX) toParamData(param interface{}) (data map[string]interface{}) {
	rt := reflect.TypeOf(param)
	rv := reflect.ValueOf(param)
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
		rv = rv.Elem()
	}
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
		rv = rv.Elem()
	}

	if rt.Kind() == reflect.Map {
		data = rv.Interface().(map[string]interface{})
	} else if rt.Kind() == reflect.Struct {
		numField := rt.NumField()
		data = make(map[string]interface{}, numField)
		cacheKey := rt.PkgPath() + ":" + rt.Name()
		cacheData, exists := g.cache.Get(cacheKey)
		if exists {
			cData := cacheData.(map[string]int)
			for field, idx := range cData {
				data[field] = rv.Field(idx).Interface()
			}
		} else {
			var cData = make(map[string]int, numField)
			for i := 0; i < numField; i++ {
				var curName string
				// 提取 column 数据
				curTag := rt.Field(i).Tag.Get("gorm")
				if curTag == "" {
					curName = su_string.Camel2Case(rt.Field(i).Name)
					data[curName] = rv.Field(i).Interface()
				} else {
					// 正则提取column信息
					tagMatch := gormColumnPattern.FindAllStringSubmatch(curTag, 1)
					if len(tagMatch) > 0 {
						curName = tagMatch[0][1]
					}
				}

				data[curName] = rv.Field(i).Interface()
				cData[curName] = i
			}

			g.cache.Set(cacheKey, cData, time.Minute*30)
		}
	}

	return data
}

func (g *GormX) toParamList(paramData interface{}) (list []map[string]interface{}) {
	rv := reflect.ValueOf(paramData)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		l := rv.Len()
		list = make([]map[string]interface{}, 0, l)
		for i := 0; i < l; i++ {
			list = append(list, g.toParamData(rv.Index(i).Interface()))
		}
	}

	return list
}

// Condition2Where
// @description 条件列表结合当前参数转sql变量模版
func (g *GormX) Condition2Where(condList []*Cond, param interface{}) (tpl string) {
	paramData := g.toParamData(param)
	w := strings.Builder{}
	for i, _ := range condList {
		cKey := condList[i].Key
		if cKey != "" {
			if curV, exists := paramData[cKey]; !exists {
				if condList[i].Default != nil {
					paramData[cKey] = condList[i].Default
				} else {
					// 未定义则忽略
					continue
				}
			} else {
				ignore := isZero
				if condList[i].Ignore != nil {
					ignore = condList[i].Ignore
				}

				if ignore(curV, paramData) {
					continue
				}
			}

			if condList[i].Sql == "" {
				condList[i].Sql = cKey + "=:" + cKey
			}
		} else {
			if condList[i].Sql == "" {
				continue
			}
		}

		if w.Len() > 0 {
			w.WriteString(" AND ")
			w.WriteString(condList[i].Sql)
		} else {
			w.WriteString(condList[i].Sql)
		}
	}

	return w.String()
}

type Cond struct {
	// 当前的key
	Key string
	// 可选, 默认值, 如果不填, 当检测到当前 key 未在 param 变量中定义时, 会忽略本条件
	Default interface{}
	// 可选, 当定义了key但未定义sql时, sql会自动创建为 `key = :key` 形式
	Sql string
	// 可选, 用于判断当前的值条件是有有效, 如 nil, [] 都认为是无效值
	Ignore func(v interface{}, param map[string]interface{}) bool
}

const variablePrefix = ':'

func (g *GormX) ParseSql(sql string, param map[string]interface{}) (s string, valList []interface{}) {
	from := strings.IndexByte(sql, variablePrefix)
	if from == -1 {
		return sql, valList
	}
	valList = make([]interface{}, 0, len(param))

	l := len(sql)

	var v strings.Builder
	var vFlag bool
	var w strings.Builder

	w.WriteString(sql[:from])
	for ; from < l; from++ {
		if sql[from] == variablePrefix {
			v.Reset()
			vFlag = true
			w.WriteByte('?')
		} else {
			if vFlag {
				i := int(sql[from])
				if (i >= 65 && i <= 90) || (i >= 97 && i <= 122) || (i >= 48 && i <= 57) || (i == 95) {
					v.WriteByte(sql[from])
				} else {
					w.WriteByte(sql[from])
					k := v.String()
					vFlag = false
					if k == "" {
						continue
					}
					v.Reset()
					// 处理变量
					if pv, exists := param[k]; exists {
						valList = append(valList, pv)
					} else {
						continue
					}
				}
			} else {
				w.WriteByte(sql[from])
			}
		}
	}

	if v.Len() > 0 {
		// 处理尾巴变量
		k := v.String()
		// 处理变量
		if pv, exists := param[k]; exists {
			valList = append(valList, pv)
		} else {
			return
		}
	}

	return w.String(), valList
}

// RawX
// 基于sql变量模版进行查询 eg: select * from tag where name=:name and type=:type
// @param param 支持的数据类型
// - map[string]interface{}
// - 结构体
// - 结构体指针
func (g *GormX) RawX(sqlTpl string, param interface{}) (tx *gorm.DB) {
	paramData := g.toParamData(param)
	sql, valList := g.ParseSql(sqlTpl, paramData)

	tx = g.Raw(sql, valList...)

	return tx
}

func (g *GormX) BuildSqlWithParam(sqlTpl string, param interface{}) string {
	paramData := g.toParamData(param)
	sqlTpl, valList := g.ParseSql(sqlTpl, paramData)
	if len(valList) > 0 {
		for i := range valList {
			v := ToSqlStringVal(valList[i])
			sqlTpl = strings.Replace(sqlTpl, "?", v, 1)
		}
	}

	return sqlTpl
}

func (g *GormX) BuildPrepareSql(sqlTpl string, vals []interface{}) string {
	if len(vals) > 0 {
		for i := range vals {
			v := ToSqlStringVal(vals[i])
			sqlTpl = strings.Replace(sqlTpl, "?", v, 1)
		}
	}

	return sqlTpl
}

func ToSqlStringVal(v interface{}) string {
	switch v.(type) {
	case string:
		s := v.(string)
		return "'" + s + "'"
	case time.Time:
		return "'" + v.(time.Time).Format("2006-01-02 15:04:05") + "'"
	case []interface{}:
		toString, _ := jsoniter.MarshalToString(v)
		return toString
	case nil:
		return "NULL"
	case map[string]interface{}:
		toString, _ := jsoniter.MarshalToString(v)
		return "'" + toString + "'"
	default:
		return cast.ToString(v)
	}
}

// ExecX
// 基于sql变量模版进行执行 eg: update tag set name=:name and type=:type where id=:id
// param 支持的类型:
// - map[string]interface{}
// - 结构体
// - 结构体指针
func (g *GormX) ExecX(sqlTpl string, param interface{}) (tx *gorm.DB) {
	paramData := g.toParamData(param)
	sql, valList := g.ParseSql(sqlTpl, paramData)

	tx = g.Exec(sql, valList...)

	return tx
}

/*TransactionX
* @Description: 开启事务, 针对事务中出现死锁的情况, 进行重试
* @param fn
* @param deadLockRetryTimes 发生死锁的重试次数, 默认为1
* @param opts
* @return err
 */
func (g *GormX) TransactionX(fn func(tx *gorm.DB) error, deadLockRetryTimes int, opts ...*sql.TxOptions) (err error) {
	if deadLockRetryTimes < 1 {
		deadLockRetryTimes = 1
	}
	for i := 0; i < deadLockRetryTimes; i++ {
		err = g.Transaction(fn, opts...)
		if err != nil {
			if strings.Contains(err.Error(), "Deadlock") {
				// 针对死锁重试
				time.Sleep(time.Millisecond * 200)
				continue
			} else {
				return err
			}
		} else {
			return nil
		}
	}

	return err
}

// BatchUpdate
// 批量更新, 对应 update case when 场景
// @param list 数据集, 支持的数据类型:
// - []map[string]interface{}
// - []结构体
// - []结构体指针
// @param uniqueFields 手动指定冲突键列表 []string{"job_leve"}
// eg:
// @param
// 执行的效果
// update  employee
// set         e_wage =
// case
// when   job_level = '1'    then e_wage*1.97
// when   job_level = '2'   then e_wage*1.07
// when   job_level = '3'   then e_wage*1.06
// else     e_wage*1.05
// end
func (g *GormX) BatchUpdate(tableName string, list interface{}, uniqueFields []string) (tx *gorm.DB) {
	var sqlStr = strings.Builder{}
	sqlStr.WriteString("UPDATE ")
	sqlStr.WriteString(addChar(tableName))
	sqlStr.WriteString(" SET ")
	dataList := g.toParamList(list)
	var valList = make([]interface{}, 0, len(dataList)*(len(dataList[0])-len(uniqueFields)))
	valFields, caseTpl := prepareUpdateField(dataList[0], uniqueFields)
	caseSql := prepareBatchUpdate(dataList, valFields, uniqueFields, caseTpl, &valList)
	sqlStr.WriteString(caseSql)
	// 构建where
	where := strings.Builder{}
	for i := range uniqueFields {
		var fieldWhere strings.Builder
		fieldWhere.WriteString(addChar(uniqueFields[i]))
		fieldWhere.WriteString(" IN (")
		for j := range dataList {
			fieldWhere.WriteString("?,")
			valList = append(valList, dataList[j][uniqueFields[i]])
		}

		if where.Len() > 0 {
			where.WriteString(" AND ")
		}
		where.WriteString(fieldWhere.String()[:fieldWhere.Len()-1])
		where.WriteString(")")
	}

	// 构建sql
	sqlStr.WriteString(" WHERE ")
	sqlStr.WriteString(where.String())

	tx = g.Exec(sqlStr.String(), valList...)

	return
}

func prepareBatchUpdate(dataList []map[string]interface{}, valFields []string, uniqFields []string, caseTpl string, valList *[]interface{}) (sqlStr string) {
	sqlBuff := strings.Builder{}
	for i := range valFields {
		if sqlBuff.Len() > 0 {
			sqlBuff.WriteString(",")
		}
		sqlBuff.WriteString(" ")
		sqlBuff.WriteString(addChar(valFields[i]))
		sqlBuff.WriteString(" = (CASE ")

		for j := range dataList {
			sqlBuff.WriteString(" WHEN ")
			sqlBuff.WriteString(caseTpl)
			for k := range uniqFields {
				*valList = append(*valList, dataList[j][uniqFields[k]])
			}
			sqlBuff.WriteString(" THEN ?")
			*valList = append(*valList, dataList[j][valFields[i]])
		}
		sqlBuff.WriteString(" END)")
	}

	sqlStr = sqlBuff.String()

	return
}

func prepareUpdateField(allFields map[string]interface{}, uniqFields []string) (valFields []string, caseSql string) {
	caseTpl := strings.Builder{}
	valFields = make([]string, 0, len(allFields)-len(uniqFields))
	fieldMap := make(map[string]bool, len(uniqFields))
	for i, _ := range uniqFields {
		caseTpl.WriteString(addChar(uniqFields[i]))
		caseTpl.WriteString(" = ? AND ")
		fieldMap[uniqFields[i]] = true
	}
	for field, _ := range allFields {
		if _, ok := fieldMap[field]; !ok {
			// 过滤掉唯一键
			valFields = append(valFields, field)
		}
	}
	caseSql = caseTpl.String()[0 : caseTpl.Len()-5]

	return
}

var charReg = regexp.MustCompile("[,'\"\\s\\.`]")
var varReg = regexp.MustCompile(":[a-zA-Z0-9_-]+")

func addChar(k string) string {
	if charReg.MatchString(k) {
		return k
	} else {
		return "`" + k + "`"
	}
}
