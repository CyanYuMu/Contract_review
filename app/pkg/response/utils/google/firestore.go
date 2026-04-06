package google

import (
	"cloud.google.com/go/firestore"
	"context"
	firebase "firebase.google.com/go"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
	"google.golang.org/api/iterator"
	"reflect"
	"strings"
	"time"
)

type Firestore struct {
	*firestore.Client
}

func NewFirestore(ctx context.Context, app *firebase.App) (cli *Firestore, err error) {
	cli = &Firestore{}
	cli.Client, err = app.Firestore(ctx)

	return
}

type OrderBy struct {
	// 排序的字段
	Path string
	// true 代表 >= <= 会结合Dir进行判断, false 代表 >, <
	Include bool
	// 排序的方向
	Dir firestore.Direction
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
	// 特定id, document id, 此字段不为空时, 忽略其他所有条件
	Id string
	// 自定义排序, 为空是, 使用DocumentId进行排序
	OrderBy *OrderBy
	// 自定义条件
	Cond []ScanWhere
	// 默认1次
	TimeoutRetryTimes int
	// 默认5秒
	RetryInterval time.Duration
}

type ScanWhere struct {
	Path string
	Cond string
	Val  interface{}
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

func (f *Firestore) Scan(ctx context.Context, coll string, opt *ScanOption) (buff chan *firestore.DocumentSnapshot) {
	// 获取集合
	c := f.Collection(coll)
	var limit = 500
	var bufSize = limit * 2
	var fromId interface{}
	var maxRetryTimes = 1
	var retryInterval = 5 * time.Second

	if opt != nil {
		if !isEmpty(opt.Id) {
			buff = make(chan *firestore.DocumentSnapshot, 1)
			get, err := c.Doc(opt.Id).Get(ctx)
			if err != nil {
				su_logger.Error(ctx, err, "get document failed")
			} else {
				buff <- get
			}
			close(buff)
			return
		}

		if opt.Limit > 0 {
			limit = opt.Limit
			bufSize = limit * 2
		}

		if opt.BuffSize > 0 {
			bufSize = opt.BuffSize
		}

		if opt.RetryInterval > 0 {
			retryInterval = opt.RetryInterval
		}

		if opt.TimeoutRetryTimes > 0 {
			maxRetryTimes = opt.TimeoutRetryTimes
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
		var tryTimes int
		for {
			it := getQuery(c, opt, fromId, limit).Documents(ctx)
			var count int
			for {
				doc, err := it.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					if strings.Contains(err.Error(), "timed out") {
						tryTimes++
						if tryTimes > maxRetryTimes {
							// 超时场景, 重试次数超过最大重试次数, 直接返回
							su_logger.Error(ctx, err, "get documents timeout")
							return
						} else {
							// 超时场景, 尝试重试
							su_logger.Warn(ctx, "get documents timeout, retrying...", su_logger.E().Int("try_times", tryTimes))

							time.Sleep(retryInterval)
							break
						}

					} else {
						su_logger.Error(ctx, err, "get documents failed")
						return
					}
				}
				tryTimes = 0
				buff <- doc
				if opt != nil && opt.GetNext != nil {
					fromId = opt.GetNext(doc)
				} else {
					fromId = doc.Ref.ID
				}

				count++
			}

			if count < limit {
				su_logger.Debug(ctx, "scan stopped", su_logger.E().Int("count", count).Any("from_id", fromId))
				return
			}
		}
	}()

	return
}
