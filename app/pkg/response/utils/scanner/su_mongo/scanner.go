package su_mongo

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/db"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/runtime"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/scanner"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ScanWhere struct {
	Path string
	Cond string
	Val  interface{}
}

type OrderBy struct {
	// 排序的字段
	Path string
	// true 代表 >= <= 会结合Dir进行判断, false 代表 >, <
	Include bool
	// 排序的方向
	Dir int
}

type ScanOption struct {
	// 默认与Limit一致
	BuffSize int
	// 每次遍历获取的行数, 默认 limit * 2
	Limit int64
	// 指定需要的列
	Cols []string
	// 定义如果获取下一个起始点, mongodb, 可以直接从返回记录提取order_key的地址,所以不需要额外定义
	//GetNext func(data interface{}) interface{}
	// 定义起始点
	FromId interface{}
	// 自定义排序, 为空是, 使用DocumentId进行排序, 默认 _id 正序
	Order *Order
	// 自定义条件
	Cond bson.M
	// 游标模式
	CursorMode bool
	// 游标默认的过期时间, 默认10分钟, -1 标识无过期时间
	CursorExpireTime time.Duration
}

func (o *ScanOption) HasConfirmedValue(field string) bool {
	for k := range o.Cond {
		if k == field {
			cond, ok := o.Cond[k].(bson.M)
			if !ok {
				continue
			}
			if cond["$in"] != nil || cond["$nin"] != nil || cond["$exists"] != nil || cond["$ne"] != nil || cond["$eq"] != nil || cond["=="] != nil || cond["="] != nil || cond["!="] != nil {
				return true
			}
		}
	}

	return false
}

type Order struct {
	Key string
	Dir int
}

type Stash struct {
	lock sync.Mutex
	// buff size
	size int
	// buff item
	items []interface{}
}

func (s *Stash) IsEmpty() bool {
	return len(s.items) == 0
}

func (s *Stash) Append(item ...interface{}) (full bool, items []interface{}) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.items = append(s.items, item...)

	if len(s.items) >= s.size {
		items = s.items
		s.items = make([]interface{}, 0, s.size)
		full = true

		return
	}

	return false, nil
}

type Scanner struct {
	cnt int
	// 计数器
	counter func(ctx context.Context, i interface{})
	// 数据过滤器
	filter  func(i bson.M) (data interface{}, ignore bool, err error)
	stash   *Stash
	db      *db.MongoDB
	coll    string
	scanOpt *ScanOption
	name    string

	// 频率限制
	limiter scanner.Limiter
	// 自定义错误处理
	errHandler  func(ctx context.Context, err error, msg string, data interface{})
	stopWhenErr bool
	// BeforeStart 会在开始前调用
	beforeStartHook func(ctx context.Context)
	// 结束时调用
	finishHook func(ctx context.Context)
	// panic 自定义处理句柄
	panicHandler func(ctx context.Context)
	// 协程池配置
	poolOpt       *PoolOption
	timeFrom      time.Time
	endWithNotify bool
}

func (s *Scanner) EndWithNotify(b bool) *Scanner {
	s.endWithNotify = b

	return s
}

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

func (s *Scanner) Pool(opt *PoolOption) *Scanner {
	s.poolOpt = opt

	return s
}

func (s *Scanner) StopWhenErr(b bool) *Scanner {
	s.stopWhenErr = b

	return s
}

func (s *Scanner) PanicHandler(fn func(ctx context.Context)) *Scanner {
	s.panicHandler = fn

	return s
}

func (s *Scanner) Counter(fn func(ctx context.Context, i interface{})) *Scanner {
	s.counter = fn

	return s
}

func (s *Scanner) Scan(db *db.MongoDB, coll string, opt *ScanOption) *Scanner {
	s.db = db
	s.coll = coll
	s.scanOpt = opt

	return s
}

func (s *Scanner) ErrHandler(fn func(ctx context.Context, err error, msg string, data interface{})) *Scanner {
	s.errHandler = fn

	return s
}

func (s *Scanner) BeforeStartHook(fn func(ctx context.Context)) *Scanner {
	s.beforeStartHook = fn

	return s
}

