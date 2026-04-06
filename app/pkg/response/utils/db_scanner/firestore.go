package db_scanner

import (
	"context"
	"errors"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/google"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
	"google.golang.org/api/iterator"
)

type FiretoreScanner struct {
	*Base
}

func (m *FiretoreScanner) toWhere(c *firestore.CollectionRef, opt *ScanOption, fromId interface{}, limit int) firestore.Query {
	query := c.Query

	var order = firestore.Asc
	var orderField = firestore.DocumentID

	if len(opt.Cols) > 0 {
		query = query.Select(opt.Cols...)
	}
	for i := range opt.Cond {
		switch opt.Cond[i].Cond {
		case "==", "=":
			query.Where(opt.Cond[i].Field, "==", opt.Cond[i].Val)
		case ">":
			query.Where(opt.Cond[i].Field, ">", opt.Cond[i].Val)
		case ">=":
			query.Where(opt.Cond[i].Field, ">=", opt.Cond[i].Val)
		case "<":
			query.Where(opt.Cond[i].Field, "<", opt.Cond[i].Val)
		case "<=":
			query.Where(opt.Cond[i].Field, "<=", opt.Cond[i].Val)
		case "in":
			query.Where(opt.Cond[i].Field, "array-contains", opt.Cond[i].Val)
		case "not-in", "nin":
			query.Where(opt.Cond[i].Field, "not-in", opt.Cond[i].Val)
		case "not", "!=":
			query.Where(opt.Cond[i].Field, "!=", opt.Cond[i].Val)
		default:
			panic("not support cond: " + opt.Cond[i].Cond)
		}
	}
	if opt.Order != nil && opt.Order.Key != "" {
		orderField = opt.Order.Key
		if opt.Order.Dir == 1 {
			order = firestore.Asc
		} else {
			order = firestore.Desc
		}
	}

	if orderField == firestore.DocumentID {
		if order == firestore.Asc {
			query = query.OrderBy(orderField, order)
			if !isEmpty(fromId) {
				query = query.StartAfter(fromId)
			}
		} else {
			query = query.OrderBy(orderField, order)
			if !isEmpty(fromId) {
				query = query.EndBefore(fromId)
			}
		}
	} else {
		query = query.OrderBy(orderField, order)

		var op string
		if order == firestore.Asc {
			op = ">"
			if !isEmpty(fromId) {
				query = query.Where(orderField, op, fromId)
			}
		} else {
			op = "<"
			if !isEmpty(fromId) {
				query = query.Where(orderField, op, fromId)
			}
		}
	}

	query = query.Limit(limit)

	return query
}

func (m *FiretoreScanner) scan(ctx context.Context) (buff chan Data) {
	db, ok := m.db.(*google.Firestore)
	if !ok {
		panic("db type error")
	}
	opt := m.scanOpt
	coll := db.Collection(m.coll)

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

	buff = make(chan Data, bufSize)
	go func() {
		defer func() {
			if buff != nil {
				close(buff)
			}
		}()
		for {
			it := m.toWhere(coll, opt, fromId, limit).Documents(ctx)
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
				d := Data{
					Ref: doc,
				}

				buff <- d

				if opt != nil && opt.GetNext != nil {
					fromId, err = opt.GetNext(&d)
					if err != nil || isEmpty(fromId) {
						su_logger.Error(ctx, err, "get next id failed")
						panic("get next id failed")
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

func NewFirestore(scannerName string, db *google.Firestore, coll string, opt *ScanOption) Scanner {
	dftB := dftBase()
	s := &FiretoreScanner{
		Base: dftB,
	}
	s.name = scannerName
	s.db = db

	scanOpt := dftScanOption()
	scanOpt.Apply(opt)
	s.scanOpt = scanOpt

	s.coll = coll
	s.scanner = s.scan

	return s
}
