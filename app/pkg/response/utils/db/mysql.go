package db

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/spf13/cast"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/su_time"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/plugin/dbresolver"
)

// MysqlConfig
// @Description: mysql 连接配置, 替代DbGroup配置, 封装读写分离逻辑
type MysqlConfig struct {
	Dsn         string
	MaxOpenConn int
	MaxIdleConn int
	// 最大空闲时间, 单位秒
	MaxIdleTime time.Duration
	// 最大生命周期, 单位秒
	MaxLifeTime time.Duration
	Master      MysqlSingleConfig
	// 从节点配置
	Slave []MysqlSingleConfig
	// 慢查询阈值, 单位秒, 默认5
	SlowThreshold int
	// 4=>info(非 master,production 默认) 3=>Warn(master,production默认) 2=> Error 1=>Silent
	LogLevel int
}

type MysqlSingleConfig struct {
	// 可选
	Dsn      string
	User     string
	Password string
	Addr     string
	Db       string
	// 默认UTC
	Loc string
	// 默认30s
	Timeout time.Duration
}

func parseDsn(conf *MysqlSingleConfig) string {
	if conf.Dsn != "" {
		return conf.Dsn
	}
	loc := conf.Loc
	if loc == "" {
		loc = url.QueryEscape("UTC")
	}
	var timeout = 30
	if conf.Timeout > 0 {
		timeout = cast.ToInt(conf.Timeout.Seconds())
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=True&loc=%s&timeout=%ds",
		conf.User,
		conf.Password,
		conf.Addr,
		conf.Db,
		loc, timeout)

	return dsn
}

func NewMysql(config *MysqlConfig) (dbInst *gorm.DB, err error) {
	dsn := config.Master.Dsn
	if dsn == "" {
		dsn = parseDsn(&config.Master)
	}

	//level := glogger.Info
	//slow := time.Second * 5

	//if config.SlowThreshold > 0 {
	//	slow = time.Second * time.Duration(config.SlowThreshold)
	//}
	//
	//if config.LogLevel > 0 {
	//	level = glogger.LogLevel(config.LogLevel)
	//} else {
	//	stage := os.Getenv(enum.StageKey)
	//	if stage == enum.EvnStageProduction || stage == "master" {
	//		// 生产环境提升到warn级别
	//		level = glogger.Warn
	//	}
	//}
	//
	//if config.SlowThreshold > 0 {
	//	slow = time.Second * time.Duration(config.SlowThreshold)
	//}

	//l := &MysqlLogger{
	//	Level:         level,
	//	SlowThreshold: slow,
	//}
	dbInst, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return
	}

	var replicas = make([]gorm.Dialector, 0)
	if len(config.Slave) > 0 {
		for i := range config.Slave {
			slaveDsn := parseDsn(&config.Slave[i])
			replicas = append(replicas, mysql.Open(slaveDsn))
		}
	}
	idleConn := compareIntWithDefault(config.MaxIdleConn, 5, 100, 20)
	openConn := compareIntWithDefault(config.MaxOpenConn, 1, 200, 10)
	idleTime := compareDurationWithDefault(config.MaxIdleTime, 10, 1800, 60)
	lifeTime := compareDurationWithDefault(config.MaxLifeTime, 30, 3600, 1800)

	err = dbInst.Use(dbresolver.Register(dbresolver.Config{
		Sources:  []gorm.Dialector{mysql.Open(dsn)},
		Replicas: replicas,
	}).SetMaxIdleConns(idleConn).SetMaxOpenConns(openConn).SetConnMaxIdleTime(idleTime).SetConnMaxLifetime(lifeTime))
	if err != nil {
		return nil, err
	}

	sqlDB, err := dbInst.DB()
	if err != nil {
		return nil, err
	}

	err = sqlDB.Ping()

	return
}

func compareDurationWithDefault(i time.Duration, minV time.Duration, maxV time.Duration, dftV time.Duration) (t time.Duration) {
	defer func() {
		t = t * time.Second
	}()
	if i < minV {
		return dftV
	} else if maxV > 0 && i > maxV {
		return dftV
	}

	return i
}

