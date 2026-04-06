package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/spf13/cast"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/su_time"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
)

// PgsqlConfig PostgreSQL 连接配置
type PgsqlConfig struct {
	MaxOpenConn int
	MaxIdleConn int
	// 最大空闲时间, 单位秒
	MaxIdleTime time.Duration
	// 最大生命周期, 单位秒
	MaxLifeTime time.Duration
	Master      PgsqlSingleConfig
}

type PgsqlSingleConfig struct {
	// 可选，如果提供则直接使用
	Dsn      string
	Host     string
	Port     int
	User     string
	Password string
	Db       string
	// SSL模式: disable, require, verify-ca, verify-full，默认 disable
	SSLMode string
	// 默认30s
	Timeout time.Duration
	// 时区，默认 UTC
	TimeZone string
}

// parsePgsqlDsn 解析 PostgreSQL DSN
func parsePgsqlDsn(conf *PgsqlSingleConfig) string {
	if conf.Dsn != "" {
		return conf.Dsn
	}

	port := conf.Port
	if port == 0 {
		port = 5432
	}

	sslMode := conf.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}

	timeZone := conf.TimeZone
	if timeZone == "" {
		timeZone = "UTC"
	}

	timeout := 30
	if conf.Timeout > 0 {
		timeout = cast.ToInt(conf.Timeout.Seconds())
	}

	// PostgreSQL DSN 格式
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s timezone=%s connect_timeout=%d",
		conf.Host,
		port,
		conf.User,
		url.QueryEscape(conf.Password),
		conf.Db,
		sslMode,
		timeZone,
		timeout)

	return dsn
}

// PgsqlLite 不依赖 gorm，基于原生 SQL 实现
type PgsqlLite struct {
	db *sql.DB
}

// NewPgsqlLite 创建 PgsqlLite 实例
func NewPgsqlLite(config *PgsqlConfig) (*PgsqlLite, error) {
	dsn := parsePgsqlDsn(&config.Master)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open pgsql connection: %w", err)
	}

	// 设置连接池参数
	idleConn := compareIntWithDefault(config.MaxIdleConn, 5, 100, 5)
	openConn := compareIntWithDefault(config.MaxOpenConn, 1, 200, 10)
	idleTime := compareDurationWithDefault(config.MaxIdleTime, 10, 1800, 60)
	lifeTime := compareDurationWithDefault(config.MaxLifeTime, 30, 3600, 1800)

	db.SetMaxOpenConns(openConn)
	db.SetMaxIdleConns(idleConn)
	db.SetConnMaxIdleTime(idleTime)
	db.SetConnMaxLifetime(lifeTime)

	// 测试连接
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping pgsql: %w", err)
	}

	return &PgsqlLite{db: db}, nil
}

// NewPgsqlLiteFromDB 从已有的 sql.DB 创建 PgsqlLite
func NewPgsqlLiteFromDB(db *sql.DB) *PgsqlLite {
	return &PgsqlLite{db: db}
}

// DB 返回底层的 sql.DB
func (p *PgsqlLite) DB() *sql.DB {
	return p.db
}

// Close 关闭数据库连接
func (p *PgsqlLite) Close() error {
	if p.db != nil {
		return p.db.Close()
	}
	return nil
}

// ==================== 核心 CRUD 方法 ====================

// Insert 创建一条新记录, 记录存在则进行覆盖
func (p *PgsqlLite) Insert(ctx context.Context, table string, row Row, options ...*InsertOptions) (string, error) {
	opt := dftInsertOptions()
	opt.Apply(options...)

	data, err := rowToMap(row)
	if err != nil {
		su_logger.Error(ctx, err, "pgsql lite insert marshal failed")
		return "", err
	}

	// 构建 INSERT ... ON CONFLICT DO UPDATE 语句
	columns, placeholders, values := buildPgsqlInsertParts(data)
	updateParts := buildPgsqlUpdateOnConflict(data)

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (\"id\") DO UPDATE SET %s",
		quotePgsqlTable(table), columns, placeholders, updateParts)

	_, err = p.db.ExecContext(ctx, query, values...)
	if err != nil {
		su_logger.Error(ctx, err, "pgsql lite insert failed", su_logger.E().String("table", table))
		return "", err
	}

	id := cast.ToString(data["id"])
	if opt.Hook() != nil {
		opt.Hook()(ctx, ChangeRow{Table: table, Id: id, Action: ActionAdd})
	}
	return id, nil
}

