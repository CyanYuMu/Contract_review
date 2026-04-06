package db

import (
	"context"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"time"

	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_ytrace/instrumentation"

	"github.com/spf13/cast"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
	"golang.org/x/exp/slices"

	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/su_struct"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/su_time"
)

type MongoCli struct {
	*mongo.Client
	cnf *MongoConfig
}

func (m *MongoCli) DB(name string) *MongoDB {
	db := m.Client.Database(name)
	return &MongoDB{
		Database: db,
		mode:     m.cnf.Mode,
	}
}

type MongoDB struct {
	*mongo.Database
	mode Mode
}

type MongoTransactionOpt struct {
	// // 设置事务选项
	//	sessionOpts := options.Session().SetDefaultReadConcern(readconcern.Majority()).SetDefaultWriteConcern(writeconcern.New(writeconcern.WMajority()))
	SessionOpts *options.SessionOptions
	Timeout     time.Duration
	// 最大重试次数
	//MaxRetryTimes int
	// 重试间隔
	//RetryInterval time.Duration
}

func (m *MongoTransactionOpt) Apply(opt *MongoTransactionOpt) {
	if opt == nil {
		return
	}

	if opt.SessionOpts != nil {
		m.SessionOpts = opt.SessionOpts
	}

	if opt.Timeout > 0 {
		m.Timeout = opt.Timeout
	}

	//if opt.MaxRetryTimes > 0 {
	//	m.MaxRetryTimes = opt.MaxRetryTimes
	//}
	//
	//if opt.RetryInterval > 0 {
	//	m.RetryInterval = opt.RetryInterval
	//}
}

func GetDftMongoTransactionOpt() *MongoTransactionOpt {
	return &MongoTransactionOpt{
		SessionOpts: options.Session().SetDefaultReadConcern(readconcern.Majority()).SetDefaultWriteConcern(writeconcern.New(writeconcern.WMajority())),
		Timeout:     time.Second * 30,
		//MaxRetryTimes: 5,
		//RetryInterval: time.Millisecond * 100,
	}
}

func (m *MongoDB) RunTransaction(ctx context.Context, do func(ctx context.Context, tx mongo.SessionContext) (interface{}, error), opt *MongoTransactionOpt) (data interface{}, err error) {
	// standalone模式下不支持
	if m.mode == ModeStandAlone {
		return do(ctx, nil)
	}

	option := GetDftMongoTransactionOpt()
	option.Apply(opt)

	sess, err := m.Client().StartSession(option.SessionOpts)
	if err != nil {
		return nil, err
	}
	defer sess.EndSession(ctx)
	retryOpts := options.Transaction().SetWriteConcern(writeconcern.New(writeconcern.WMajority())).SetMaxCommitTime(&option.Timeout)

	data, err = sess.WithTransaction(ctx, func(sessCtx mongo.SessionContext) (interface{}, error) {
		return do(ctx, sessCtx)
	}, retryOpts)

	return data, err
}

// func (m *MongoDB) MongoBatchUpdate(ctx context.Context, coll *mongo.Collection, items []interface{}, opt *BatchUpdateOpt) error {
// 	if len(items) == 0 {
// 		return nil
// 	}
// 	dftO := dftBatchUpdateOpt()
// 	dftO.Apply(opt)

// 	bwItems := make([]mongo.WriteModel, 0, len(items))

// 	for _, item := range items {
// 		docId, ups := m.ToMongoUpdateField(item, dftO.FieldOpt)

// 		w := mongo.NewUpdateOneModel().SetFilter(bson.M{"_id": docId}).SetUpdate(ups)
// 		if _, ok := ups["$setOnInsert"]; ok {
// 			w.SetUpsert(true)
// 		}
// 		bwItems = append(bwItems, w)
// 	}

// 	o := options.BulkWrite()
// 	if dftO.IgnoreError != nil {
// 		o.SetOrdered(*dftO.IgnoreError)
// 	}

// 	_, err := coll.BulkWrite(ctx, bwItems, o)
// 	return err
// }

func (m *MongoDB) Count(ctx context.Context, table string, conds Conds) (int64, error) {
	coll := m.coll(table)

	filter := m.buildFilter(conds)

	count, err := coll.CountDocuments(ctx, filter)
	return count, err
}

var structFieldCache = su_struct.NewStructCache()

type Opt struct {
	// ? 自定义判断是否进行过滤
	EmptyIgnore func(tagName string, v interface{}) (ignore bool)
	// ? 标签tag名称
	Tag string
	// 忽略更新时间字段, 默认自动填充
	IgnoreUpdateTimeField bool
	// ? 仅更新指定字段
	Fields []string
	// ? 指定字段的操作映射
	FieldOpRef map[string]FieldOpFunc
}

type FieldOpFunc func(key string, v interface{}) (nv interface{}, op string)