func (s *Scanner) FinishHook(fn func(ctx context.Context)) *Scanner {
	s.finishHook = fn

	return s
}

func (s *Scanner) StashSize(n int) *Scanner {
	s.stash.size = n
	s.stash.items = make([]interface{}, 0, n)

	return s
}

func (s *Scanner) Filter(fn func(i bson.M) (data interface{}, ignore bool, err error)) *Scanner {
	s.filter = fn

	return s
}

func Scan(ctx context.Context, db *db.MongoDB, coll string, opt *ScanOption) (buff chan bson.M) {
	if opt.BuffSize == 0 {
		opt.BuffSize = 500
	}
	buff = make(chan bson.M, opt.BuffSize)

	if opt.Order == nil {
		opt.Order = &Order{
			Key: "",
			Dir: 1,
		}
		if !opt.CursorMode {
			opt.Order.Key = "_id"
		}

		if opt.FromId != nil {
			opt.Order.Key = "_id"
		}
	}

	// if opt.CursorExpireTime == 0 {
	// 	opt.CursorExpireTime = time.Minute * 10
	// }

	if opt.Cond == nil {
		opt.Cond = bson.M{}
	}

	if opt.FromId != nil {
		if opt.Order.Dir > 0 {
			opt.Cond[opt.Order.Key] = bson.M{opt.Order.Key: bson.M{"$gt": opt.FromId}}
		} else {
			opt.Cond[opt.Order.Key] = bson.M{opt.Order.Key: bson.M{"$lt": opt.FromId}}
		}
	}

	scanOpt := options.Find()
	if opt.Order != nil && opt.Order.Key != "" {
		scanOpt.SetSort(bson.D{{opt.Order.Key, opt.Order.Dir}})
	}
	if opt.CursorMode {
		if opt.CursorExpireTime > 0 {
			scanOpt.SetMaxTime(opt.CursorExpireTime)
			scanOpt.SetCursorType(options.NonTailable)
		} else if opt.CursorExpireTime == 0 {
			scanOpt.SetCursorType(options.NonTailable)
		} else if opt.CursorExpireTime < 0 {
			scanOpt.SetMaxTime(time.Second)
			scanOpt.SetCursorType(options.NonTailable)
			scanOpt.SetNoCursorTimeout(true)
		}
		var limit int64 = 0
		scanOpt.Limit = &limit
		opt.Limit = 0
	} else {
		scanOpt.SetLimit(opt.Limit)
	}

	if len(opt.Cols) > 0 {
		cols := bson.M{}
		var findOrderKey bool
		for _, col := range opt.Cols {
			cols[col] = 1
			if col == opt.Order.Key {
				findOrderKey = true
			}
		}
		if !findOrderKey && opt.Order.Key != "" && opt.Order.Key != "_id" {
			cols[opt.Order.Key] = 1
		}
		scanOpt.SetProjection(cols)
	}

	if opt.CursorMode {
		go func() {
			cur, err := db.Collection(coll).Find(ctx, opt.Cond, scanOpt)
			if err != nil {
				log.Fatal(err)
			}
			defer func() {
				if buff != nil {
					close(buff)
				}
				cur.Close(ctx)
			}()
			for {
				if cur.Next(ctx) {
					data := bson.M{}
					err = cur.Decode(&data)
					if err != nil {
						log.Fatal(err)
						return
					}
					// 将数据投递到chan中
					buff <- data
				} else {
					log.Println("cursorEnd", cur.Err())
					break
				}
			}
		}()

	} else {
		go func() {
			defer func() {
				if buff != nil {
					close(buff)
				}
			}()

			for {
				var cnt int
				cur, err := db.Collection(coll).Find(ctx, opt.Cond, scanOpt)
				if err != nil {
					panic(err)
				}

				var next interface{}
				for cur.Next(ctx) {
					cnt++
					data := bson.M{}
					err = cur.Decode(&data)
					if err != nil {
						log.Fatal(err)
						return
					}
					next = data[opt.Order.Key]
					// 将数据投递到chan中
					buff <- data
				}
				cur.Close(ctx)
				if cnt == 0 {
					// 跑完收工
					break
				}
				// 判断条件中是否包含排序字段, 如果包含则不能修改
				if opt.HasConfirmedValue(opt.Order.Key) {
					// 通过skip方式进行分页
					scanOpt.SetSkip(int64(cnt))
				} else {
					if opt.Order.Dir > 0 {
						opt.Cond[opt.Order.Key] = bson.M{"$gt": next}
					} else {
						opt.Cond[opt.Order.Key] = bson.M{"$lt": next}
					}
				}
			}
		}()
	}

	return
}