// Delete 删除一条记录
func (p *PgsqlLite) Delete(ctx context.Context, table string, id string, options ...*DeleteOptions) (int, error) {
	opt := dftDeleteOptions()
	opt.Apply(options...)

	query := fmt.Sprintf("DELETE FROM %s WHERE \"id\" = $1", quotePgsqlTable(table))
	result, err := p.db.ExecContext(ctx, query, id)
	if err != nil {
		su_logger.Error(ctx, err, "pgsql lite delete failed", su_logger.E().String("table", table).String("id", id))
		return 0, err
	}

	affected, _ := result.RowsAffected()
	if opt.Hook() != nil {
		opt.Hook()(ctx, ChangeRow{Table: table, Id: id, Action: ActionDelete})
	}
	return int(affected), nil
}

// Create 创建一条新记录, 如果记录存在返回Error
func (p *PgsqlLite) Create(ctx context.Context, table string, row Row, options ...*CreateOptions) (*CreateResult, error) {
	opt := dftCreateOptions()
	opt.Apply(options...)

	data, err := rowToMap(row)
	if err != nil {
		su_logger.Error(ctx, err, "pgsql lite create marshal failed")
		return nil, err
	}

	columns, placeholders, values := buildPgsqlInsertParts(data)
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", quotePgsqlTable(table), columns, placeholders)

	_, err = p.db.ExecContext(ctx, query, values...)
	if err != nil {
		if isPgsqlDuplicateError(err) {
			return nil, ErrDocAlreadyExists
		}
		su_logger.Error(ctx, err, "pgsql lite create failed", su_logger.E().String("table", table))
		return nil, err
	}

	id := cast.ToString(data["id"])
	if opt.Hook() != nil {
		opt.Hook()(ctx, ChangeRow{Table: table, Id: id, Action: ActionAdd})
	}
	return &CreateResult{ID: id}, nil
}

// Update 更新一条记录
func (p *PgsqlLite) Update(ctx context.Context, table string, update UpdateOne, options ...*UpdateOptions) error {
	opt := dftUpdateOptions()
	opt.Apply(options...)

	if update.ID == "" {
		return errors.New("empty id in UpdateOne")
	}

	if len(update.Updates) == 0 {
		return nil
	}

	// 分离普通更新和增量更新
	var setParts []string
	var values []interface{}
	paramIdx := 1

	for _, u := range update.Updates {
		qf := quotePgsqlField(u.Field)
		switch strings.ToLower(u.Op) {
		case "incr":
			setParts = append(setParts, fmt.Sprintf("%s = %s + $%d", qf, qf, paramIdx))
			values = append(values, u.Value)
		default:
			setParts = append(setParts, fmt.Sprintf("%s = $%d", qf, paramIdx))
			values = append(values, u.Value)
		}
		paramIdx++
	}

	values = append(values, update.ID)
	query := fmt.Sprintf("UPDATE %s SET %s WHERE \"id\" = $%d", quotePgsqlTable(table), strings.Join(setParts, ", "), paramIdx)

	_, err := p.db.ExecContext(ctx, query, values...)
	if err != nil {
		su_logger.Error(ctx, err, "pgsql lite update failed", su_logger.E().String("table", table).String("id", update.ID))
		return err
	}

	if opt.Hook() != nil {
		opt.Hook()(ctx, ChangeRow{Table: table, Id: update.ID, Action: ActionUpdate, Data: updatesToMapStringInterface(update.Updates)})
	}
	return nil
}