var (
	// set unset inc mul push pushAll pull pullAll addToSet currentDate bit max min rename currentDate
	KeyOpSet FieldOpFunc = func(key string, v interface{}) (nv interface{}, op string) {
		return v, "$set"
	}
	KeyOpUnset FieldOpFunc = func(key string, v interface{}) (nv interface{}, op string) {
		return v, "$unset"
	}
	KeyOpInc FieldOpFunc = func(key string, v interface{}) (nv interface{}, op string) {
		return v, "$inc"
	}
	KeyOpMul FieldOpFunc = func(key string, v interface{}) (nv interface{}, op string) {
		return v, "$mul"
	}
	KeyOpPush FieldOpFunc = func(key string, v interface{}) (nv interface{}, op string) {
		return v, "$push"
	}
	KeyOpPushAll FieldOpFunc = func(key string, v interface{}) (nv interface{}, op string) {
		return v, "$pushAll"
	}
	KeyOpPull FieldOpFunc = func(key string, v interface{}) (nv interface{}, op string) {
		return v, "$pull"
	}
	KeyOpPullAll FieldOpFunc = func(key string, v interface{}) (nv interface{}, op string) {
		return v, "$pullAll"
	}
	KeyOpAddToSet FieldOpFunc = func(key string, v interface{}) (nv interface{}, op string) {
		return v, "$addToSet"
	}
	KeyOpCurrentDate FieldOpFunc = func(key string, v interface{}) (nv interface{}, op string) {
		return true, "$currentDate"
	}
	KeyOpMax FieldOpFunc = func(key string, v interface{}) (nv interface{}, op string) {
		return v, "$max"
	}
	KeyOpMin FieldOpFunc = func(key string, v interface{}) (nv interface{}, op string) {
		return v, "$min"
	}

	KeyOpSetOnInsert = func(key string, v interface{}) (nv interface{}, op string) {
		return v, "$setOnInsert"
	}
)

func (o *Opt) Apply(opt *Opt) {
	if opt == nil {
		return
	}
	if opt.EmptyIgnore != nil {
		o.EmptyIgnore = opt.EmptyIgnore
	}
	if opt.Tag != "" {
		o.Tag = opt.Tag
	}

	if opt.IgnoreUpdateTimeField {
		o.IgnoreUpdateTimeField = opt.IgnoreUpdateTimeField
	}

	if len(opt.Fields) > 0 {
		o.Fields = opt.Fields
	}

	if len(opt.FieldOpRef) > 0 {
		o.FieldOpRef = opt.FieldOpRef
	}
}

func dftMongoOpt() *Opt {
	return &Opt{
		EmptyIgnore:           nil,
		Tag:                   "bson",
		IgnoreUpdateTimeField: false,
	}
}

func (m *MongoDB) ToMongoUpdateField(data interface{}, option *Opt) (id string, up bson.M) {
	if data == nil {
		return
	}
	opt := dftMongoOpt()
	opt.Apply(option)
	up = bson.M{}
	m.doToMongoUpdateField(&up, &id, data, opt)

	return id, up
}

func (m *MongoDB) doToMongoUpdateField(up *bson.M, id *string, data interface{}, opt *Opt) {
	v := reflect.ValueOf(data)
	t := v.Type()

	// 判断是否是指针
	//if v.IsNil() {
	//	return
	//}

	if t.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		v = v.Elem()
		t = v.Type()
	}

	if t.Kind() == reflect.Ptr {
		v = v.Elem()
		t = v.Type()
		if v.IsNil() {
			return
		}
	}

	fieldData, err := structFieldCache.ExtractStructCache(data)
	if err != nil {
		panic(err)
	}

	if t.Kind() != reflect.Struct {
		panic("data must be struct")
	}

	// nF := v.NumField()
	// data = make(map[string]interface{}, nF)
	var containUpdateField bool
	fields := fieldData.Fields()
	for i := range fields {
		value := v.Field(i)
		if len(fields[i].Tags) == 0 {
			continue
		}
		var tagData = fields[i].Tags[opt.Tag]
		if tagData == nil {
			continue
		}

		tagItem, _ := tagData.Get("_raw_")
		if tagItem == nil || tagItem.Value == "" {
			continue
		}
		key := tagItem.Value
		if pos := strings.Index(key, ","); pos != -1 {
			if key[pos+1:] == "inline" {
				m.doToMongoUpdateField(up, id, value.Interface(), opt)
				continue
			}
			key = key[0:pos]
		}

		key = strings.TrimSpace(key)
		if key == "" || key == "-" {
			continue
		}

		if key == "_id" {
			*id = value.String()
			continue
		}
		vi := value.Interface()
		isZero := reflect.DeepEqual(vi, reflect.Zero(value.Type()).Interface())
		if len(opt.Fields) > 0 && slices.Contains(opt.Fields, key) {
			// 既然已经指定了特定的更新字段, 此时不管值是否是零值都要进行更新
			isZero = false
			continue
		}
		if (isZero && opt.EmptyIgnore != nil && !opt.EmptyIgnore(key, vi)) || !isZero {
			//up[key] = vi
			m.doFillToMongoUpdate(up, key, vi, opt)
			if key == UpdateField {
				containUpdateField = true
			}
		}
	}

	if len(*up) > 0 {
		if !containUpdateField && !opt.IgnoreUpdateTimeField {
			//up[UpdateField] = su_time.CurrentTimestampMilli()
			m.doFillToMongoUpdate(up, UpdateField, su_time.CurrentTimestampMilli(), opt)
		}
	}
}

func (m *MongoDB) doFillToMongoUpdate(data *bson.M, key string, v interface{}, opt *Opt) {
	if key == "" {
		return
	}
	op := "$set"
	if opt.FieldOpRef != nil {
		if curOp, ok := opt.FieldOpRef[key]; ok {
			v, op = curOp(key, v)
		}
	}

	if _, ok := (*data)[op]; !ok {
		(*data)[op] = bson.M{}
	}
	(*data)[op].(bson.M)[key] = v
}

