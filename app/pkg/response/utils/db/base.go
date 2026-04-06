package db

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"

	"cloud.google.com/go/firestore"
	jsoniter "github.com/json-iterator/go"
	"go.mongodb.org/mongo-driver/bson"
)

type Action string

const (
	ActionAdd    Action = "add"
	ActionUpdate Action = "update"
	ActionDelete Action = "delete"
)

type ChangeRow struct {
	Table  string                 `json:"table"`
	Id     string                 `json:"id"`
	Action Action                 `json:"action"`
	Data   map[string]interface{} `json:"data"`
}

type ChangeHook func(ctx context.Context, row ...ChangeRow)

var glbChangeHook ChangeHook

// Row 定义了一个接口，用于标识数据库的行
type Row interface {
	// ID 返回文档ID
	ID() string
	SetID(id string)
}

// Cond 定义了一个条件结构体，用于表示查询条件
type Cond struct {
	Field string
	// 支持 >, >=, <, <=, in, array-contains, not-in, !=, ==|= (default)
	Cond  string
	Value interface{}
}

type Conds []Cond

type Sort struct {
	// 排序字段
	Field string
	// 排序顺序，1表示升序，-1表示降序
	Order int8
}

type Sorts []Sort

type FindOptions struct {
	Options
	Offset int64
	Limit  int64
	Sorts  Sorts
	Fields []string
	// 是否返回总数
	// WithTotal bool
}

type Options struct {
	// default jsoniter
	Unmarshal func(data []byte, v interface{}) error
	// default jsoniter
	Marshal func(v interface{}) ([]byte, error)
}

type GetOptions struct {
	Options
	Fields []string
}

// BatchWriteOptions 定义批量写入选项
type BatchWriteOptions struct {
	// 当记录存在时是否继续, 1是 2 否, 服务于 [批量]插入, [批量]创建
	ContinueOnExistsError int8
	// 当发生错误时是否继续, 1是 2 否, 服务于 [批量]删除, [批量]更新
	ContinueOnError int8
	// 如果存在则更新, 1是 2 否
	InsertOnDuplicate int8
	// 指定更新的列
	UpdateFields []string
	// 批次大小, 默认30
	BatchSize  int
	ChangeHook ChangeHook
}

func (b *BatchWriteOptions) Hook() ChangeHook {
	if b == nil {
		return nil
	}
	return b.ChangeHook
}

func (b *BatchWriteOptions) Apply(o ...*BatchWriteOptions) {
	if len(o) == 0 || o[0] == nil {
		return
	}

	if o[0].ContinueOnExistsError > 0 {
		b.ContinueOnExistsError = o[0].ContinueOnExistsError
	}
	if o[0].InsertOnDuplicate > 0 {
		b.InsertOnDuplicate = o[0].InsertOnDuplicate
	}
	if o[0].BatchSize > 0 {
		b.BatchSize = o[0].BatchSize
	}
	if o[0].UpdateFields != nil {
		b.UpdateFields = o[0].UpdateFields
	}

	if o[0].ContinueOnError > 0 {
		b.ContinueOnError = o[0].ContinueOnError
	}

	if o[0].ChangeHook != nil {
		b.ChangeHook = o[0].ChangeHook
	}
}

func dftBatchWriteOptions() *BatchWriteOptions {
	return &BatchWriteOptions{
		ContinueOnExistsError: 1,
		InsertOnDuplicate:     1,
		UpdateFields:          nil,
		BatchSize:             30,
		ContinueOnError:       2,
		ChangeHook:            glbChangeHook,
	}
}

type BatchWriteResult struct {
	Ids    []string
	Errors []error
	// 影响行数
	Affected int64
}

type ErrorOptions struct {
	IgnoreDuplicate bool
	IgnoreNotFound  bool
}

func (b *BatchWriteResult) Error(opt ...*ErrorOptions) error {
	if b == nil {
		return nil
	}

	for _, err := range b.Errors {
		if err == nil {
			continue
		}

		if len(opt) > 0 && opt[0] != nil {
			if opt[0].IgnoreDuplicate && strings.Contains(err.Error(), "duplicate key") {
				continue
			} else if opt[0].IgnoreNotFound && strings.Contains(err.Error(), "not found") {
				continue
			}
		}
		return err
	}

	return nil
}

type Update struct {
	// 更新的字段
	Field string
	// 更新的方式, 默认 =, 其他支持 incr
	Op    string
	Value interface{}
}

var ErrDocAlreadyExists = errors.New("already exists")
var ErrDocNotFound = errors.New("document not found")

func IsNotExistErr(err error) bool {
	if err == nil {
		return false
	}
	errS := err.Error()
	if strings.Contains(errS, "no documents") || strings.Contains(errS, "exist") || strings.Contains(errS, "found") || strings.Contains(errS, "nil") {
		return true
	}

	return false
}

func IsAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	errS := err.Error()
	if strings.Contains(errS, "already exist") || strings.Contains(errS, "dup key") || strings.Contains(errS, "duplicate") {
		return true
	}

	return false
}

// type RowData []byte
type RowData interface{}

// DatabaseCURL 定义了数据库操作的通用接口
// DocumentRef 定义了文档引用的结构体
type DocumentRef struct {
	Options
	ID   string
	Data RowData
}

// 添加一个类型缓存结构
var (
	typeCache = make(map[reflect.Type]reflect.Type)
	// 用于保护 typeCache 的并发访问
	typeCacheMu sync.RWMutex
)

// 获取缓存的类型
func getCachedType(t reflect.Type) reflect.Type {
	typeCacheMu.RLock()
	if cached, ok := typeCache[t]; ok {
		typeCacheMu.RUnlock()
		return cached
	}
	typeCacheMu.RUnlock()

	typeCacheMu.Lock()
	defer typeCacheMu.Unlock()

	// Double check after acquiring write lock
	if cached, ok := typeCache[t]; ok {
		return cached
	}

	typeCache[t] = t
	return t
}

// 优化后的 All 方法
func (i *Iterator) All(ctx context.Context, data interface{}) (err error) {
	if len(i.items) == 0 {
		return nil
	}

	v := reflect.ValueOf(data)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return fmt.Errorf("data argument must be a non-nil pointer to a slice")
	}

	sliceVal := v.Elem()
	if sliceVal.Kind() != reflect.Slice {
		return fmt.Errorf("data argument must point to a slice")
	}

	// 使用类型缓存
	sliceType := getCachedType(sliceVal.Type())
	elemType := getCachedType(sliceType.Elem())

	// 预分配切片容量
	slice := reflect.MakeSlice(sliceType, 0, len(i.items))

	for j := range i.items {
		// 从池中获取元素
		var elem interface{}
		if elemType.Kind() != reflect.Ptr {
			elem = reflect.New(elemType).Interface()
		} else {
			elem = reflect.New(elemType.Elem()).Interface()
		}
		ref := DocumentRef{
			ID:   i.items[j].ID,
			Data: i.items[j].Data,
		}

		if err := ref.Unmarshal(ctx, elem.(Row)); err != nil {
			return err
		}

		// 将元素添加到切片中
		if elemType.Kind() == reflect.Ptr {
			// 如果元素类型是指针，直接附加 elem
			slice = reflect.Append(slice, reflect.ValueOf(elem))
		} else {
			// 如果元素类型是值，解引用指针
			slice = reflect.Append(slice, reflect.ValueOf(elem).Elem())
		}
	}

	// 设置最终的切片值
	sliceVal.Set(slice)
	return nil
}

// 优化 DocumentRef 的 Unmarshal 方法
func (d *DocumentRef) Unmarshal(ctx context.Context, data Row) error {
	if d == nil {
		return ErrDocNotFound
	}

	switch v := d.Data.(type) {
	case []byte:
		if len(v) < 4 {
			return ErrDocNotFound
		}
		// 使用自定义的 Unmarshal 函数（如果提供）
		if d.Options.Unmarshal != nil {
			return d.Options.Unmarshal(v, data)
		}
		data.SetID(d.ID)
		return jsoniter.Unmarshal(v, data)
	case bson.Raw:
		err := bson.Unmarshal(v, data)
		if err != nil {
			return err
		}

		data.SetID(d.ID)

		return nil

	case *firestore.DocumentSnapshot:
		err := v.DataTo(data)
		if err != nil {
			return err
		}

		// 尝试通过反射设置ID字段
		data.SetID(v.Ref.ID)
		// 	else {
		// 	// 如果没有实现Row接口，尝试通过反射设置ID字段
		// 	val := reflect.ValueOf(data)
		// 	if val.Kind() == reflect.Ptr {
		// 		val = val.Elem()
		// 	}
		// 	if val.Kind() == reflect.Struct {
		// 		if idField := val.FieldByName("Id"); idField.IsValid() && idField.CanSet() {
		// 			idField.SetString(v.Ref.ID)
		// 		}
		// 	}
		// }

		return nil
	case map[string]interface{}:
		// 处理 map 类型的数据
		b, err := jsoniter.Marshal(v)
		if err != nil {
			return err
		}
		if d.Options.Unmarshal != nil {
			return d.Options.Unmarshal(b, data)
		}
		err = jsoniter.Unmarshal(b, data)
		if err != nil {
			return err
		}
		data.SetID(d.ID)
	}

	return fmt.Errorf("unsupported data type: %T", d.Data)
}

// Iterator 定义了迭代器结构体
type Iterator struct {
	// 这里可以添加迭代器需要的字段
	items []*DocumentRef
}