// Get 读取一条记录
func (p *PgsqlLite) Get(ctx context.Context, table string, id string, options ...*GetOptions) (*DocumentRef, error) {
	var fields string = "*"
	if len(options) > 0 && options[0] != nil && len(options[0].Fields) > 0 {
		fields = strings.Join(options[0].Fields, ", ")
	}

	query := fmt.Sprintf("SELECT %s FROM %s WHERE \"id\" = $1 LIMIT 1", fields, quotePgsqlTable(table))
	rows, err := p.db.QueryContext(ctx, query, id)
	if err != nil {
		su_logger.Error(ctx, err, "pgsql lite get failed", su_logger.E().String("table", table).String("id", id))
		return nil, err
	}
	defer rows.Close()

	items, err := scanRowsToMaps(rows)
	if err != nil {
		su_logger.Error(ctx, err, "pgsql lite get scan failed", su_logger.E().String("table", table).String("id", id))
		return nil, err
	}

	if len(items) == 0 {
		return nil, ErrDocNotFound
	}

	return &DocumentRef{ID: id, Data: items[0]}, nil
}

// ==================== 查询方法 ====================

// Find 查询多条记录
func (p *PgsqlLite) Find(ctx context.Context, table string, conds Conds, options ...*FindOptions) (*Iterator, error) {
	var fields string = "*"
	var opt *FindOptions
	if len(options) > 0 && options[0] != nil {
		opt = options[0]
		if len(opt.Fields) > 0 {
			fields = strings.Join(opt.Fields, ", ")
		}
	}

	query := fmt.Sprintf("SELECT %s FROM %s", fields, quotePgsqlTable(table))
	var args []interface{}

	// 构建 WHERE 条件
	if where, whereArgs := buildPgsqlConds(conds); where != "" {
		query += " WHERE " + where
		args = append(args, whereArgs...)
	}

	// 构建 ORDER BY
	if opt != nil && len(opt.Sorts) > 0 {
		var orderParts []string
		for _, s := range opt.Sorts {
			dir := "ASC"
			if s.Order < 0 {
				dir = "DESC"
			}
			orderParts = append(orderParts, fmt.Sprintf("%s %s", quotePgsqlField(s.Field), dir))
		}
		query += " ORDER BY " + strings.Join(orderParts, ", ")
	}

	// 构建 LIMIT OFFSET
	if opt != nil {
		if opt.Limit > 0 {
			query += fmt.Sprintf(" LIMIT %d", opt.Limit)
		}
		if opt.Offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", opt.Offset)
		}
	}

	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		su_logger.Error(ctx, err, "pgsql lite find failed", su_logger.E().String("table", table))
		return nil, err
	}
	defer rows.Close()

	items, err := scanRowsToMaps(rows)
	if err != nil {
		su_logger.Error(ctx, err, "pgsql lite find scan failed", su_logger.E().String("table", table))
		return nil, err
	}

	it := &Iterator{items: make([]*DocumentRef, 0, len(items))}
	for _, item := range items {
		id := cast.ToString(item["id"])
		it.items = append(it.items, &DocumentRef{ID: id, Data: item})
	}
	return it, nil
}

// Count 获取记录总数
func (p *PgsqlLite) Count(ctx context.Context, table string, conds Conds) (int64, error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", quotePgsqlTable(table))
	var args []interface{}

	if where, whereArgs := buildPgsqlConds(conds); where != "" {
		query += " WHERE " + where
		args = append(args, whereArgs...)
	}

	var count int64
	err := p.db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		su_logger.Error(ctx, err, "pgsql lite count failed", su_logger.E().String("table", table))
		return 0, err
	}
	return count, nil
}

// ==================== 批量操作方法 ====================