func (b *MongoCli) ToInterfaceSlice(slice interface{}) []interface{} {
	s := reflect.ValueOf(slice)
	if s.Kind() != reflect.Slice {
		panic("ConvertToInterfaceSlice: value is not a slice")
	}

	result := make([]interface{}, s.Len())
	for i := 0; i < s.Len(); i++ {
		result[i] = s.Index(i).Interface()
	}
	return result
}

type Mode string

const (
	ModeMasterSlave Mode = "MS"
	ModeCluster     Mode = "C"
	ModeStandAlone  Mode = "SA"
)

type MongoConfig struct {
	Address    string
	Addresses  []string
	Credential *MongoCredential
	// 最大连接池, 默认100
	MaxPoolSize int
	// 最大空闲时间, 秒, 默认5分钟
	MaxConnIdleSec int
	// 超时时长 秒, 默认0
	ConnectTimeoutSec int
	// 最大连接中的数量, 默认10
	MaxConnecting int
	// 默认集群模式
	Mode Mode
	Db   string
	// 主库优先, 仅对主从模式生效
	PrimaryPreferred *bool
}

type MongoCredential struct {
	AuthSource string
	Username   string
	Password   string
	// 加密算法, 默认SCRAM-SHA-256"
	AuthMechanism string
}

func (m *MongoConfig) Apply(cfg *MongoConfig) {
	if cfg == nil {
		return
	}

	if cfg.Address != "" {
		m.Address = cfg.Address
	}

	if len(cfg.Addresses) > 0 {
		m.Addresses = cfg.Addresses
	}

	if cfg.Credential != nil {
		m.Credential = cfg.Credential
	}

	if cfg.MaxPoolSize > 0 {
		m.MaxPoolSize = cfg.MaxPoolSize
	}

	if cfg.MaxConnIdleSec > 0 {
		m.MaxConnIdleSec = cfg.MaxConnIdleSec
	}

	if cfg.ConnectTimeoutSec > 0 {
		m.ConnectTimeoutSec = cfg.ConnectTimeoutSec
	}

	if cfg.MaxConnecting > 0 {
		m.MaxConnecting = cfg.MaxConnecting
	}

	if cfg.Mode != "" {
		m.Mode = cfg.Mode
	}

	if cfg.PrimaryPreferred != nil {
		m.PrimaryPreferred = cfg.PrimaryPreferred
	}
}

func GetDftMongoConfig() *MongoConfig {
	return &MongoConfig{
		MaxPoolSize:       100,
		MaxConnIdleSec:    300,
		ConnectTimeoutSec: 0,
		MaxConnecting:     10,
	}
}

func NewMongoClient(ctx context.Context, config *MongoConfig) (*MongoCli, error) {
	conf := GetDftMongoConfig()
	conf.Apply(config)
	var opt *options.ClientOptions
	if len(conf.Addresses) > 0 {
		opt = options.Client().ApplyURI(fmt.Sprintf("mongodb://%s", strings.Join(conf.Addresses, ",")))
	} else {
		opt = options.Client().ApplyURI(fmt.Sprintf("mongodb://%s", conf.Address))
	}
	if conf.Credential != nil {
		opt.SetAuth(options.Credential{
			AuthSource:    conf.Credential.AuthSource,
			Username:      url.QueryEscape(conf.Credential.Username),
			Password:      url.QueryEscape(conf.Credential.Password),
			AuthMechanism: conf.Credential.AuthMechanism,
		})
	}
	opt.SetMaxPoolSize(cast.ToUint64(conf.MaxPoolSize))
	opt.SetMaxConnecting(cast.ToUint64(conf.MaxConnecting))
	opt.SetMaxConnIdleTime(time.Second * time.Duration(conf.MaxConnIdleSec))
	opt.SetConnectTimeout(time.Second * time.Duration(conf.ConnectTimeoutSec))
	if len(conf.Addresses) > 0 && conf.Mode == ModeMasterSlave {
		if conf.PrimaryPreferred != nil && *conf.PrimaryPreferred {
			opt.SetReadPreference(readpref.PrimaryPreferred())
		} else {
			opt.SetReadPreference(readpref.SecondaryPreferred())
		}
	}

	// 启用 MongoDB 追踪
	var mongoEndpoint string
	if len(conf.Addresses) > 0 {
		mongoEndpoint = "mongodb://" + conf.Addresses[0]
	} else {
		mongoEndpoint = "mongodb://" + conf.Address
	}
	opt.SetMonitor(instrumentation.NewMongoMonitor(mongoEndpoint).CommandMonitor())

	cli, err := mongo.Connect(ctx, opt)
	if err != nil {
		return nil, err
	}

	return &MongoCli{Client: cli, cnf: conf}, nil
}

type MGFilterOption struct {
	Filter   func(v interface{}) interface{}
	KeepZero bool
}

type MGFilter struct {
	filter bson.M
}

func NewMGFilter() *MGFilter {
	return &MGFilter{
		filter: make(bson.M, 4),
	}
}

func (m *MGFilter) Get() bson.M {
	return m.filter
}

