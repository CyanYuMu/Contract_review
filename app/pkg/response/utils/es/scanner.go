package es

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/tidwall/gjson"

	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/runtime"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/scanner"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
)

type Sort struct {
	// 排序字段
	Field string
	// > 0 代表升序, < 0 代表降序
	Direction int8
}

type ScanParam struct {
	// ? 查询语句, 建议使用es_builder进行构建
	QueryBody string
	// ? 自定义排序, 为空时, 默认使用 create_at 进行排序
	Sort []*Sort
	// ? 指定需要的列, 为空时, 默认获取所有列, 自定义排序列会合并到Source中
	Source []string
	// ? 每次遍历获取的行数, 默认 limit * 2
	PageSize int
	// ? 默认与PageSize一致
	BuffSize int
	// 定义如果获取下一个起始点
	// GetNext func(data string) string
	// ? 定义起始点, fromId 的值需要和Sort的数量一致
	After         []interface{}
	queryTemplate string
}

func (s *ScanParam) SetQueryTemplate(queryTemplate string) *ScanParam {
	if s == nil {
		return nil
	}
	s.queryTemplate = queryTemplate
	return s
}

func (s ScanParam) BuildAfter() string {
	after := strings.Builder{}
	if len(s.After) == 0 {
		return ""
	}
	for _, v := range s.After {
		if after.Len() > 0 {
			after.WriteString(",")
		}
		switch val := v.(type) {
		case string:
			after.WriteString(`"` + val + `"`)
		default:
			after.WriteString(fmt.Sprintf("%v", val))
		}
	}

	return `,"search_after":[` + after.String() + `]`
}

func (s ScanParam) BuildSort() string {
	sort := strings.Builder{}
	for idx, v := range s.Sort {
		if idx > 0 {
			sort.WriteString(",")
		}
		order := "asc"
		if v.Direction < 0 {
			order = "desc"
		}
		sort.WriteString(`{"` + v.Field + `":{"order":"` + order + `"}}`)
	}

	return sort.String()
}

func buildQueryTemplate(param *ScanParam) {
	if param == nil || param.queryTemplate != "" {
		return
	}
	if len(param.Sort) == 0 {
		param.Sort = []*Sort{
			{Field: "create_at", Direction: 1},
		}
	}
	if param.PageSize <= 0 {
		param.PageSize = 1000
	}
	if param.BuffSize <= 0 {
		param.BuffSize = param.PageSize
	}

	queryBody := param.QueryBody
	if queryBody == "" {
		queryBody = `{"match_all":{}}`
	}

	bdy := strings.Builder{}
	bdy.WriteString(`{"query":`)
	bdy.WriteString(queryBody)
	bdy.WriteString(`,"sort":[`)
	bdy.WriteString(param.BuildSort())
	bdy.WriteString(`],"size":`)
	bdy.WriteString(fmt.Sprintf("%d", param.PageSize))
	if len(param.Source) > 0 {
		bdy.WriteString(`,"_source":[`)
		for idx, src := range param.Source {
			if idx > 0 {
				bdy.WriteString(",")
			}
			bdy.WriteString(`"`)
			bdy.WriteString(src)
			bdy.WriteString(`"`)
		}
		bdy.WriteString(`]`)
	}
	// {$search_after} 占位符在闭合 } 之前，BuildAfter 返回 "" 或 ,"search_after":[...]
	bdy.WriteString(`{$search_after}}`)
	param.queryTemplate = bdy.String()
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
	counter func(ctx context.Context)
	// 数据过滤器
	filter func(i string) (data interface{}, ignore bool, err error)
	stash  *Stash

	cli     *elasticsearch.Client
	idx     string
	scanOpt *ScanParam
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

func (s *Scanner) Counter(fn func(ctx context.Context)) *Scanner {
	s.counter = fn

	return s
}

func (s *Scanner) Scan(cli *elasticsearch.Client, idx string, param ScanParam) *Scanner {
	s.cli = cli
	s.idx = idx
	s.scanOpt = &param
	buildQueryTemplate(s.scanOpt)

	return s
}

func Scan(ctx context.Context, cli *elasticsearch.Client, idx string, param *ScanParam) <-chan string {
	if param == nil {
		param = &ScanParam{}
	}
	buildQueryTemplate(param)
	out := make(chan string, param.BuffSize)

	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			body := strings.Replace(param.queryTemplate, "{$search_after}", param.BuildAfter(), 1)

			response, err := (&esapi.SearchRequest{
				Index: []string{idx},
				Body:  strings.NewReader(body),
			}).Do(ctx, cli)
			if err != nil {
				su_logger.Error(ctx, err, "es scan search error", su_logger.E().String("index", idx))
				return
			}
			if response == nil {
				su_logger.Error(ctx, nil, "es scan search nil response", su_logger.E().String("index", idx))
				return
			}

			raw, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if readErr != nil {
				su_logger.Error(ctx, readErr, "es scan read body failed", su_logger.E().String("index", idx))
				return
			}
			if response.IsError() {
				su_logger.Error(ctx, fmt.Errorf("es status %s", response.Status()),
					"es scan search error response",
					su_logger.E().String("index", idx).String("body", string(raw)))
				return
			}

			gData := gjson.ParseBytes(raw)
			hits := gData.Get("hits.hits").Array()
			for _, value := range hits {
				source := value.Get("_source").String()
				select {
				case out <- source:
				case <-ctx.Done():
					return
				}
			}
			numItems := len(hits)

			if numItems < param.PageSize {
				break
			}

			lastSort := hits[numItems-1].Get("sort")
			if !lastSort.Exists() {
				break
			}
			after := make([]interface{}, 0, len(lastSort.Array()))
			lastSort.ForEach(func(_, value gjson.Result) bool {
				if value.Type == gjson.Number {
					after = append(after, value.Num)
				} else {
					after = append(after, value.String())
				}
				return true
			})
			if len(after) == 0 {
				break
			}
			param.After = after
		}
	}()

	return out
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

	for item := range Scan(ctx, s.cli, s.idx, s.scanOpt) {
		data, ignore, err := s.filter(item)
		s.counter(ctx)
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

func NewScanner(name string) *Scanner {
	s := &Scanner{
		timeFrom: time.Now(),
		name:     name,
	}

	s.counter = func(ctx context.Context) {
		s.cnt++
		if s.cnt%1000 == 0 {
			su_logger.Info(ctx, s.name+" scanned", su_logger.E().Int("cnt", s.cnt))
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

	s.filter = func(i string) (data interface{}, ignore bool, err error) {
		return i, false, nil
	}

	return s
}