// BatchCreate 批量创建记录, 如果记录存在则返回错误
func (p *PgsqlLite) BatchCreate(ctx context.Context, table string, rows []Row, options ...*BatchWriteOptions) *BatchWriteResult {
	opt := dftBatchWriteOptions()
	opt.Apply(options...)
	result := &BatchWriteResult{Ids: make([]string, 0, len(rows)), Errors: make([]error, 0)}

	if len(rows) == 0 {
		return result
	}

	for _, row := range rows {
		_, err := p.Create(ctx, table, row, &CreateOptions{ChangeHook: opt.Hook()})
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

// BatchInsert 批量插入记录, 如果记录存在则进行覆盖
func (p *PgsqlLite) BatchInsert(ctx context.Context, table string, rows []Row, options ...*BatchWriteOptions) *BatchWriteResult {
	opt := dftBatchWriteOptions()
	opt.Apply(options...)
	result := &BatchWriteResult{Ids: make([]string, 0, len(rows)), Errors: make([]error, 0)}

	if len(rows) == 0 {
		return result
	}

	for _, row := range rows {
		id, err := p.Insert(ctx, table, row, &InsertOptions{ChangeHook: opt.Hook(), MergeAll: true})
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

// BatchDelete 批量删除记录
func (p *PgsqlLite) BatchDelete(ctx context.Context, table string, ids []string, options ...*BatchWriteOptions) *BatchWriteResult {
	opt := dftBatchWriteOptions()
	opt.Apply(options...)
	result := &BatchWriteResult{Errors: make([]error, 0)}

	if len(ids) == 0 {
		return result
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	query := fmt.Sprintf("DELETE FROM %s WHERE \"id\" IN (%s)", quotePgsqlTable(table), strings.Join(placeholders, ", "))
	res, err := p.db.ExecContext(ctx, query, args...)
	if err != nil {
		su_logger.Error(ctx, err, "pgsql lite batch delete failed", su_logger.E().String("table", table))
		result.Errors = append(result.Errors, err)
		return result
	}

	result.Affected, _ = res.RowsAffected()
	if opt.Hook() != nil {
		for _, id := range ids {
			opt.Hook()(ctx, ChangeRow{Table: table, Id: id, Action: ActionDelete})
		}
	}
	return result
}

// BatchUpdate 批量更新记录，使用 CASE WHEN 优化性能
func (p *PgsqlLite) BatchUpdate(ctx context.Context, table string, updates []UpdateOne, options ...*BatchWriteOptions) *BatchWriteResult {
	opt := dftBatchWriteOptions()
	opt.Apply(options...)
	result := &BatchWriteResult{Errors: make([]error, 0)}

	if len(updates) == 0 {
		return result
	}

	// 按批次处理
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

		affected, err := p.batchUpdateWithCaseWhen(ctx, table, batch, opt)
		if err != nil {
			result.Errors = append(result.Errors, err)
			if opt.ContinueOnError != 1 {
				return result
			}
		} else {
			result.Affected += affected
		}
	}

	return result
}

// batchUpdateWithCaseWhen 使用 CASE WHEN 实现批量更新
func (p *PgsqlLite) batchUpdateWithCaseWhen(ctx context.Context, table string, updates []UpdateOne, opt *BatchWriteOptions) (int64, error) {
	if len(updates) == 0 {
		return 0, nil
	}

	// 1. 收集所有字段的更新信息
	updateFieldMap := make(map[string][]struct {
		id    string
		value interface{}
		op    string
	})

	var ids []interface{}
	for _, up := range updates {
		if up.ID == "" {
			continue
		}
		ids = append(ids, up.ID)
		for _, u := range up.Updates {
			updateFieldMap[u.Field] = append(updateFieldMap[u.Field], struct {
				id    string
				value interface{}
				op    string
			}{id: up.ID, value: u.Value, op: u.Op})
		}
	}

	if len(ids) == 0 || len(updateFieldMap) == 0 {
		return 0, nil
	}

	// 2. 构建 SET 部分（使用 CASE WHEN）
	var setParts []string
	var args []interface{}
	paramIdx := 1

	for field, fieldUpdates := range updateFieldMap {
		qf := quotePgsqlField(field)
		var caseWhen strings.Builder
		caseWhen.Grow(len(fieldUpdates) * 48)

		// 检查是否有 incr 操作
		hasIncr := false
		for _, u := range fieldUpdates {
			if strings.ToLower(u.op) == "incr" {
				hasIncr = true
				break
			}
		}

		if hasIncr {
			// 对于 incr 操作，使用增量更新
			caseWhen.WriteString(qf + " = " + qf + " + CASE \"id\"")
			for _, u := range fieldUpdates {
				caseWhen.WriteString(fmt.Sprintf(" WHEN $%d THEN $%d", paramIdx, paramIdx+1))
				args = append(args, u.id, u.value)
				paramIdx += 2
			}
			caseWhen.WriteString(" ELSE 0 END")
		} else {
			// 普通更新使用 CASE WHEN
			caseWhen.WriteString(qf + " = CASE \"id\"")
			for _, u := range fieldUpdates {
				caseWhen.WriteString(fmt.Sprintf(" WHEN $%d THEN $%d", paramIdx, paramIdx+1))
				args = append(args, u.id, u.value)
				paramIdx += 2
			}
			caseWhen.WriteString(" ELSE " + qf + " END")
		}

		setParts = append(setParts, caseWhen.String())
	}

	// 3. 构建 WHERE IN 部分
	placeholders := make([]string, len(ids))
	for i := range ids {
		placeholders[i] = fmt.Sprintf("$%d", paramIdx)
		paramIdx++
	}
	args = append(args, ids...)

	// 4. 构建完整 SQL
	query := fmt.Sprintf("UPDATE %s SET %s WHERE \"id\" IN (%s)",
		quotePgsqlTable(table),
		strings.Join(setParts, ", "),
		strings.Join(placeholders, ", "))

	res, err := p.db.ExecContext(ctx, query, args...)
	if err != nil {
		su_logger.Error(ctx, err, "pgsql lite batch update failed", su_logger.E().String("table", table))
		return 0, err
	}

	affected, _ := res.RowsAffected()

	// 触发 hook
	if opt.Hook() != nil {
		for _, up := range updates {
			opt.Hook()(ctx, ChangeRow{Table: table, Id: up.ID, Action: ActionUpdate, Data: updatesToMapStringInterface(up.Updates)})
		}
	}

	return affected, nil
}

// BatchGet 批量获取记录
func (p *PgsqlLite) BatchGet(ctx context.Context, table string, ids []string, options ...*GetOptions) (*Iterator, error) {
	if len(ids) == 0 {
		return &Iterator{items: []*DocumentRef{}}, nil
	}

	var fields string = "*"
	if len(options) > 0 && options[0] != nil && len(options[0].Fields) > 0 {
		fields = strings.Join(options[0].Fields, ", ")
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	query := fmt.Sprintf("SELECT %s FROM %s WHERE \"id\" IN (%s)", fields, quotePgsqlTable(table), strings.Join(placeholders, ", "))
	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		su_logger.Error(ctx, err, "pgsql lite batch get failed", su_logger.E().String("table", table))
		return nil, err
	}
	defer rows.Close()

	items, err := scanRowsToMaps(rows)
	if err != nil {
		su_logger.Error(ctx, err, "pgsql lite batch get scan failed", su_logger.E().String("table", table))
		return nil, err
	}

	it := &Iterator{items: make([]*DocumentRef, 0, len(items))}
	for _, item := range items {
		it.items = append(it.items, &DocumentRef{ID: cast.ToString(item["id"]), Data: item})
	}
	return it, nil
}

// ==================== Upsert 相关方法 ====================

// Upsert 更新或插入一条记录
func (p *PgsqlLite) Upsert(ctx context.Context, table string, row UpsertRow, options ...*UpsertOptions) (*UpsertRs, error) {
	opt := dftUpsertOptions()
	opt.Apply(options...)

	if row.Id == "" {
		return nil, errors.New("empty id in Upsert")
	}

	// 构建插入数据
	values := map[string]interface{}{"id": row.Id}
	for _, u := range row.Inserts {
		values[u.Field] = u.Value
	}
	for _, u := range row.Updates {
		if strings.ToLower(u.Op) != "incr" {
			values[u.Field] = u.Value
		}
	}

	// 构建更新部分
	var updateParts []string
	var updateArgs []interface{}
	paramIdx := len(values) + 1 // 从插入参数之后开始

	for _, u := range row.Updates {
		qf := quotePgsqlField(u.Field)
		if strings.ToLower(u.Op) == "incr" {
			updateParts = append(updateParts, fmt.Sprintf("%s = %s.%s + $%d", qf, quotePgsqlTable(table), qf, paramIdx))
		} else {
			updateParts = append(updateParts, fmt.Sprintf("%s = $%d", qf, paramIdx))
		}
		updateArgs = append(updateArgs, u.Value)
		paramIdx++
	}

	columns, placeholders, insertArgs := buildPgsqlInsertParts(values)
	allArgs := append(insertArgs, updateArgs...)

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", quotePgsqlTable(table), columns, placeholders)

	if len(updateParts) > 0 {
		query += " ON CONFLICT (\"id\") DO UPDATE SET " + strings.Join(updateParts, ", ")
	} else {
		query += " ON CONFLICT (\"id\") DO NOTHING"
	}

	_, err := p.db.ExecContext(ctx, query, allArgs...)
	if err != nil {
		su_logger.Error(ctx, err, "pgsql lite upsert failed", su_logger.E().String("table", table).String("id", row.Id))
		return nil, err
	}

	if opt.ChangeHook != nil {
		opt.ChangeHook(ctx, ChangeRow{Table: table, Id: row.Id, Action: ActionUpdate, Data: updatesToMapStringInterface(row.Updates)})
	}
	return &UpsertRs{Id: row.Id, MatchCount: 1}, nil
}

// UpsertSingleField 对单个字段进行更新或插入
func (p *PgsqlLite) UpsertSingleField(ctx context.Context, table string, row UpsertSingleFields, options ...*UpsertSingleFieldRowOptions) error {
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

	columns, placeholders, values := buildPgsqlInsertParts(insertVals)
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", quotePgsqlTable(table), columns, placeholders)

	if mergeAll {
		updateParts := buildPgsqlUpdateOnConflict(insertVals)
		query += " ON CONFLICT (\"id\") DO UPDATE SET " + updateParts
	} else {
		query += " ON CONFLICT (\"id\") DO NOTHING"
	}

	_, err := p.db.ExecContext(ctx, query, values...)
	if err != nil {
		su_logger.Error(ctx, err, "pgsql lite upsert single field failed", su_logger.E().String("table", table).String("id", row.Id))
		return err
	}
	return nil
}

// BatchUpsert 批量更新或插入记录
func (p *PgsqlLite) BatchUpsert(ctx context.Context, table string, rows []UpsertRow, options ...*BatchWriteOptions) (*BatchUpsertRs, error) {
	opt := dftBatchWriteOptions()
	opt.Apply(options...)
	rs := &BatchUpsertRs{}

	if len(rows) == 0 {
		return rs, nil
	}

	// 按批次处理
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

		if err := p.batchUpsertInternal(ctx, table, batch, opt); err != nil {
			if opt.ContinueOnError != 1 {
				return nil, err
			}
		}
	}

	return rs, nil
}

// batchUpsertInternal 批量 upsert 内部实现
func (p *PgsqlLite) batchUpsertInternal(ctx context.Context, table string, rows []UpsertRow, opt *BatchWriteOptions) error {
	if len(rows) == 0 {
		return nil
	}

	// 1. 收集所有字段和构建插入数据
	allFields := make(map[string]struct{})
	allFields["id"] = struct{}{}

	for _, row := range rows {
		for _, u := range row.Inserts {
			allFields[u.Field] = struct{}{}
		}
		for _, u := range row.Updates {
			allFields[u.Field] = struct{}{}
		}
	}

	// 2. 构建列名列表
	var columns []string
	for field := range allFields {
		columns = append(columns, field)
	}

	// 3. 构建 VALUES 部分
	var valueRows []string
	var args []interface{}
	paramIdx := 1

	for _, row := range rows {
		// 合并 Inserts 和 Updates 到 map
		rowData := make(map[string]interface{})
		rowData["id"] = row.Id
		for _, u := range row.Inserts {
			rowData[u.Field] = u.Value
		}
		for _, u := range row.Updates {
			if strings.ToLower(u.Op) != "incr" {
				rowData[u.Field] = u.Value
			}
		}

		var placeholders []string
		for _, col := range columns {
			if val, ok := rowData[col]; ok {
				placeholders = append(placeholders, fmt.Sprintf("$%d", paramIdx))
				args = append(args, val)
				paramIdx++
			} else {
				placeholders = append(placeholders, "DEFAULT")
			}
		}
		valueRows = append(valueRows, "("+strings.Join(placeholders, ", ")+")")
	}

	// 4. 构建 ON CONFLICT DO UPDATE 部分
	var updateParts []string
	for _, col := range columns {
		if col == "id" {
			continue
		}
		qf := quotePgsqlField(col)
		updateParts = append(updateParts, fmt.Sprintf("%s = EXCLUDED.%s", qf, qf))
	}

	// 5. 构建完整 SQL
	var quotedColumns []string
	for _, col := range columns {
		quotedColumns = append(quotedColumns, quotePgsqlField(col))
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s",
		quotePgsqlTable(table),
		strings.Join(quotedColumns, ", "),
		strings.Join(valueRows, ", "))

	if len(updateParts) > 0 {
		query += " ON CONFLICT (\"id\") DO UPDATE SET " + strings.Join(updateParts, ", ")
	}

	_, err := p.db.ExecContext(ctx, query, args...)
	if err != nil {
		su_logger.Error(ctx, err, "pgsql lite batch upsert failed", su_logger.E().String("table", table))
		return err
	}

	// 触发 hook
	if opt.Hook() != nil {
		for _, row := range rows {
			opt.Hook()(ctx, ChangeRow{Table: table, Id: row.Id, Action: ActionUpdate, Data: updatesToMapStringInterface(row.Updates)})
		}
	}

	return nil
}

// ==================== ToUpdateOne 辅助方法 ====================

// ToUpdateOne 提取结构体为 UpdateOne
func (p *PgsqlLite) ToUpdateOne(data any, options *UpdateOptions) (upData UpdateOne) {
	opt := dftPgsqlLiteToUpdateOptions()
	if options != nil {
		opt.Apply(options)
	}

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
	processStructPgsqlLite(v, t, opt, &upData, &containUpdateField)
	if len(upData.Updates) > 0 {
		if !containUpdateField && !opt.IgnoreUpdateTimeField {
			upData.Updates = append(upData.Updates, Update{Field: UpdateField, Value: su_time.CurrentTimestampMilli()})
		}
	}
	return upData
}

func dftPgsqlLiteToUpdateOptions() *UpdateOptions {
	return &UpdateOptions{
		Tag: "json",
	}
}

func processStructPgsqlLite(v reflect.Value, t reflect.Type, opt *UpdateOptions, upData *UpdateOne, containUpdateField *bool) {
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
		if key == "_id" || key == "id" {
			upData.ID = cast.ToString(value.Interface())
			continue
		}
		vi := value.Interface()
		isZero := reflect.DeepEqual(vi, reflect.Zero(value.Type()).Interface())
		if opt.Filter != nil && opt.Filter(key, vi) {
			continue
		}
		if (isZero && opt.EmptyIgnore != nil && !opt.EmptyIgnore(key, vi)) || !isZero {
			upData.Updates = append(upData.Updates, Update{Field: key, Value: vi})
			if key == UpdateField {
				*containUpdateField = true
			}
		}
	}
}

// ==================== 辅助函数 ====================

// quotePgsqlTable 对表名添加双引号，防止 SQL 注入
// users => "users"
// schema.users => "schema"."users"
func quotePgsqlTable(table string) string {
	// 如果已经包含双引号，不重复处理
	if strings.Contains(table, "\"") {
		return table
	}
	if idx := strings.Index(table, "."); idx != -1 {
		// 带 schema 的情况: schema.users => "schema"."users"
		schemaName := table[:idx]
		tableName := table[idx+1:]
		return "\"" + schemaName + "\".\"" + tableName + "\""
	}
	return "\"" + table + "\""
}

// quotePgsqlField 对字段名添加双引号，防止 SQL 注入
// name => "name"
// t.name => t."name"
func quotePgsqlField(field string) string {
	// 如果已经包含双引号，不重复处理
	if strings.Contains(field, "\"") {
		return field
	}
	if idx := strings.LastIndex(field, "."); idx != -1 {
		// 带表别名的情况: t.name => t."name"
		prefix := field[:idx+1]
		name := field[idx+1:]
		return prefix + "\"" + name + "\""
	}
	return "\"" + field + "\""
}

// buildPgsqlInsertParts 构建 INSERT 语句的列、占位符和值（使用 $1, $2 等）
func buildPgsqlInsertParts(data map[string]interface{}) (columns string, placeholders string, values []interface{}) {
	var cols []string
	var phs []string
	idx := 1
	for k, v := range data {
		cols = append(cols, quotePgsqlField(k))
		phs = append(phs, fmt.Sprintf("$%d", idx))
		values = append(values, v)
		idx++
	}
	return strings.Join(cols, ", "), strings.Join(phs, ", "), values
}

// buildPgsqlUpdateOnConflict 构建 ON CONFLICT DO UPDATE SET 部分
func buildPgsqlUpdateOnConflict(data map[string]interface{}) string {
	var parts []string
	for k := range data {
		if k == "id" {
			continue
		}
		qf := quotePgsqlField(k)
		parts = append(parts, fmt.Sprintf("%s = EXCLUDED.%s", qf, qf))
	}
	return strings.Join(parts, ", ")
}

// buildPgsqlConds 构建 WHERE 条件（使用 $1, $2 等占位符）
func buildPgsqlConds(conds Conds) (string, []interface{}) {
	if len(conds) == 0 {
		return "", nil
	}
	var sb strings.Builder
	sb.Grow(len(conds) * 32) // 预分配容量
	args := make([]interface{}, 0, len(conds))
	paramIdx := 1

	for i, c := range conds {
		if i > 0 {
			sb.WriteString(" AND ")
		}
		qf := quotePgsqlField(c.Field)
		op := strings.ToLower(c.Cond)
		switch op {
		case ">", ">=", "<", "<=", "=", "==", "!=":
			if op == "==" {
				op = "="
			}
			sb.WriteString(fmt.Sprintf("%s %s $%d", qf, op, paramIdx))
			args = append(args, c.Value)
			paramIdx++
		case "in", "array-contains":
			vals := toSlice(c.Value)
			phs := make([]string, len(vals))
			for j := range vals {
				phs[j] = fmt.Sprintf("$%d", paramIdx)
				args = append(args, vals[j])
				paramIdx++
			}
			sb.WriteString(qf + " IN (" + strings.Join(phs, ", ") + ")")
		case "not-in":
			vals := toSlice(c.Value)
			phs := make([]string, len(vals))
			for j := range vals {
				phs[j] = fmt.Sprintf("$%d", paramIdx)
				args = append(args, vals[j])
				paramIdx++
			}
			sb.WriteString(qf + " NOT IN (" + strings.Join(phs, ", ") + ")")
		case "like":
			sb.WriteString(fmt.Sprintf("%s LIKE $%d", qf, paramIdx))
			args = append(args, c.Value)
			paramIdx++
		case "ilike": // PostgreSQL 特有的大小写不敏感 LIKE
			sb.WriteString(fmt.Sprintf("%s ILIKE $%d", qf, paramIdx))
			args = append(args, c.Value)
			paramIdx++
		case "is null":
			sb.WriteString(qf + " IS NULL")
		case "is not null":
			sb.WriteString(qf + " IS NOT NULL")
		default:
			sb.WriteString(fmt.Sprintf("%s = $%d", qf, paramIdx))
			args = append(args, c.Value)
			paramIdx++
		}
	}
	return sb.String(), args
}

// isPgsqlDuplicateError 判断是否是重复键错误
func isPgsqlDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	// PostgreSQL 重复键错误码是 23505
	return strings.Contains(errStr, "duplicate") ||
		strings.Contains(errStr, "23505") ||
		strings.Contains(errStr, "unique_violation")
}

