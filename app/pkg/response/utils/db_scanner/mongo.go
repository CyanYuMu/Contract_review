package db_scanner

import (
	"context"
	"errors"
	"fmt"
	"log"
	"reflect"
	"time"

	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/db"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsoncodec"
	"go.mongodb.org/mongo-driver/bson/bsonrw"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoScanner struct {
	*Base
}

func (m *MongoScanner) toWhere(condS []Cond) bson.M {
	where := bson.M{}

	var initWhere = func(field string) {
		if where[field] == nil {
			where[field] = bson.M{}
		}
	}

	for i := range condS {
		switch condS[i].Cond {
		case "==", "=":
			where[condS[i].Field] = condS[i].Val
		case ">":
			initWhere(condS[i].Field)
			where[condS[i].Field] = bson.M{"$gt": condS[i].Val}
		case ">=":
			initWhere(condS[i].Field)
			where[condS[i].Field] = bson.M{"$gte": condS[i].Val}
		case "<":
			initWhere(condS[i].Field)
			where[condS[i].Field] = bson.M{"$lt": condS[i].Val}
		case "<=":
			initWhere(condS[i].Field)
		case "in":
			initWhere(condS[i].Field)
			where[condS[i].Field] = bson.M{"$in": condS[i].Val}
		case "not-in", "nin":
			initWhere(condS[i].Field)
			where[condS[i].Field] = bson.M{"$nin": condS[i].Val}
		case "exists":
			initWhere(condS[i].Field)
			where[condS[i].Field] = bson.M{"$exists": true}
		case "not-exists", "nex":
			initWhere(condS[i].Field)
			where[condS[i].Field] = bson.M{"$exists": false}
		case "not":
			initWhere(condS[i].Field)
			where[condS[i].Field] = bson.M{"$ne": condS[i].Val}

		default:
			panic("not support cond: " + condS[i].Cond)
		}
	}

	//if orderKey != "" {
	//	initWhere(orderKey)
	//	where[orderKey] = bson.M{orderKeyCond: orderVal}
	//}

	return where
}

func (m *MongoScanner) scan(ctx context.Context) (buff chan Data) {
	dbInst, ok := m.db.(*db.MongoDB)
	if !ok {
		panic("db type error")
	}
	opt := m.scanOpt
	coll := m.coll

	buff = make(chan Data, opt.BuffSize)

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

	if opt.Cond == nil {
		opt.Cond = []Cond{}
	}

	if opt.FromId != nil {
		if opt.Order.Dir == 1 {
			opt.Cond = append(opt.Cond, Cond{Field: opt.Order.Key, Cond: ">", Val: opt.FromId})
		} else {
			opt.Cond = append(opt.Cond, Cond{Field: opt.Order.Key, Cond: "<", Val: opt.FromId})
		}
	}

	scanOpt := options.Find()
	if opt.Order != nil && opt.Order.Key != "" {
		scanOpt.SetSort(bson.D{{Key: opt.Order.Key, Value: opt.Order.Dir}})
	}
	if opt.CursorMode {
		if opt.CursorExpireTime > 0 {
			scanOpt.SetMaxTime(opt.CursorExpireTime)
			scanOpt.SetCursorType(options.NonTailable)
		} else if opt.CursorExpireTime == 0 {
			scanOpt.SetCursorType(options.NonTailable)
			scanOpt.SetNoCursorTimeout(true)
		} else if opt.CursorExpireTime < 0 {
			scanOpt.SetMaxTime(time.Second)
			scanOpt.SetCursorType(options.NonTailable)
			scanOpt.SetNoCursorTimeout(true)
		}
		var limit int64 = 0
		scanOpt.Limit = &limit
		opt.Limit = 0
	} else {
		scanOpt.SetLimit(int64(opt.Limit))
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

	where := m.toWhere(opt.Cond)

	if opt.CursorMode {
		go func() {
			cur, err := dbInst.Collection(coll).Find(ctx, where, scanOpt)
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
					var data = Data{
						ByteData: cur.Current,
					}
					err1 := cur.Decode(&data)
					if err1 != nil {
						cur.Close(ctx)
						panic(err1)
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
				cur, err := dbInst.Collection(coll).Find(ctx, where, scanOpt)
				if err != nil {
					panic(err)
				}

				var next interface{}
				for cur.Next(ctx) {
					cnt++
					var item = Data{
						ByteData: cur.Current,
					}
					next, err = opt.GetNext(&item)
					if err != nil || isEmpty(next) {
						cur.Close(ctx)
						su_logger.Error(ctx, err, "get next id failed")
						panic("get next id failed")
					}

					// 将数据投递到chan中
					buff <- item
				}
				cur.Close(ctx)
				if cnt == 0 {
					// 跑完收工
					break
				}

				ord := "$gt"
				if opt.Order.Dir == -1 {
					ord = "$lt"
				}
				if _, ok := where[opt.Order.Key]; ok {
					where[opt.Order.Key] = bson.M{ord: next}
				} else {
					where[opt.Order.Key] = bson.M{ord: next}
				}
			}
		}()
	}

	return
}

func NewMongo(scannerName string, dbInst *db.MongoDB, coll string, opt *ScanOption) Scanner {
	dftB := dftBase()
	s := &MongoScanner{
		Base: dftB,
	}
	s.name = scannerName
	s.db = dbInst

	scanOpt := dftScanOption()
	scanOpt.GetNext = func(data *Data) (next interface{}, err error) {
		if data == nil || data.ByteData == nil {
			return nil, errors.New("data is nil")
		}

		var item map[string]interface{}
		err = bson.Unmarshal(data.ByteData, &item)
		if err != nil {
			return nil, err
		}

		next = item["_id"]
		return next, nil
	}
	scanOpt.Apply(opt)
	s.scanOpt = scanOpt

	s.coll = coll
	s.scanner = s.scan

	return s
}

type Float64ToFloat32Codec struct{}

func (fic *Float64ToFloat32Codec) DecodeValue(dc bsoncodec.DecodeContext, vr bsonrw.ValueReader, val reflect.Value) error {
	if vr.Type() != bson.TypeDouble {
		return fmt.Errorf("expected double but got %v", vr.Type())
	}
	f, err := vr.ReadDouble()
	if err != nil {
		return err
	}
	val.SetFloat(float64(float32(f))) // 显式转换为 float32
	return nil
}

func (fic *Float64ToFloat32Codec) EncodeValue(ec bsoncodec.EncodeContext, vw bsonrw.ValueWriter, val reflect.Value) error {
	return vw.WriteDouble(float64(val.Float()))
}