func (m *MGFilter) Where(field string, cond string, val interface{}, option ...*MGFilterOption) *MGFilter {
	var opt *MGFilterOption
	if len(option) > 0 && option[0] != nil {
		opt = option[0]
		if opt.Filter != nil {
			val = opt.Filter(val)
		}
	}

	isZero := isZeroValue(val)

	if (opt != nil && opt.KeepZero && isZero) || !isZero {
		switch cond {
		case "eq", "=", "==":
			m.filter[field] = val
		case "ne", "!=":
			m.filter[field] = bson.M{"$ne": val}
		case "gt", ">":
			m.filter["gt"] = bson.M{"$gt": val}
		case "lt", "<":
			m.filter[field] = bson.M{"$lt": val}
		case "gte", ">=":
			m.filter[field] = bson.M{"$gte": val}
		case "lte", "<=":
			m.filter[field] = bson.M{"$lte": val}
		case "in", "array-contains":
			m.filter[field] = bson.M{"$in": val}
		default:
			panic("not support cond")
		}
	}

	return m
}

func isZeroValue(val interface{}) bool {
	if val == nil {
		return true
	}

	value := reflect.ValueOf(val)
	switch value.Kind() {
	case reflect.Ptr, reflect.Interface:
		return value.IsNil()
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return value.Len() == 0
	}

	zeroValue := reflect.Zero(value.Type()).Interface()
	return reflect.DeepEqual(val, zeroValue)
}

//type MongoDB struct {
//	client *mongo.Client
//	db     *mongo.Database
//}

func NewMongoDB(client *mongo.Client, dbName string) *MongoDB {
	db := &MongoDB{
		Database: client.Database(dbName),
	}

	return db
}

func (m *MongoDB) coll(table string) *mongo.Collection {
	return m.Database.Collection(table)
}

// Insert implements DB interface
func (m *MongoDB) Insert(ctx context.Context, table string, row Row, options ...*InsertOptions) (string, error) {
	result, err := m.coll(table).InsertOne(ctx, row)
	opt := dftInsertOptions()
	opt.Apply(options...)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			// If duplicate key, try to update
			filter := bson.M{"_id": row.ID()}
			_, err = m.coll(table).ReplaceOne(ctx, filter, row)
			if err != nil {
				return "", err
			}
			// 调用Hook
			if len(options) > 0 && options[0] != nil && options[0].ChangeHook != nil {
				options[0].ChangeHook(ctx, ChangeRow{
					Table:  table,
					Id:     row.ID(),
					Action: ActionAdd,
				})
			}
			return row.ID(), nil
		}
		return "", err
	}
	id, ok := result.InsertedID.(string)
	// 调用Hook
	if opt.ChangeHook != nil {
		opt.ChangeHook(ctx, ChangeRow{
			Table:  table,
			Id:     id,
			Action: ActionAdd,
		})
	}
	if ok {
		return id, nil
	}
	return "", nil
}

// Delete implements DB interface
func (m *MongoDB) Delete(ctx context.Context, table string, id string, options ...*DeleteOptions) (int, error) {
	opt := dftDeleteOptions()
	opt.Apply(options...)
	result, err := m.coll(table).DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return 0, err
	}
	// 调用Hook
	if opt.ChangeHook != nil {
		opt.ChangeHook(ctx, ChangeRow{
			Table:  table,
			Id:     id,
			Action: ActionDelete,
		})
	}
	return int(result.DeletedCount), nil
}

// Create implements DB interface
func (m *MongoDB) Create(ctx context.Context, table string, row Row, options ...*CreateOptions) (*CreateResult, error) {
	opt := dftCreateOptions()
	opt.Apply(options...)
	result, err := m.coll(table).InsertOne(ctx, row)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, ErrDocAlreadyExists
		}
		return nil, err
	}

	if id, ok := result.InsertedID.(string); ok {
		// 调用Hook
		if opt.ChangeHook != nil {
			opt.ChangeHook(ctx, ChangeRow{
				Table:  table,
				Id:     id,
				Action: ActionAdd,
			})
		}
		return &CreateResult{ID: id}, nil
	}
	return &CreateResult{ID: row.ID()}, nil
}

func (m *MongoDB) toDBUpdate(updates []Update, forceOverwriteByOp string, upData *bson.M) error {
	for _, u := range updates {
		if forceOverwriteByOp != "" {
			if (*upData)[forceOverwriteByOp] == nil {
				(*upData)[forceOverwriteByOp] = bson.M{}
			}

			for _, ups := range updates {
				(*upData)[forceOverwriteByOp].(bson.M)[ups.Field] = ups.Value
			}

			return nil
		}
		switch u.Op {
		case "incr":
			if (*upData)["$inc"] == nil {
				(*upData)["$inc"] = bson.M{}
			}
			(*upData)["$inc"].(bson.M)[u.Field] = u.Value
		case "=", "set", "":
			if (*upData)["$set"] == nil {
				(*upData)["$set"] = bson.M{}
			}
			(*upData)["$set"].(bson.M)[u.Field] = u.Value
		case "setOnInsert":
			if (*upData)["$setOnInsert"] == nil {
				(*upData)["$setOnInsert"] = bson.M{}
			}
			(*upData)["$setOnInsert"].(bson.M)[u.Field] = u.Value
		default:
			return fmt.Errorf("not support op: %s", u.Op)
		}
	}
	return nil
}

func (m *MongoDB) UpsertSingleField(ctx context.Context, table string, row UpsertSingleFields, ops ...*UpsertSingleFieldRowOptions) (err error) {
	coll := m.coll(table)
	var ups = bson.M{}
	var mergeAll = true
	if len(ops) > 0 && ops[0] != nil {
		mergeAll = !ops[0].DisableMergeAll
	}
	for key, val := range row.Fields {
		ups[key] = val
	}

	_, err = coll.UpdateOne(ctx, bson.M{"_id": row.Id}, ups, &options.UpdateOptions{
		Upsert: &mergeAll,
	})
	if err != nil {
		return err
	}
	return nil
}