// IsEmpty 判断迭代器是否为空
func (i *Iterator) IsEmpty() bool {
	return i == nil || len(i.items) == 0
}

// Next 返回下一个文档引用和可能的错误
func (i *Iterator) Next() (*DocumentRef, error) {
	if len(i.items) == 0 {
		// 回收
		i.items = nil
		return nil, io.EOF
	}

	item := i.items[0]
	i.items = i.items[1:]

	return item, nil
}

func (i *Iterator) Close() {
	i.items = nil
}

func (i *Iterator) First(ctx context.Context, data Row) (err error) {
	if i == nil {
		return ErrDocNotFound
	}

	if len(i.items) == 0 {
		return ErrDocNotFound
	}

	ref := DocumentRef{
		ID:   i.items[0].ID,
		Data: i.items[0].Data,
	}

	return ref.Unmarshal(ctx, data)
}

type InsertOptions struct {
	ChangeHook ChangeHook
	MergeAll   bool
}

func (o *InsertOptions) Hook() ChangeHook {
	if o == nil {
		return nil
	}
	return o.ChangeHook
}

func (o *InsertOptions) Apply(opt ...*InsertOptions) {
	if len(opt) == 0 || opt[0] == nil {
		return
	}

	if opt[0].ChangeHook != nil {
		o.ChangeHook = opt[0].ChangeHook
	}
}

func dftInsertOptions() *InsertOptions {
	return &InsertOptions{
		ChangeHook: glbChangeHook,
		MergeAll:   false,
	}
}

type DeleteOptions struct {
	ChangeHook ChangeHook
}

func (o *DeleteOptions) Hook() ChangeHook {
	if o == nil {
		return nil
	}
	return o.ChangeHook
}

func (o *DeleteOptions) Apply(opt ...*DeleteOptions) {
	if len(opt) == 0 || opt[0] == nil {
		return
	}

	if opt[0].ChangeHook != nil {
		o.ChangeHook = opt[0].ChangeHook
	}
}

func dftDeleteOptions() *DeleteOptions {
	return &DeleteOptions{
		ChangeHook: glbChangeHook,
	}
}

type CreateOptions struct {
	ChangeHook ChangeHook
}

func (o *CreateOptions) Hook() ChangeHook {
	if o == nil {
		return nil
	}
	return o.ChangeHook
}

func (o *CreateOptions) Apply(opt ...*CreateOptions) {
	if len(opt) == 0 || opt[0] == nil {
		return
	}

	if opt[0].ChangeHook != nil {
		o.ChangeHook = opt[0].ChangeHook
	}
}

func dftCreateOptions() *CreateOptions {
	return &CreateOptions{
		ChangeHook: glbChangeHook,
	}
}

type UpdateOptions struct {
	// ? 自定义判断是否进行过滤
	Filter func(tagName string, v interface{}) (ignore bool)
	// ? 自定义判断是否进行过滤
	EmptyIgnore func(tagName string, v interface{}) (ignore bool)
	// ? 标签tag名称
	Tag string
	// 忽略更新时间字段, 默认自动填充
	IgnoreUpdateTimeField bool
	ChangeHook            ChangeHook
	// structTag             string
}

func (o *UpdateOptions) Hook() ChangeHook {
	if o == nil {
		return nil
	}
	return o.ChangeHook
}

func (o *UpdateOptions) Apply(opt ...*UpdateOptions) {
	if len(opt) == 0 || opt[0] == nil {
		return
	}

	if opt[0].Filter != nil {
		o.Filter = opt[0].Filter
	}
	if opt[0].EmptyIgnore != nil {
		o.EmptyIgnore = opt[0].EmptyIgnore
	}
	if opt[0].Tag != "" {
		o.Tag = opt[0].Tag
	}
	if opt[0].IgnoreUpdateTimeField {
		o.IgnoreUpdateTimeField = opt[0].IgnoreUpdateTimeField
	}
	if opt[0].ChangeHook != nil {
		o.ChangeHook = opt[0].ChangeHook
	}
}

func (o *UpdateOptions) ToMongoOptions() *UpdateOptions {
	return &UpdateOptions{
		Filter:                o.Filter,
		EmptyIgnore:           o.EmptyIgnore,
		Tag:                   "bson",
		IgnoreUpdateTimeField: o.IgnoreUpdateTimeField,
		ChangeHook:            o.ChangeHook,
	}
}

func dftUpdateOptions() *UpdateOptions {
	return &UpdateOptions{
		ChangeHook: glbChangeHook,
	}
}

type CreateResult struct {
	ID string
}

func dftUpsertOptions() *UpsertOptions {
	return &UpsertOptions{
		ChangeHook: glbChangeHook,
	}
}