func compareIntWithDefault(i int, minV int, maxV int, dftV int) int {
	if i < minV {
		return dftV
	} else if maxV > 0 && i > maxV {
		return dftV
	}

	return i
}

// MysqlDB implements DB interface on top of GORM MySQL
type MysqlDB struct {
	db *gorm.DB
}

func NewMysqlDB(db *gorm.DB) *MysqlDB { return &MysqlDB{db: db} }

// helpers
func rowToMap(row Row) (map[string]interface{}, error) {
	if row == nil {
		return nil, errors.New("nil row")
	}
	b, err := jsoniter.Marshal(row)
	if err != nil {
		return nil, err
	}
	m := map[string]interface{}{}
	if err := jsoniter.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	// ensure id present
	if _, ok := m["id"]; !ok {
		if row.ID() != "" {
			m["id"] = row.ID()
		}
	}
	return m, nil
}

func (m *MysqlDB) Insert(ctx context.Context, table string, row Row, options ...*InsertOptions) (string, error) {
	opt := dftInsertOptions()
	opt.Apply(options...)

	data, err := rowToMap(row)
	if err != nil {
		su_logger.Error(ctx, err, "mysql insert marshal failed")
		return "", err
	}

	// Upsert (overwrite on duplicate)
	onConflict := clause.OnConflict{UpdateAll: true}
	if err := m.db.WithContext(ctx).Table(table).Clauses(onConflict).Create(data).Error; err != nil {
		su_logger.Error(ctx, err, "mysql insert failed", su_logger.E().String("table", table))
		return "", err
	}
	if opt.Hook() != nil {
		opt.Hook()(ctx, ChangeRow{Table: table, Id: cast.ToString(data["id"]), Action: ActionAdd})
	}
	return cast.ToString(data["id"]), nil
}

func (m *MysqlDB) Delete(ctx context.Context, table string, id string, options ...*DeleteOptions) (int, error) {
	opt := dftDeleteOptions()
	opt.Apply(options...)
	tx := m.db.WithContext(ctx).Table(table).Where("id = ?", id).Delete(nil)
	if tx.Error != nil {
		su_logger.Error(ctx, tx.Error, "mysql delete failed", su_logger.E().String("table", table).String("id", id))
		return 0, tx.Error
	}
	if opt.Hook() != nil {
		opt.Hook()(ctx, ChangeRow{Table: table, Id: id, Action: ActionDelete})
	}
	return int(tx.RowsAffected), nil
}

func (m *MysqlDB) Create(ctx context.Context, table string, row Row, options ...*CreateOptions) (*CreateResult, error) {
	opt := dftCreateOptions()
	opt.Apply(options...)
	data, err := rowToMap(row)
	if err != nil {
		su_logger.Error(ctx, err, "mysql create marshal failed")
		return nil, err
	}
	// pure insert, duplicate -> error
	if err := m.db.WithContext(ctx).Table(table).Create(data).Error; err != nil {
		if IsAlreadyExists(err) || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return nil, ErrDocAlreadyExists
		}
		su_logger.Error(ctx, err, "mysql create failed", su_logger.E().String("table", table))
		return nil, err
	}
	id := cast.ToString(data["id"])
	if opt.Hook() != nil {
		opt.Hook()(ctx, ChangeRow{Table: table, Id: id, Action: ActionAdd})
	}
	return &CreateResult{ID: id}, nil
}

func (m *MysqlDB) Update(ctx context.Context, table string, update UpdateOne, options ...*UpdateOptions) error {
	opt := dftUpdateOptions()
	opt.Apply(options...)
	if update.ID == "" {
		return errors.New("empty id in UpdateOne")
	}
	sets := map[string]interface{}{}
	incrs := map[string]interface{}{}
	for _, u := range update.Updates {
		switch strings.ToLower(u.Op) {
		case "incr":
			incrs[u.Field] = u.Value
		default:
			sets[u.Field] = u.Value
		}
	}
	q := m.db.WithContext(ctx).Table(table).Where("id = ?", update.ID)
	if len(sets) > 0 {
		if err := q.Updates(sets).Error; err != nil {
			su_logger.Error(ctx, err, "mysql update set failed", su_logger.E().String("table", table).String("id", update.ID))
			return err
		}
	}
	if len(incrs) > 0 {
		for k, v := range incrs {
			if err := m.db.WithContext(ctx).Table(table).Where("id = ?", update.ID).UpdateColumn(k, gorm.Expr(k+" + ?", v)).Error; err != nil {
				su_logger.Error(ctx, err, "mysql update incr failed", su_logger.E().String("table", table).String("id", update.ID).String("field", k))
				return err
			}
		}
	}
	if opt.Hook() != nil {
		opt.Hook()(ctx, ChangeRow{Table: table, Id: update.ID, Action: ActionUpdate, Data: updatesToMapStringInterface(update.Updates)})
	}
	return nil
}