func (m *MongoDB) Upsert(ctx context.Context, table string, row UpsertRow, ops ...*UpsertOptions) (*UpsertRs, error) {
	opt := dftUpsertOptions()
	opt.Apply(ops...)
	coll := m.coll(table)
	var ups = bson.M{}
	err := m.toDBUpdate(row.Updates, "", &ups)
	if err != nil {
		return nil, err
	}
	err = m.toDBUpdate(row.Inserts, "$setOnInsert", &ups)
	if err != nil {
		return nil, err
	}

	upsert := true
	rs, err := coll.UpdateOne(ctx, bson.M{"_id": row.Id}, ups, &options.UpdateOptions{
		Upsert: &upsert,
	})
	if err != nil {
		return nil, err
	}
	if rs.UpsertedID != nil {
		id := rs.UpsertedID.(string)
		// hook
		if opt.ChangeHook != nil {
			opt.ChangeHook(ctx, ChangeRow{
				Table:  table,
				Id:     id,
				Action: ActionAdd,
			})
		}
		return &UpsertRs{
			Id:         id,
			MatchCount: rs.MatchedCount,
		}, nil
	} else {
		// hook
		if opt.ChangeHook != nil {
			opt.ChangeHook(ctx, ChangeRow{
				Table:  table,
				Id:     row.Id,
				Action: ActionUpdate,
				Data:   updatesToMapStringInterface(row.Updates),
			})
		}
	}
	return &UpsertRs{
		Id: row.Id,
	}, nil
}

// Update implements DB interface
func (m *MongoDB) Update(ctx context.Context, table string, update UpdateOne, options ...*UpdateOptions) error {
	opt := dftUpdateOptions()
	opt.Apply(options...)
	updateDoc := bson.M{}
	for _, u := range update.Updates {
		switch u.Op {
		case "incr":
			if updateDoc["$inc"] == nil {
				updateDoc["$inc"] = bson.M{}
			}
			updateDoc["$inc"].(bson.M)[u.Field] = u.Value
		case "addSet":
			val := reflect.ValueOf(u.Value)
			if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
				return fmt.Errorf("Value for addSet must be array type, got %T", u.Value)
			}

			if updateDoc["$addToSet"] == nil {
				updateDoc["$addToSet"] = bson.M{}
			}
			// 将值包装到 $each 中，支持批量添加（类似 Firestore 的 arrayUnion）
			updateDoc["$addToSet"].(bson.M)[u.Field] = bson.M{
				"$each": u.Value, // 要求 u.Value 是数组类型
			}
		default:
			if updateDoc["$set"] == nil {
				updateDoc["$set"] = bson.M{}
			}
			updateDoc["$set"].(bson.M)[u.Field] = u.Value
		}
	}

	result, err := m.coll(table).UpdateOne(ctx, bson.M{"_id": update.ID}, updateDoc)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrDocNotFound
	}
	// hook
	if opt.ChangeHook != nil {
		opt.ChangeHook(ctx, ChangeRow{
			Table:  table,
			Id:     update.ID,
			Action: ActionUpdate,
			Data:   updatesToMapStringInterface(update.Updates),
		})
	}
	return nil
}

// Get implements DB interface
func (m *MongoDB) Get(ctx context.Context, table string, id string, options ...*GetOptions) (*DocumentRef, error) {
	findOptions := toMongoFindOneOptions(options)

	// var result bson.M
	result := m.coll(table).FindOne(ctx, bson.M{"_id": id}, findOptions)
	if result.Err() != nil {
		if result.Err() == mongo.ErrNoDocuments {
			return nil, ErrDocNotFound
		}
		return nil, result.Err()
	}

	raw, err := result.DecodeBytes()
	if err != nil {
		return nil, err
	}

	ref := &DocumentRef{
		ID:   id,
		Data: raw,
	}

	return ref, nil
}

func (m *MongoDB) buildFilter(conds Conds) bson.M {
	filter := bson.M{}
	for _, cond := range conds {
		fieldName := cond.Field

		switch strings.ToLower(cond.Cond) {
		case "in":
			m.setFilterCondition(&filter, fieldName, "$in", cond.Value)
		case ">":
			m.setFilterCondition(&filter, fieldName, "$gt", cond.Value)
		case ">=":
			m.setFilterCondition(&filter, fieldName, "$gte", cond.Value)
		case "<":
			m.setFilterCondition(&filter, fieldName, "$lt", cond.Value)
		case "<=":
			m.setFilterCondition(&filter, fieldName, "$lte", cond.Value)
		case "not-in":
			m.setFilterCondition(&filter, fieldName, "$nin", cond.Value)
		case "!=":
			m.setFilterCondition(&filter, fieldName, "$ne", cond.Value)
		default:
			// 对于等于条件，直接设置值
			filter[fieldName] = cond.Value
		}
	}
	return filter
}

// setFilterCondition 设置过滤条件，支持同一字段的多个条件合并
func (m *MongoDB) setFilterCondition(filter *bson.M, field, operator string, value interface{}) {
	if existingValue, exists := (*filter)[field]; exists {
		// 如果字段已存在，检查是否为 bson.M 类型
		if existingConditions, ok := existingValue.(bson.M); ok {
			// 合并到现有条件中
			existingConditions[operator] = value
		} else {
			// 如果现有值不是 bson.M，创建新的条件映射
			(*filter)[field] = bson.M{
				"$eq":    existingValue, // 保留原有的等于条件
				operator: value,
			}
		}
	} else {
		// 字段不存在，创建新的条件
		(*filter)[field] = bson.M{operator: value}
	}
}

