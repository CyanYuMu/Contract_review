package db

import (
	"context"
)

type DB interface {
	// Insert 创建一条新记录, 记录存在则进行覆盖
	Insert(ctx context.Context, table string, row Row, options ...*InsertOptions) (string, error)
	// Delete 删除一条记录
	Delete(ctx context.Context, table string, id string, options ...*DeleteOptions) (int, error)
	// Create 创建一条新记录, 如果记录存在返回Error, 返回对应的ID
	Create(ctx context.Context, table string, row Row, options ...*CreateOptions) (*CreateResult, error)
	// Update 更新一条记录
	Update(ctx context.Context, table string, update UpdateOne, options ...*UpdateOptions) error
	// Get 读取一条记录
	Get(ctx context.Context, table string, id string, options ...*GetOptions) (*DocumentRef, error)
	// Find 查询多条记录
	Find(ctx context.Context, table string, conds Conds, options ...*FindOptions) (*Iterator, error)
	// BatchCreate 批量创建记录, 如果记录存在则返回错误
	BatchCreate(ctx context.Context, table string, rows []Row, options ...*BatchWriteOptions) *BatchWriteResult
	// BatchInsert 批量插入记录, 如果记录存在则进行覆盖
	BatchInsert(ctx context.Context, table string, rows []Row, options ...*BatchWriteOptions) *BatchWriteResult
	// BatchDelete 批量删除记录
	BatchDelete(ctx context.Context, table string, ids []string, options ...*BatchWriteOptions) *BatchWriteResult
	// BatchUpdate 批量更新记录
	BatchUpdate(ctx context.Context, table string, updates []UpdateOne, options ...*BatchWriteOptions) *BatchWriteResult
	// BatchGet 批量获取记录
	BatchGet(ctx context.Context, table string, ids []string, options ...*GetOptions) (*Iterator, error)
	// Count 获取记录总数
	Count(ctx context.Context, table string, conds Conds) (int64, error)
	// Upsert 更新或插入一条记录
	Upsert(ctx context.Context, table string, row UpsertRow, options ...*UpsertOptions) (*UpsertRs, error)
	// 对单个字段进行更新或插入, 比如 cover 对象有一个 height, width, url 字段, 可以直接更新其中的某个字段并如果不存在会自动创建
	UpsertSingleField(ctx context.Context, table string, row UpsertSingleFields, options ...*UpsertSingleFieldRowOptions) error
	// 批量更新或插入记录
	BatchUpsert(ctx context.Context, table string, rows []UpsertRow, options ...*BatchWriteOptions) (*BatchUpsertRs, error)

	UpDateOne
}

type UpsertSingleFields struct {
	Id     string
	Fields map[string]interface{}
}

type UpsertSingleFieldRowOptions struct {
	// 是否禁用合并所有字段, 默认是禁用
	DisableMergeAll bool
}

type BatchUpsertRs struct {
	InsertCount int64
	UpdateCount int64
}

type UpDateOne interface {
	ToUpdateOne(data any, options *UpdateOptions) (upData UpdateOne)
}

func InitGlobalChangeHook(hook ChangeHook) {
	glbChangeHook = hook
}

type UpsertRs struct {
	// 新增的记录id
	Id string
	// 匹配到的记录数量
	MatchCount int64
}

type UpsertOptions struct {
	// IgnoreError 忽略错误
	IgnoreError bool
	ChangeHook  ChangeHook
}

func (u *UpsertOptions) Apply(opt ...*UpsertOptions) {
	if len(opt) == 0 || opt[0] == nil {
		return
	}
	if opt[0].ChangeHook != nil {
		u.ChangeHook = opt[0].ChangeHook
	}
	if opt[0].IgnoreError {
		u.IgnoreError = opt[0].IgnoreError
	}
}

type UpsertRow struct {
	Id string
	// 更新操作
	Updates []Update
	// 插入操作
	Inserts []Update
}

// BatchUpdateOptions 定义批量更新的选项
type BatchUpdateOptions struct {
	// ContinueOnError 控制在更新失败时是否继续
	ContinueOnError bool
}

// 定义了包含ID的更新结构
type UpdateOne struct {
	ID      string
	Updates []Update
}

const UpdateField = "update_at"