func (m *MysqlDB) Get(ctx context.Context, table string, id string, options ...*GetOptions) (*DocumentRef, error) {
	var row map[string]interface{}
	q := m.db.WithContext(ctx).Table(table).Where("id = ?", id)
	if len(options) > 0 && options[0] != nil && len(options[0].Fields) > 0 {
		q = q.Select(options[0].Fields)
	}
	if err := q.Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDocNotFound
		}
		su_logger.Error(ctx, err, "mysql get failed", su_logger.E().String("table", table).String("id", id))
		return nil, err
	}
	return &DocumentRef{ID: id, Data: row}, nil
}

func buildSQLConds(cond Conds) (string, []interface{}) {
	if len(cond) == 0 {
		return "", nil
	}
	var sb strings.Builder
	args := make([]interface{}, 0, len(cond))
	for i, c := range cond {
		if i > 0 {
			sb.WriteString(" AND ")
		}
		op := strings.ToLower(c.Cond)
		switch op {
		case ">", ">=", "<", "<=", "=", "==", "!=":
			if op == "==" {
				op = "="
			}
			sb.WriteString(c.Field + " " + op + " ?")
			args = append(args, c.Value)
		case "in", "array-contains":
			// treat both as IN
			sb.WriteString(c.Field + " IN (?)")
			args = append(args, c.Value)
		case "not-in":
			sb.WriteString(c.Field + " NOT IN (?)")
			args = append(args, c.Value)
		default:
			sb.WriteString(c.Field + " = ?")
			args = append(args, c.Value)
		}
	}
	return sb.String(), args
}

func (m *MysqlDB) Find(ctx context.Context, table string, conds Conds, options ...*FindOptions) (*Iterator, error) {
	q := m.db.WithContext(ctx).Table(table)
	if where, args := buildSQLConds(conds); where != "" {
		q = q.Where(where, args...)
	}
	if len(options) > 0 && options[0] != nil {
		opt := options[0]
		if len(opt.Fields) > 0 {
			q = q.Select(opt.Fields)
		}
		if opt.Offset > 0 {
			q = q.Offset(int(opt.Offset))
		}
		if opt.Limit > 0 {
			q = q.Limit(int(opt.Limit))
		}
		if len(opt.Sorts) > 0 {
			for _, s := range opt.Sorts {
				dir := "ASC"
				if s.Order < 0 {
					dir = "DESC"
				}
				q = q.Order(s.Field + " " + dir)
			}
		}
	}
	var rows []map[string]interface{}
	if err := q.Find(&rows).Error; err != nil {
		su_logger.Error(ctx, err, "mysql find failed", su_logger.E().String("table", table))
		return nil, err
	}
	it := &Iterator{items: make([]*DocumentRef, 0, len(rows))}
	for _, r := range rows {
		id := cast.ToString(r["id"])
		it.items = append(it.items, &DocumentRef{ID: id, Data: r})
	}
	return it, nil
}

func (m *MysqlDB) BatchCreate(ctx context.Context, table string, rows []Row, options ...*BatchWriteOptions) *BatchWriteResult {
	opt := dftBatchWriteOptions()
	opt.Apply(options...)
	result := &BatchWriteResult{Ids: make([]string, 0, len(rows)), Errors: make([]error, 0)}
	if len(rows) == 0 {
		return result
	}
	// ordered bulk create
	for _, row := range rows {
		_, err := m.Create(ctx, table, row, &CreateOptions{ChangeHook: opt.Hook()})
		if err != nil {
			result.Errors = append(result.Errors, err)
			if opt.ContinueOnError != 1 {
				return result
			}
			continue
		}
		result.Ids = append(result.Ids, row.ID())
		result.Affected++
	}
	return result
}