// Find implements DB interface
func (m *MongoDB) Find(ctx context.Context, table string, conds Conds, options ...*FindOptions) (*Iterator, error) {
	filter := m.buildFilter(conds)
	findOptions := toMongoFindOptions(options)

	cursor, err := m.coll(table).Find(ctx, filter, findOptions)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	rs := &Iterator{
		items: make([]*DocumentRef, 0),
	}
	for cursor.Next(ctx) {
		raw := cursor.Current
		id := cursor.Current.Lookup("_id")
		var idStr string
		switch id.Type {
		case bson.TypeObjectID:
			idStr = id.ObjectID().Hex()
		case bson.TypeString:
			idStr = id.StringValue()
		default:
			idStr = id.StringValue()
		}
		rs.items = append(rs.items, &DocumentRef{ID: idStr, Data: raw})
	}

	return rs, nil
}

func BatchWriteOptionToMongoOptions(opt *BatchWriteOptions) *options.BulkWriteOptions {
	if opt == nil {
		return nil
	}
	bulkOptions := &options.BulkWriteOptions{}
	if opt.ContinueOnError == 1 {
		ord := true
		bulkOptions.Ordered = &ord
	}
	if opt.ContinueOnExistsError == 1 {
		ord := true
		bulkOptions.Ordered = &ord
	}

	return bulkOptions
}

// BatchCreate implements DB interface
func (m *MongoDB) BatchCreate(ctx context.Context, table string, rows []Row, options ...*BatchWriteOptions) *BatchWriteResult {
	opt := dftBatchWriteOptions()
	opt.Apply(options...)

	result := &BatchWriteResult{
		Ids:    make([]string, 0, len(rows)),
		Errors: make([]error, 0),
	}

	dbOpt := BatchWriteOptionToMongoOptions(opt)

	batchSize := opt.BatchSize
	if batchSize <= 0 {
		batchSize = 30
	}

	for i := 0; i < len(rows); i += batchSize {
		end := i + batchSize
		if end > len(rows) {
			end = len(rows)
		}

		batch := rows[i:end]
		models := make([]mongo.WriteModel, len(batch))
		for j, row := range batch {
			models[j] = mongo.NewInsertOneModel().SetDocument(row)
		}

		bulkResult, err := m.coll(table).BulkWrite(ctx, models, dbOpt)
		if err != nil {
			if opt.ContinueOnExistsError == 1 && mongo.IsDuplicateKeyError(err) {
				err = nil
			}
			result.Errors = append(result.Errors, err)
			if err != nil && opt.ContinueOnError != 1 {
				return result
			}
		}

		if bulkResult != nil {
			result.Affected += bulkResult.InsertedCount
			for _, row := range batch {
				result.Ids = append(result.Ids, row.ID())
				// hook
				if opt.ChangeHook != nil {
					opt.ChangeHook(ctx, ChangeRow{
						Table:  table,
						Id:     row.ID(),
						Action: ActionAdd,
					})
				}
			}
		}
	}
	return result
}

// BatchInsert implements DB interface
func (m *MongoDB) BatchInsert(ctx context.Context, table string, rows []Row, options ...*BatchWriteOptions) *BatchWriteResult {
	opt := dftBatchWriteOptions()
	opt.Apply(options...)

	result := &BatchWriteResult{
		Ids:    make([]string, 0, len(rows)),
		Errors: make([]error, 0),
	}

	batchSize := opt.BatchSize
	if batchSize <= 0 {
		batchSize = 30
	}

	for i := 0; i < len(rows); i += batchSize {
		end := i + batchSize
		if end > len(rows) {
			end = len(rows)
		}

		batch := rows[i:end]
		models := make([]mongo.WriteModel, len(batch))
		for j, row := range batch {
			models[j] = mongo.NewReplaceOneModel().
				SetFilter(bson.M{"_id": row.ID()}).
				SetReplacement(row).
				SetUpsert(true)
		}

		bulkResult, err := m.coll(table).BulkWrite(ctx, models)
		if err != nil {
			result.Errors = append(result.Errors, err)
			if opt.ContinueOnError != 1 {
				return result
			} else {
				result.Errors = append(result.Errors, nil)
			}
		}

		if bulkResult != nil {
			result.Affected += bulkResult.ModifiedCount + bulkResult.UpsertedCount
			for _, id := range bulkResult.UpsertedIDs {
				if strID, ok := id.(string); ok {
					result.Ids = append(result.Ids, strID)
					// hook
					if opt.ChangeHook != nil {
						opt.ChangeHook(ctx, ChangeRow{
							Table:  table,
							Id:     strID,
							Action: ActionAdd,
						})
					}
				}
			}
		}
	}

	return result
}

// BatchDelete implements DB interface
func (m *MongoDB) BatchDelete(ctx context.Context, table string, ids []string, options ...*BatchWriteOptions) *BatchWriteResult {
	result := &BatchWriteResult{
		Errors: make([]error, 0),
	}

	opt := dftBatchWriteOptions()
	opt.Apply(options...)

	deleteResult, err := m.coll(table).DeleteMany(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		result.Errors = append(result.Errors, err)
		return result
	}
	// hook
	if opt.ChangeHook != nil {
		for _, id := range ids {
			opt.ChangeHook(ctx, ChangeRow{
				Table:  table,
				Id:     id,
				Action: ActionDelete,
			})
		}
	}

	result.Affected = deleteResult.DeletedCount
	return result
}

