package db_scanner

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cast"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
	"go.mongodb.org/mongo-driver/bson"
)

type PoolOption struct {
	// * 协程池大小
	Size int
	// * 当任务队列满时, 是否阻塞, 默认 false
	Blocking bool
	// ? 最大阻塞的任务量, 当 Nonblocking 设置为true, 此参数无效
	MaxBlockingTasks int
	// ? 过期时间, 默认1h
	ExpiryDuration time.Duration
	// ? 是否预分配
	PreAlloc bool
	// ? panic 处理句柄
	PanicHandler func(interface{})
}

type ScanOption struct {
	// 默认与Limit一致
	BuffSize int
	// 每次遍历获取的行数, 默认 limit * 2
	Limit int
	// 指定需要的列
	Cols []string
	// 定义如果获取下一个起始点, mongodb, 可以直接从返回记录提取order_key的地址,所以不需要额外定义
	FromId interface{}
	// 自定义排序, 为空是, 使用DocumentId进行排序, 默认 _id 正序
	Order *Order
	// 自定义条件
	Cond []Cond
	// 游标模式
	CursorMode bool
	// 游标默认的过期时间, 默认10分钟, -1 标识无过期时间
	CursorExpireTime time.Duration
	GetNext          func(data *Data) (interface{}, error)
}

func (o *ScanOption) Apply(opt *ScanOption) {
	if opt == nil {
		return
	}

	if opt.CursorMode {
		o.CursorMode = opt.CursorMode
	}
	if len(opt.Cond) > 0 {
		o.Cond = opt.Cond
	}
	if opt.BuffSize > 0 {
		o.BuffSize = opt.BuffSize
	}
	if opt.Limit > 0 {
		o.Limit = opt.Limit
	}
	if opt.Order != nil {
		o.Order = opt.Order
	}
	if opt.FromId != nil {
		o.FromId = opt.FromId
	}
	if opt.CursorExpireTime > 0 {
		o.CursorExpireTime = opt.CursorExpireTime
	}
	if len(opt.Cols) > 0 {
		o.Cols = opt.Cols
	}

	if opt.GetNext != nil {
		o.GetNext = opt.GetNext
	}
}

type Cond struct {
	// 字段
	Field string
	// 条件 =, ==, >, >=, <, <=, [not-in, nin], exists, [not-exists, nex], [in, in], [not, not]
	Cond string
	// 值
	Val interface{}
}

type Order struct {
	Key string
	// 1=>asc -1=>desc
	Dir int
}

type LimiterOption struct {
	// 桶大小
	Burst int
}

var dftScanOption = func() *ScanOption {
	opt := &ScanOption{
		BuffSize:         1000,
		Limit:            500,
		Cols:             nil,
		FromId:           nil,
		Order:            nil,
		Cond:             nil,
		CursorMode:       false,
		CursorExpireTime: 0,
		GetNext: func(data *Data) (next interface{}, err error) {
			if data == nil {
				return nil, errors.New("data is nil")
			}

			if len(data.ByteData) > 0 {
				var d map[string]interface{}
				err = bson.Unmarshal(data.ByteData, &d)
				if err != nil {
					return nil, err
				}
				return d["_id"], nil
			} else if data.Ref != nil {
				return data.Ref.Ref.ID, nil
			} else {
				return nil, errors.New("scanData is nil")
			}
		},
	}

	return opt
}

var dftBase = func() *Base {
	b := &Base{
		counter:         nil,
		filter:          nil,
		stash:           nil,
		db:              nil,
		coll:            "",
		scanOpt:         nil,
		name:            "",
		limiter:         nil,
		errHandler:      nil,
		stopWhenErr:     false,
		beforeStartHook: nil,
		finishHook:      nil,
		panicHandler:    nil,
		poolOpt:         nil,
		timeFrom:        time.Time{},
		endWithNotify:   false,
		buff:            nil,
		scanner:         nil,
	}

	b.counter = func(ctx context.Context) {
		if b.scanCnt > 0 && b.scanCnt%1000 == 0 {
			su_logger.Info(ctx, b.name+" scanned "+cast.ToString(b.scanCnt), su_logger.E().Int("handleCnt", b.handleCnt))
		}
	}

	b.stash = &Stash{
		size:  500,
		items: make([]interface{}, 0, 500),
	}

	b.poolOpt = &PoolOption{
		Size:     10,
		Blocking: true,
		PreAlloc: true,
	}

	b.errHandler = func(ctx context.Context, err error, msg string, data interface{}) {
		su_logger.Error(ctx, err, msg, su_logger.E().Any("data", data))
	}

	b.beforeStartHook = func(ctx context.Context) {
		su_logger.Info(ctx, "------------- begin "+b.name)
	}

	b.finishHook = func(ctx context.Context) {
		if b.endWithNotify {
			su_logger.InfoWithNotify(ctx, "------------- finish "+b.name, su_logger.E().String("usage", time.Since(b.timeFrom).String()).Int("scanned", b.scanCnt).Int("handled", b.handleCnt))
		} else {
			su_logger.Info(ctx, "------------- finish "+b.name, su_logger.E().String("usage", time.Since(b.timeFrom).String()).Int("scanned", b.scanCnt).Int("handled", b.handleCnt))
		}
	}

	b.panicHandler = func(ctx context.Context) {
		if err := recover(); err != nil {
			su_logger.Error(ctx, fmt.Errorf("%v", err), "panic", su_logger.E().Any("err", err))
		}
	}

	b.filter = func(i Data) (data interface{}, ignore bool, err error) {
		return i, false, nil
	}

	return b
}