func (m *MysqlDB) BatchInsert(ctx context.Context, table string, rows []Row, options ...*BatchWriteOptions) *BatchWriteResult {
	opt := dftBatchWriteOptions()
	opt.Apply(options...)
	result := &BatchWriteResult{Ids: make([]string, 0, len(rows)), Errors: make([]error, 0)}
	if len(rows) == 0 {
		return result
	}
	for _, row := range rows {
		id, err := m.Insert(ctx, table, row, &InsertOptions{ChangeHook: opt.Hook(), MergeAll: true})
		if err != nil {
			result.Errors = append(result.Errors, err)
			if opt.ContinueOnError != 1 {
				return result
			}
			continue
		}
		result.Ids = append(result.Ids, id)
		result.Affected++
	}
	return result
}

func (m *MysqlDB) BatchDelete(ctx context.Context, table string, ids []string, options ...*BatchWriteOptions) *BatchWriteResult {
	opt := dftBatchWriteOptions()
	opt.Apply(options...)
	result := &BatchWriteResult{Errors: make([]error, 0)}
	if len(ids) == 0 {
		return result
	}
	tx := m.db.WithContext(ctx).Table(table).Where("id IN (?)", ids).Delete(nil)
	if tx.Error != nil {
		result.Errors = append(result.Errors, tx.Error)
		return result
	}
	result.Affected = tx.RowsAffected
	if opt.Hook() != nil {
		for _, id := range ids {
			opt.Hook()(ctx, ChangeRow{Table: table, Id: id, Action: ActionDelete})
		}
	}
	return result
}

func (m *MysqlDB) BatchUpdate(ctx context.Context, table string, updates []UpdateOne, options ...*BatchWriteOptions) *BatchWriteResult {
	opt := dftBatchWriteOptions()
	opt.Apply(options...)
	result := &BatchWriteResult{Errors: make([]error, 0)}
	for _, up := range updates {
		if err := m.Update(ctx, table, up, &UpdateOptions{ChangeHook: opt.Hook()}); err != nil {
			result.Errors = append(result.Errors, err)
			if opt.ContinueOnError != 1 {
				return result
			}
		} else {
			result.Affected++
		}
	}
	return result
}

func (m *MysqlDB) BatchGet(ctx context.Context, table string, ids []string, options ...*GetOptions) (*Iterator, error) {
	q := m.db.WithContext(ctx).Table(table).Where("id IN (?)", ids)
	if len(options) > 0 && options[0] != nil && len(options[0].Fields) > 0 {
		q = q.Select(options[0].Fields)
	}
	var rows []map[string]interface{}
	if err := q.Find(&rows).Error; err != nil {
		su_logger.Error(ctx, err, "mysql batch get failed", su_logger.E().String("table", table))
		return nil, err
	}
	it := &Iterator{items: make([]*DocumentRef, 0, len(rows))}
	for _, r := range rows {
		it.items = append(it.items, &DocumentRef{ID: cast.ToString(r["id"]), Data: r})
	}
	return it, nil
}

func (m *MysqlDB) Count(ctx context.Context, table string, conds Conds) (int64, error) {
	q := m.db.WithContext(ctx).Table(table)
	if where, args := buildSQLConds(conds); where != "" {
		q = q.Where(where, args...)
	}
	var cnt int64
	if err := q.Count(&cnt).Error; err != nil {
		su_logger.Error(ctx, err, "mysql count failed", su_logger.E().String("table", table))
		return 0, err
	}
	return cnt, nil
}