// func dftMongoOpt() *UpdateOptions {
// 	return &UpdateOptions{
// 		EmptyIgnore:           nil,
// 		Tag:                   "bson",
// 		IgnoreUpdateTimeField: false,
// 	}
// }

// func ToMongoUpdateField(data interface{}, option *UpdateOptions) (up bson.M) {
// 	opt := option.ToMongoOptions()

// 	v := reflect.ValueOf(data)
// 	t := v.Type()
// 	up = bson.M{}
// 	if t.Kind() == reflect.Ptr {
// 		if v.IsNil() {
// 			return
// 		}
// 		v = v.Elem()
// 		t = v.Type()
// 	}

// 	if t.Kind() != reflect.Struct {
// 		panic("data must be struct")
// 	}

// 	nF := v.NumField()
// 	data = make(map[string]interface{}, nF)
// 	var containUpdateField bool
// 	for i := 0; i < nF; i++ {
// 		field := t.Field(i)
// 		value := v.Field(i)

// 		key := field.Tag.Get(opt.Tag)
// 		if key == "" {
// 			continue
// 		}
// 		if pos := strings.Index(key, ","); pos != -1 {
// 			key = key[0:pos]
// 		}
// 		key = strings.TrimSpace(key)
// 		if key == "" || key == "-" {
// 			continue
// 		}

// 		if key == "_id" {
// 			continue
// 		}

// 		vi := value.Interface()
// 		isZero := reflect.DeepEqual(vi, reflect.Zero(value.Type()).Interface())
// 		if (isZero && opt.EmptyIgnore != nil && !opt.EmptyIgnore(key, vi)) || !isZero {
// 			up[key] = vi
// 			if key == UpdateField {
// 				containUpdateField = true
// 			}
// 		}
// 	}

// 	if len(up) > 0 {
// 		if !containUpdateField && !opt.IgnoreUpdateTimeField {
// 			up[UpdateField] = su_time.CurrentTimestampMilli()
// 		}
// 	}

// 	return up
// }

// BatchUpdate implements DB interface
func (m *MongoDB) BatchUpdate(ctx context.Context, table string, updates []UpdateOne, options ...*BatchWriteOptions) *BatchWriteResult {
	opt := dftBatchWriteOptions()
	opt.Apply(options...)

	dbOpt := BatchWriteOptionToMongoOptions(opt)

	result := &BatchWriteResult{
		Errors: make([]error, 0),
	}

	batchSize := opt.BatchSize
	if batchSize <= 0 {
		batchSize = 30
	}

	for i := 0; i < len(updates); i += batchSize {
		end := i + batchSize
		if end > len(updates) {
			end = len(updates)
		}

		batch := updates[i:end]
		models := make([]mongo.WriteModel, len(batch))
		for j, update := range batch {
			updateDoc := bson.M{}
			for _, u := range update.Updates {
				switch u.Op {
				case "incr":
					if updateDoc["$inc"] == nil {
						updateDoc["$inc"] = bson.M{}
					}
					updateDoc["$inc"].(bson.M)[u.Field] = u.Value
				case "addSet":
					val := reflect.ValueOf(u.Value)
					if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
						su_logger.Error(ctx, fmt.Errorf("Value for addSet must be array type, got %T", u.Value), "")
						continue
					}

					if updateDoc["$addToSet"] == nil {
						updateDoc["$addToSet"] = bson.M{}
					}
					// 将值包装到 $each 中，支持批量添加（类似 Firestore 的 arrayUnion）
					updateDoc["$addToSet"].(bson.M)[u.Field] = bson.M{
						"$each": u.Value, // 要求 u.Value 是数组类型
					}
				default:
					if updateDoc["$set"] == nil {
						updateDoc["$set"] = bson.M{}
					}
					updateDoc["$set"].(bson.M)[u.Field] = u.Value
				}
			}
			models[j] = mongo.NewUpdateOneModel().
				SetFilter(bson.M{"_id": update.ID}).
				SetUpdate(updateDoc)
		}

		bulkResult, err := m.coll(table).BulkWrite(ctx, models, dbOpt)
		if err != nil {
			result.Errors = append(result.Errors, err)
			if opt.ContinueOnExistsError != 1 {
				return result
			}
		}

		if bulkResult != nil {
			result.Affected += bulkResult.ModifiedCount
		}
		// hook
		if opt.ChangeHook != nil {
			for _, update := range batch {
				opt.ChangeHook(ctx, ChangeRow{
					Table:  table,
					Id:     update.ID,
					Action: ActionUpdate,
					Data:   updatesToMapStringInterface(update.Updates),
				})
			}
		}
	}

	return result
}

