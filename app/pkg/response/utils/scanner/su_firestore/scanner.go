package su_firestore

import (
	"cloud.google.com/go/firestore"
	"context"
	"errors"
	"fmt"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/google"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/runtime"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/scanner"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
	"google.golang.org/api/iterator"
	"reflect"
	"strings"
	"sync"
	"time"
)

func (s *Scanner) RateLimiter(limiter scanner.Limiter) *Scanner {
	s.limiter = limiter

	return s
}

type OrderBy struct {
	// 排序的字段
	Path string
	// true 代表 >= <= 会结合Dir进行判断, false 代表 >, <
	Include bool
	// 排序的方向
	Dir firestore.Direction
}

func isEmpty(i interface{}) bool {
	if i == nil {
		return true
	}

	if reflect.ValueOf(i).IsZero() {
		return true
	}

	return false
}

type ScanWhere struct {
	Path string
	Cond string
	Val  interface{}
}

type ScanOption struct {
	// 默认与Limit一致
	BuffSize int
	// 每次遍历获取的行数, 默认 limit * 2
	Limit int
	// 指定需要的列
	Cols []string
	// 定义如果获取下一个起始点
	GetNext func(data *firestore.DocumentSnapshot) interface{}
	// 定义起始点
	FromId interface{}
	// 自定义排序, 为空是, 使用DocumentId进行排序
	OrderBy *OrderBy
	// 自定义条件
	Cond []ScanWhere
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
	counter func(ctx context.Context, i *firestore.DocumentSnapshot)
	// 数据过滤器
	filter func(i *firestore.DocumentSnapshot) (data interface{}, ignore bool, err error)
	stash  *Stash

	db      *google.Firestore
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
	poolOpt       *FSPoolOption
	timeFrom      time.Time
	endWithNotify bool
}

func (s *Scanner) EndWithNotify(b bool) *Scanner {
	s.endWithNotify = b

	return s
}

type FSPoolOption struct {
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

func (s *Scanner) Pool(opt *FSPoolOption) *Scanner {
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

func (s *Scanner) Counter(fn func(ctx context.Context, i *firestore.DocumentSnapshot)) *Scanner {
	s.counter = fn

	return s
}

func (s *Scanner) Scan(db *google.Firestore, coll string, opt *ScanOption) *Scanner {
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

func (s *Scanner) Filter(fn func(i *firestore.DocumentSnapshot) (data interface{}, ignore bool, err error)) *Scanner {
	s.filter = fn

	return s
}

func (s *Scanner) Run(ctx context.Context, dataHandler func(data interface{})) error {
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()
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

func NewScanner(name string) *Scanner {
	s := &Scanner{
		timeFrom: time.Now(),
		name:     name,
	}

	s.counter = func(ctx context.Context, i *firestore.DocumentSnapshot) {
		s.cnt++
		if s.cnt%1000 == 0 {
			su_logger.Info(ctx, s.name+" scanned", su_logger.E().Int("cnt", s.cnt))
		}
	}

	s.stash = &Stash{
		size:  500,
		items: make([]interface{}, 0, 500),
	}

	s.poolOpt = &FSPoolOption{
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

	s.filter = func(i *firestore.DocumentSnapshot) (data interface{}, ignore bool, err error) {
		return i, false, nil
	}

	return s
}

func Scan(ctx context.Context, db *google.Firestore, coll string, opt *ScanOption) (buff chan *firestore.DocumentSnapshot) {
	// 获取集合
	c := db.Collection(coll)
	var limit = 500
	var bufSize = limit * 2
	var fromId interface{}
	if opt != nil {
		if opt.Limit > 0 {
			limit = opt.Limit
			bufSize = limit * 2
		}

		if opt.BuffSize > 0 {
			bufSize = opt.BuffSize
		}

		fromId = opt.FromId
	}

	buff = make(chan *firestore.DocumentSnapshot, bufSize)

	go func() {
		defer func() {
			if buff != nil {
				close(buff)
			}
		}()
		for {
			it := getQuery(c, opt, fromId, limit).Documents(ctx)
			var count int
			for {
				doc, err := it.Next()
				if errors.Is(err, iterator.Done) {
					break
				}
				if err != nil {
					if strings.Contains(err.Error(), "error reading from server: EOF") {
						it.Stop()
						su_logger.Warn(ctx, "get documents failed, retrying...", su_logger.E().Error(err))
						time.Sleep(time.Second * 2)
						continue
					} else {
						su_logger.Error(ctx, err, "get documents failed")
						it.Stop()
						return
					}

				}
				buff <- doc

				if opt != nil && opt.GetNext != nil {
					nextId := opt.GetNext(doc)
					if !isEmpty(nextId) {
						fromId = opt.GetNext(doc)
					} else {
						su_logger.Warn(ctx, "get next id failed", su_logger.E().String("doc_id", doc.Ref.ID))
					}
				} else {
					fromId = doc.Ref.ID
				}

				count++
			}
			it.Stop()

			if count < limit {
				return
			}

		}
	}()

	return
}

func getQuery(c *firestore.CollectionRef, opt *ScanOption, fromId interface{}, limit int) firestore.Query {
	var order = firestore.Asc
	var orderField = firestore.DocumentID
	var include = false

	query := c.Query
	if opt != nil {
		if len(opt.Cols) > 0 {
			query = query.Select(opt.Cols...)
		}
		if len(opt.Cond) > 0 {
			for i, _ := range opt.Cond {
				query = query.Where(opt.Cond[i].Path, opt.Cond[i].Cond, opt.Cond[i].Val)
			}
		}
		if opt.OrderBy != nil {
			order = opt.OrderBy.Dir
			orderField = opt.OrderBy.Path
			include = opt.OrderBy.Include
		}
	}

	if orderField == firestore.DocumentID {
		if order == firestore.Asc {
			query = query.OrderBy(orderField, order)
			if !isEmpty(fromId) {
				if include {
					query = query.StartAt(fromId)
				} else {
					query = query.StartAfter(fromId)
				}
			}
		} else {
			query = query.OrderBy(orderField, order)
			if !isEmpty(fromId) {
				if include {
					query = query.EndAt(fromId)
				} else {
					query = query.EndBefore(fromId)
				}
			}
		}
	} else {
		query = query.OrderBy(orderField, order)

		var op string
		if order == firestore.Asc {
			op = ">"
			if include {
				op = ">="
			}
			if !isEmpty(fromId) {
				query = query.Where(orderField, op, fromId)
			}
		} else {
			op = "<"
			if include {
				op = "<="
			}
			if !isEmpty(fromId) {
				query = query.Where(orderField, op, fromId)
			}
		}
	}

	query = query.Limit(limit)

	return query
}
