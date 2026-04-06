package db_scanner

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/runtime"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
	"go.mongodb.org/mongo-driver/bson"
)

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

type Base struct {
	// 扫描记录数
	scanCnt int
	// 处理记录数
	handleCnt int
	// 计数器
	counter Counter
	// 数据过滤器
	filter func(i Data) (data interface{}, ignore bool, err error)
	stash  *Stash
	// 数据库实例
	db      interface{}
	coll    string
	scanOpt *ScanOption
	name    string
	// 频率限制
	limiter Limiter
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
	buff          chan interface{}
	scanner       func(ctx context.Context) (buff chan Data)
}

type Data struct {
	ByteData []byte
	Ref      *firestore.DocumentSnapshot
}

func (d Data) Unmarshal(i interface{}) error {
	if len(d.ByteData) > 0 {
		return bson.Unmarshal(d.ByteData, i)
	} else if d.Ref != nil {
		err := d.Ref.DataTo(i)
		if err != nil {
			return err
		}

		return nil
	}

	return errors.New("nil data and ref")
}

func (s *Base) Pool(opt *PoolOption) *Base {
	s.poolOpt = opt

	return s
}

func (s *Base) StopWhenErr(bl bool) *Base {
	s.stopWhenErr = bl
	return s
}

func (s *Base) PanicHandler(fn func(ctx context.Context)) *Base {
	s.panicHandler = fn
	return s
}

type Counter func(ctx context.Context)

func (s *Base) Counter(fn Counter) *Base {
	s.counter = fn

	return s
}

type ErrHandler func(ctx context.Context, err error, msg string, data interface{})

func (s *Base) ErrHandler(fn ErrHandler) *Base {
	s.errHandler = fn

	return s
}

func (s *Base) BeforeStartHook(fn func(ctx context.Context)) *Base {
	s.beforeStartHook = fn

	return s
}

func (s *Base) FinishHook(fn func(ctx context.Context)) *Base {
	s.finishHook = fn

	return s
}

func (s *Base) StashSize(n int) *Base {
	s.stash.size = n
	s.stash.items = make([]interface{}, 0, n)

	return s
}

func (s *Base) Limiter(limiter Limiter) *Base {
	s.limiter = limiter

	return s
}

type Filter = func(i Data) (data interface{}, ignore bool, err error)

func (s *Base) Filter(fn Filter) *Base {
	s.filter = fn

	return s
}

//func (s *Base) Scan(ctx context.Context, coll string, opt *ScanOption) (buff chan interface{}) {
//	return nil
//}

func (s *Base) Run(ctx context.Context, dataHandler Handler) {
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

	for item := range s.scanner(ctx) {
		data, ignore, err := s.filter(item)

		s.counter(ctx)
		s.scanCnt++
		if ignore {
			continue
		}
		if err != nil {
			s.errHandler(ctx, err, "filter error", item)
			if s.stopWhenErr {
				return
			}
			continue
		}

		if s.limiter != nil {
			err = s.limiter.Wait(ctx)
			if err != nil {
				su_logger.Error(ctx, err, "limiter error")
			}
		}

		s.handleCnt++

		if full, items := s.stash.Append(data); full {
			wg.Add(1)
			err = pool.Invoke(items)
			if err != nil {
				s.errHandler(ctx, err, "pool invoke error", items)
				if s.stopWhenErr {
					return
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
}

type Handler = func(data interface{})

func isEmpty(i interface{}) bool {
	if i == nil {
		return true
	}

	if reflect.ValueOf(i).IsZero() {
		return true
	}

	return false
}