func (m *MysqlDB) Upsert(ctx context.Context, table string, row UpsertRow, options ...*UpsertOptions) (*UpsertRs, error) {
	opt := dftUpsertOptions()
	opt.Apply(options...)
	if row.Id == "" {
		return nil, errors.New("empty id in Upsert")
	}
	// Build a map for insert values from Inserts+Updates
	values := map[string]interface{}{"id": row.Id}
	for _, u := range row.Inserts {
		values[u.Field] = u.Value
	}
	for _, u := range row.Updates {
		if strings.ToLower(u.Op) != "incr" {
			values[u.Field] = u.Value
		}
	}

	// Build updates for conflict
	setMap := map[string]interface{}{}
	for _, u := range row.Updates {
		if strings.ToLower(u.Op) != "incr" {
			setMap[u.Field] = u.Value
		}
	}

	onConflict := clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.Assignments(setMap)}
	if err := m.db.WithContext(ctx).Table(table).Clauses(onConflict).Create(values).Error; err != nil {
		su_logger.Error(ctx, err, "mysql upsert failed", su_logger.E().String("table", table).String("id", row.Id))
		return nil, err
	}
	if opt.ChangeHook != nil {
		opt.ChangeHook(ctx, ChangeRow{Table: table, Id: row.Id, Action: ActionUpdate, Data: updatesToMapStringInterface(row.Updates)})
	}
	return &UpsertRs{Id: row.Id, MatchCount: 1}, nil
}

func (m *MysqlDB) UpsertSingleField(ctx context.Context, table string, row UpsertSingleFields, options ...*UpsertSingleFieldRowOptions) error {
	var mergeAll = true
	if len(options) > 0 && options[0] != nil {
		mergeAll = !options[0].DisableMergeAll
	}
	if row.Id == "" {
		return errors.New("empty id")
	}
	insertVals := map[string]interface{}{"id": row.Id}
	for k, v := range row.Fields {
		insertVals[k] = v
	}
	var onConflict clause.OnConflict
	if mergeAll {
		onConflict = clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, UpdateAll: true}
	} else {
		onConflict = clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}
	}
	return m.db.WithContext(ctx).Table(table).Clauses(onConflict).Create(insertVals).Error
}

func (m *MysqlDB) BatchUpsert(ctx context.Context, table string, rows []UpsertRow, options ...*BatchWriteOptions) (*BatchUpsertRs, error) {
	opt := dftBatchWriteOptions()
	opt.Apply(options...)
	rs := &BatchUpsertRs{}
	for _, r := range rows {
		_, err := m.Upsert(ctx, table, r)
		if err != nil {
			if opt.ContinueOnError != 1 {
				return nil, err
			}
			continue
		}
		// 无法准确区分 insert/update，这里不统计明细，只累计总数
	}
	return rs, nil
}

// ToUpdateOne 提取结构体为 UpdateOne（复用与 mongo 相同的语义）
func (m *MysqlDB) ToUpdateOne(data any, options *UpdateOptions) (upData UpdateOne) {
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
	processStructMysql(v, t, opt, &upData, &containUpdateField)
	if len(upData.Updates) > 0 {
		if !containUpdateField && !opt.IgnoreUpdateTimeField {
			upData.Updates = append(upData.Updates, Update{Field: UpdateField, Value: su_time.CurrentTimestampMilli()})
		}
	}
	return upData
}

// processStruct 与 mongo 的实现一致，用于扫描结构体生成 UpdateOne
func processStructMysql(v reflect.Value, t reflect.Type, opt *UpdateOptions, upData *UpdateOne, containUpdateField *bool) {
	nF := v.NumField()
	for i := 0; i < nF; i++ {
		field := t.Field(i)
		value := v.Field(i)
		key := field.Tag.Get(opt.Tag)
		if key == "" {
			continue
		}
		if pos := strings.Index(key, ","); pos != -1 {
			key = key[0:pos]
		}
		key = strings.TrimSpace(key)
		if key == "" || key == "-" {
			continue
		}
		if key == "_id" || key == "id" { // id 字段
			upData.ID = cast.ToString(value.Interface())
			continue
		}
		vi := value.Interface()
		isZero := reflect.DeepEqual(vi, reflect.Zero(value.Type()).Interface())
		if (isZero && opt.EmptyIgnore != nil && !opt.EmptyIgnore(key, vi)) || !isZero {
			upData.Updates = append(upData.Updates, Update{Field: key, Value: vi})
			if key == UpdateField {
				*containUpdateField = true
			}
		}
	}
}