// BatchGet implements DB interface
func (m *MongoDB) BatchGet(ctx context.Context, table string, ids []string, options ...*GetOptions) (*Iterator, error) {
	findOptions := toMongoFindOptions([]*FindOptions{{
		Fields: func() []string {
			if len(options) > 0 && options[0] != nil {
				return options[0].Fields
			}
			return nil
		}(),
	}})

	cursor, err := m.coll(table).Find(ctx, bson.M{"_id": bson.M{"$in": ids}}, findOptions)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	it := &Iterator{
		items: make([]*DocumentRef, 0, len(ids)),
	}
	for cursor.Next(ctx) {
		raw := cursor.Current
		id := cursor.Current.Lookup("_id")
		var idStr string
		switch id.Type {
		case bson.TypeObjectID:
			idStr = id.ObjectID().Hex()
		case bson.TypeString:
			idStr = id.StringValue()
		default:
			idStr = id.StringValue()
		}
		it.items = append(it.items, &DocumentRef{
			ID:   idStr,
			Data: raw,
		})
	}

	return it, nil
}

func (m *MongoDB) BatchUpsert(ctx context.Context, table string, rows []UpsertRow, opts ...*BatchWriteOptions) (*BatchUpsertRs, error) {
	opt := dftBatchWriteOptions()
	opt.Apply(opts...)

	// 创建 MongoDB 批量写入选项
	var dbOpt *options.BulkWriteOptions

	result := &BatchUpsertRs{
		InsertCount: 0,
		UpdateCount: 0,
	}

	if len(rows) == 0 {
		return result, nil
	}

	coll := m.coll(table)
	var operations []mongo.WriteModel

	// 构建批量操作
	for _, row := range rows {
		var ups = bson.M{}

		// 处理更新操作
		err := m.toDBUpdate(row.Updates, "", &ups)
		if err != nil {
			su_logger.Error(ctx, err, "failed to process updates for batch upsert", su_logger.E().Any("row_id", row.Id))
			if opt.ContinueOnError != 1 {
				return nil, err
			}
			continue
		}

		// 处理插入操作
		err = m.toDBUpdate(row.Inserts, "$setOnInsert", &ups)
		if err != nil {
			su_logger.Error(ctx, err, "failed to process inserts for batch upsert", su_logger.E().Any("row_id", row.Id))
			if opt.ContinueOnError != 1 {
				return nil, err
			}
			continue
		}

		// 创建 upsert 操作
		upsert := true
		updateModel := mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": row.Id}).
			SetUpdate(ups).
			SetUpsert(upsert)

		operations = append(operations, updateModel)
	}

	if len(operations) == 0 {
		return result, nil
	}

	// 执行批量操作
	bulkResult, err := coll.BulkWrite(ctx, operations, dbOpt)
	if err != nil {
		su_logger.Error(ctx, err, "failed to execute batch upsert", su_logger.E().Any("table", table))
		return nil, err
	}

	// 统计结果
	if bulkResult.UpsertedCount > 0 {
		result.InsertCount = bulkResult.UpsertedCount
	}
	if bulkResult.ModifiedCount > 0 {
		result.UpdateCount = bulkResult.ModifiedCount
	}

	// 处理 change hook
	if opt.ChangeHook != nil {
		// 由于 MongoDB BulkWrite 无法直接区分插入和更新，我们简化处理
		// 对于 upsert 操作，统一使用 Update 动作
		for _, row := range rows {
			opt.ChangeHook(ctx, ChangeRow{
				Table:  table,
				Id:     row.Id,
				Action: ActionUpdate,
				Data:   updatesToMapStringInterface(row.Updates),
			})
		}
	}

	return result, nil
}

// Helper functions for MongoDB options
func toMongoFindOneOptions(opts []*GetOptions) *options.FindOneOptions {
	if len(opts) == 0 || opts[0] == nil {
		return nil
	}
	opt := opts[0]
	if opt == nil {
		return nil
	}

	findOpts := options.FindOne()
	if len(opt.Fields) > 0 {
		projection := bson.M{}
		for _, field := range opt.Fields {
			projection[field] = 1
		}
		findOpts.SetProjection(projection)
	}
	return findOpts
}

func toMongoFindOptions(opts []*FindOptions) *options.FindOptions {
	if len(opts) == 0 || opts[0] == nil {
		return nil
	}
	opt := opts[0]
	if opt == nil {
		return nil
	}

	findOpts := options.Find()
	if opt.Limit > 0 {
		findOpts.SetLimit(opt.Limit)
	}
	if opt.Offset > 0 {
		findOpts.SetSkip(opt.Offset)
	}
	if len(opt.Sorts) > 0 {
		sort := bson.D{}
		for _, s := range opt.Sorts {
			sort = append(sort, bson.E{Key: s.Field, Value: s.Order})
		}
		findOpts.SetSort(sort)
	}
	if len(opt.Fields) > 0 {
		projection := bson.M{}
		for _, field := range opt.Fields {
			projection[field] = 1
		}
		findOpts.SetProjection(projection)
	}
	return findOpts
}

func (m *MongoDB) ToUpdateOne(data any, options *UpdateOptions) (upData UpdateOne) {
	opt := dftMongoToUpdateOptions()
	opt.Apply(options)
	v := reflect.ValueOf(data)
	t := v.Type()
	if t.Kind() == reflect.Ptr {
		v = v.Elem()
		t = v.Type()
	}

	if t.Kind() != reflect.Struct {
		panic("data must be struct")
	}

	var containUpdateField bool
	processStruct(v, t, opt, &upData, &containUpdateField)

	if len(upData.Updates) > 0 {
		if !containUpdateField && !opt.IgnoreUpdateTimeField {
			upData.Updates = append(upData.Updates, Update{Field: UpdateField, Value: su_time.CurrentTimestampMilli()})
		}
	}

	return upData
}