func (s *Scanner) Run(ctx context.Context, dataHandler func(data interface{})) error {
	defer func() {
		s.panicHandler(ctx)
	}()

	s.beforeStartHook(ctx)
	wg := sync.WaitGroup{}

	pool := runtime.Pool(ctx, s.name+"Pool", runtime.PoolConfig{
		Size: s.poolOpt.Size,
		Handler: func(i interface{}) {
			defer wg.Done()
			dataHandler(i)
		},
		Blocking:         s.poolOpt.Blocking,
		MaxBlockingTasks: s.poolOpt.MaxBlockingTasks,
		ExpiryDuration:   s.poolOpt.ExpiryDuration,
		PreAlloc:         s.poolOpt.PreAlloc,
		PanicHandler:     s.poolOpt.PanicHandler,
	})

	for item := range Scan(ctx, s.db, s.coll, s.scanOpt) {
		data, ignore, err := s.filter(item)

		s.counter(ctx, item)
		if ignore {
			continue
		}
		if err != nil {
			s.errHandler(ctx, err, "filter error", item)
			if s.stopWhenErr {
				return err
			}
			continue
		}

		if s.limiter != nil {
			err = s.limiter.Wait(ctx)
			if err != nil {
				su_logger.Error(ctx, err, "limiter error")
			}
		}

		if full, items := s.stash.Append(data); full {
			wg.Add(1)
			err = pool.Invoke(items)
			if err != nil {
				s.errHandler(ctx, err, "pool invoke error", items)
				if s.stopWhenErr {
					return err
				}
			}
		}
	}

	if !s.stash.IsEmpty() {
		wg.Add(1)
		_ = pool.Invoke(s.stash.items)
	}

	wg.Wait()
	pool.Release()
	s.finishHook(ctx)

	return nil
}

func New(name string) *Scanner {
	s := &Scanner{
		timeFrom: time.Now(),
		name:     name,
	}

	s.counter = func(ctx context.Context, i interface{}) {
		s.cnt++
		if s.cnt%1000 == 0 {
			su_logger.Info(ctx, name+" scanned", su_logger.E().Int("cnt", s.cnt))
		}
	}

	s.stash = &Stash{
		size:  500,
		items: make([]interface{}, 0, 500),
	}

	s.poolOpt = &PoolOption{
		Size:     500,
		Blocking: true,
		PreAlloc: true,
	}

	s.errHandler = func(ctx context.Context, err error, msg string, data interface{}) {
		su_logger.Error(ctx, err, msg, su_logger.E().Any("data", data))
	}

	s.beforeStartHook = func(ctx context.Context) {
		su_logger.Info(ctx, "------------- begin "+s.name)
	}

	s.finishHook = func(ctx context.Context) {
		if s.endWithNotify {
			su_logger.InfoWithNotify(ctx, "------------- finish "+s.name, su_logger.E().String("usage", time.Since(s.timeFrom).String()))
		} else {
			su_logger.Info(ctx, "------------- finish "+s.name, su_logger.E().String("usage", time.Since(s.timeFrom).String()))
		}
	}

	s.panicHandler = func(ctx context.Context) {
		if err := recover(); err != nil {
			su_logger.Error(ctx, fmt.Errorf("%v", err), "panic", su_logger.E().Any("err", err))
		}
	}

	s.filter = func(i bson.M) (data interface{}, ignore bool, err error) {
		return i, false, nil
	}

	return s
}
