package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/spf13/cast"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/su_time"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
)

// MysqlDW 不依赖 gorm，基于原生 SQL 实现，不使用 prepare 方式
// 专为数仓数据库（StarRocks、MaxCompute 等）设计
// 注意：StarRocks 的 INSERT INTO 在主键冲突时会自动替换旧记录
type MysqlDW struct {
	db *sql.DB
}

// NewMysqlDW 创建 MysqlDW 实例
func NewMysqlDW(config *MysqlConfig) (*MysqlDW, error) {
	dsn := parseDsn(&config.Master)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open mysql connection: %w", err)
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
		return nil, fmt.Errorf("failed to ping mysql: %w", err)
	}

	return &MysqlDW{db: db}, nil
}

// NewMysqlDWFromDB 从已有的 sql.DB 创建 MysqlDW
func NewMysqlDWFromDB(db *sql.DB) *MysqlDW {
	return &MysqlDW{db: db}
}

// DB 返回底层的 sql.DB
func (m *MysqlDW) DB() *sql.DB {
	return m.db
}

// Close 关闭数据库连接
func (m *MysqlDW) Close() error {
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

// ==================== SQL 值转义函数 ====================

// escapeDWString 转义字符串，防止 SQL 注入
func escapeDWString(s string) string {
	var buf strings.Builder
	buf.Grow(len(s) * 2)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case 0:
			buf.WriteString("\\0")
		case '\n':
			buf.WriteString("\\n")
		case '\r':
			buf.WriteString("\\r")
		case '\\':
			buf.WriteString("\\\\")
		case '\'':
			buf.WriteString("\\'")
		case '"':
			buf.WriteString("\\\"")
		case '\x1a': // ASCII 26
			buf.WriteString("\\Z")
		default:
			buf.WriteByte(c)
		}
	}
	return buf.String()
}

// escapeDWValue 将值转换为 SQL 安全的字符串表示
func escapeDWValue(v interface{}) string {
	if v == nil {
		return "NULL"
	}

	switch val := v.(type) {
	case string:
		return "'" + escapeDWString(val) + "'"
	case []byte:
		return "'" + escapeDWString(string(val)) + "'"
	case int:
		return strconv.FormatInt(int64(val), 10)
	case int8:
		return strconv.FormatInt(int64(val), 10)
	case int16:
		return strconv.FormatInt(int64(val), 10)
	case int32:
		return strconv.FormatInt(int64(val), 10)
	case int64:
		return strconv.FormatInt(val, 10)
	case uint:
		return strconv.FormatUint(uint64(val), 10)
	case uint8:
		return strconv.FormatUint(uint64(val), 10)
	case uint16:
		return strconv.FormatUint(uint64(val), 10)
	case uint32:
		return strconv.FormatUint(uint64(val), 10)
	case uint64:
		return strconv.FormatUint(val, 10)
	case float32:
		return strconv.FormatFloat(float64(val), 'g', -1, 32)
	case float64:
		return strconv.FormatFloat(val, 'g', -1, 64)
	case bool:
		if val {
			return "1"
		}
		return "0"
	case time.Time:
		return "'" + val.Format("2006-01-02 15:04:05") + "'"
	default:
		// 尝试使用 reflect 处理其他类型
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Ptr:
			if rv.IsNil() {
				return "NULL"
			}
			return escapeDWValue(rv.Elem().Interface())
		case reflect.Slice, reflect.Array:
			// 对于切片/数组，递归处理每个元素
			var parts []string
			for i := 0; i < rv.Len(); i++ {
				parts = append(parts, escapeDWValue(rv.Index(i).Interface()))
			}
			return strings.Join(parts, ", ")
		default:
			// 其他类型转为字符串
			return "'" + escapeDWString(cast.ToString(v)) + "'"
		}
	}
}

// ==================== 核心 CRUD 方法 ====================

// Insert 创建一条新记录, 记录存在则进行覆盖
// StarRocks 的 INSERT INTO 在主键冲突时会自动替换旧记录
func (m *MysqlDW) Insert(ctx context.Context, table string, row Row, options ...*InsertOptions) (string, error) {
	opt := dftInsertOptions()
	opt.Apply(options...)

	data, err := rowToMap(row)
	if err != nil {
		su_logger.Error(ctx, err, "mysql dw insert marshal failed")
		return "", err
	}

	// StarRocks INSERT INTO 主键冲突时自动替换
	columns, values := buildDWInsertParts(data)
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);", quoteDWTable(table), columns, values)

	_, err = m.db.ExecContext(ctx, query)
	if err != nil {
		su_logger.Error(ctx, err, "mysql dw insert failed", su_logger.E().String("table", table).String("sql", query))
		return "", err
	}

	id := cast.ToString(data["id"])
	if opt.Hook() != nil {
		opt.Hook()(ctx, ChangeRow{Table: table, Id: id, Action: ActionAdd})
	}
	return id, nil
}

// Delete 删除一条记录
func (m *MysqlDW) Delete(ctx context.Context, table string, id string, options ...*DeleteOptions) (int, error) {
	opt := dftDeleteOptions()
	opt.Apply(options...)

	query := fmt.Sprintf("DELETE FROM %s WHERE `id` = %s LIMIT 1;", quoteDWTable(table), escapeDWValue(id))
	result, err := m.db.ExecContext(ctx, query)
	if err != nil {
		su_logger.Error(ctx, err, "mysql dw delete failed", su_logger.E().String("table", table).String("id", id))
		return 0, err
	}

	affected, _ := result.RowsAffected()
	if opt.Hook() != nil {
		opt.Hook()(ctx, ChangeRow{Table: table, Id: id, Action: ActionDelete})
	}
	return int(affected), nil
}

// Create 创建一条新记录, 如果记录存在返回Error
func (m *MysqlDW) Create(ctx context.Context, table string, row Row, options ...*CreateOptions) (*CreateResult, error) {
	opt := dftCreateOptions()
	opt.Apply(options...)

	data, err := rowToMap(row)
	if err != nil {
		su_logger.Error(ctx, err, "mysql dw create marshal failed")
		return nil, err
	}

	columns, values := buildDWInsertParts(data)
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);", quoteDWTable(table), columns, values)

	_, err = m.db.ExecContext(ctx, query)
	if err != nil {
		if isDWDuplicateError(err) {
			return nil, ErrDocAlreadyExists
		}
		su_logger.Error(ctx, err, "mysql dw create failed", su_logger.E().String("table", table))
		return nil, err
	}

	id := cast.ToString(data["id"])
	if opt.Hook() != nil {
		opt.Hook()(ctx, ChangeRow{Table: table, Id: id, Action: ActionAdd})
	}
	return &CreateResult{ID: id}, nil
}

// Update 更新一条记录
func (m *MysqlDW) Update(ctx context.Context, table string, update UpdateOne, options ...*UpdateOptions) error {
	opt := dftUpdateOptions()
	opt.Apply(options...)

	if update.ID == "" {
		return errors.New("empty id in UpdateOne")
	}

	if len(update.Updates) == 0 {
		return nil
	}

	var setParts []string
	for _, u := range update.Updates {
		qf := quoteDWField(u.Field)
		switch strings.ToLower(u.Op) {
		case "incr":
			setParts = append(setParts, fmt.Sprintf("%s = %s + %s", qf, qf, escapeDWValue(u.Value)))
		default:
			setParts = append(setParts, fmt.Sprintf("%s = %s", qf, escapeDWValue(u.Value)))
		}
	}

	query := fmt.Sprintf("UPDATE %s SET %s WHERE `id` = %s;",
		quoteDWTable(table), strings.Join(setParts, ", "), escapeDWValue(update.ID))

	_, err := m.db.ExecContext(ctx, query)
	if err != nil {
		su_logger.Error(ctx, err, "mysql dw update failed", su_logger.E().String("table", table).String("id", update.ID))
		return err
	}

	if opt.Hook() != nil {
		opt.Hook()(ctx, ChangeRow{Table: table, Id: update.ID, Action: ActionUpdate, Data: updatesToMapStringInterface(update.Updates)})
	}
	return nil
}

// Get 读取一条记录
func (m *MysqlDW) Get(ctx context.Context, table string, id string, options ...*GetOptions) (*DocumentRef, error) {
	var fields string = "*"
	if len(options) > 0 && options[0] != nil && len(options[0].Fields) > 0 {
		fields = strings.Join(options[0].Fields, ", ")
	}

	query := fmt.Sprintf("SELECT %s FROM %s WHERE `id` = %s LIMIT 1;", fields, quoteDWTable(table), escapeDWValue(id))
	rows, err := m.db.QueryContext(ctx, query)
	if err != nil {
		su_logger.Error(ctx, err, "mysql dw get failed", su_logger.E().String("table", table).String("id", id))
		return nil, err
	}
	defer rows.Close()

	items, err := scanDWRowsToMaps(rows)
	if err != nil {
		su_logger.Error(ctx, err, "mysql dw get scan failed", su_logger.E().String("table", table).String("id", id))
		return nil, err
	}

	if len(items) == 0 {
		return nil, ErrDocNotFound
	}

	return &DocumentRef{ID: id, Data: items[0]}, nil
}

// ==================== 查询方法 ====================

// Find 查询多条记录
func (m *MysqlDW) Find(ctx context.Context, table string, conds Conds, options ...*FindOptions) (*Iterator, error) {
	var fields string = "*"
	var opt *FindOptions
	if len(options) > 0 && options[0] != nil {
		opt = options[0]
		if len(opt.Fields) > 0 {
			fields = strings.Join(opt.Fields, ", ")
		}
	}

	query := fmt.Sprintf("SELECT %s FROM %s", fields, quoteDWTable(table))

	// 构建 WHERE 条件
	if where := buildDWSQLConds(conds); where != "" {
		query += " WHERE " + where
	}

	// 构建 ORDER BY
	if opt != nil && len(opt.Sorts) > 0 {
		var orderParts []string
		for _, s := range opt.Sorts {
			dir := "ASC"
			if s.Order < 0 {
				dir = "DESC"
			}
			orderParts = append(orderParts, fmt.Sprintf("%s %s", s.Field, dir))
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
	query += ";"

	rows, err := m.db.QueryContext(ctx, query)
	if err != nil {
		su_logger.Error(ctx, err, "mysql dw find failed", su_logger.E().String("table", table))
		return nil, err
	}
	defer rows.Close()

	items, err := scanDWRowsToMaps(rows)
	if err != nil {
		su_logger.Error(ctx, err, "mysql dw find scan failed", su_logger.E().String("table", table))
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
func (m *MysqlDW) Count(ctx context.Context, table string, conds Conds) (int64, error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", quoteDWTable(table))

	if where := buildDWSQLConds(conds); where != "" {
		query += " WHERE " + where
	}
	query += ";"

	var count int64
	err := m.db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		su_logger.Error(ctx, err, "mysql dw count failed", su_logger.E().String("table", table))
		return 0, err
	}
	return count, nil
}

// ==================== 批量操作方法 ====================

// BatchCreate 批量创建记录, 如果记录存在则返回错误
func (m *MysqlDW) BatchCreate(ctx context.Context, table string, rows []Row, options ...*BatchWriteOptions) *BatchWriteResult {
	opt := dftBatchWriteOptions()
	opt.Apply(options...)
	result := &BatchWriteResult{Ids: make([]string, 0, len(rows)), Errors: make([]error, 0)}

	if len(rows) == 0 {
		return result
	}

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

// BatchInsert 批量插入记录, 如果记录存在则进行覆盖
func (m *MysqlDW) BatchInsert(ctx context.Context, table string, rows []Row, options ...*BatchWriteOptions) *BatchWriteResult {
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

// BatchDelete 批量删除记录
func (m *MysqlDW) BatchDelete(ctx context.Context, table string, ids []string, options ...*BatchWriteOptions) *BatchWriteResult {
	opt := dftBatchWriteOptions()
	opt.Apply(options...)
	result := &BatchWriteResult{Errors: make([]error, 0)}

	if len(ids) == 0 {
		return result
	}

	// 构建 IN 列表
	var escapedIds []string
	for _, id := range ids {
		escapedIds = append(escapedIds, escapeDWValue(id))
	}

	query := fmt.Sprintf("DELETE FROM %s WHERE `id` IN (%s);", quoteDWTable(table), strings.Join(escapedIds, ", "))
	res, err := m.db.ExecContext(ctx, query)
	if err != nil {
		su_logger.Error(ctx, err, "mysql dw batch delete failed", su_logger.E().String("table", table))
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
func (m *MysqlDW) BatchUpdate(ctx context.Context, table string, updates []UpdateOne, options ...*BatchWriteOptions) *BatchWriteResult {
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

		affected, err := m.batchUpdateWithCaseWhenDW(ctx, table, batch, opt)
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

// batchUpdateWithCaseWhenDW 使用 CASE WHEN 实现批量更新
func (m *MysqlDW) batchUpdateWithCaseWhenDW(ctx context.Context, table string, updates []UpdateOne, opt *BatchWriteOptions) (int64, error) {
	if len(updates) == 0 {
		return 0, nil
	}

	// 1. 收集所有字段的更新信息
	updateFieldMap := make(map[string][]struct {
		id    string
		value interface{}
		op    string
	})

	var ids []string
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

	for field, fieldUpdates := range updateFieldMap {
		qf := quoteDWField(field)
		var caseWhen strings.Builder
		caseWhen.Grow(len(fieldUpdates) * 64)

		// 检查是否有 incr 操作
		hasIncr := false
		for _, u := range fieldUpdates {
			if strings.ToLower(u.op) == "incr" {
				hasIncr = true
				break
			}
		}

		if hasIncr {
			caseWhen.WriteString(qf + " = " + qf + " + CASE `id`")
			for _, u := range fieldUpdates {
				caseWhen.WriteString(fmt.Sprintf(" WHEN %s THEN %s", escapeDWValue(u.id), escapeDWValue(u.value)))
			}
			caseWhen.WriteString(" ELSE 0 END")
		} else {
			caseWhen.WriteString(qf + " = CASE `id`")
			for _, u := range fieldUpdates {
				caseWhen.WriteString(fmt.Sprintf(" WHEN %s THEN %s", escapeDWValue(u.id), escapeDWValue(u.value)))
			}
			caseWhen.WriteString(" ELSE " + qf + " END")
		}

		setParts = append(setParts, caseWhen.String())
	}

	// 3. 构建 WHERE IN 部分
	var escapedIds []string
	for _, id := range ids {
		escapedIds = append(escapedIds, escapeDWValue(id))
	}

	// 4. 构建完整 SQL
	query := fmt.Sprintf("UPDATE %s SET %s WHERE `id` IN (%s);",
		quoteDWTable(table),
		strings.Join(setParts, ", "),
		strings.Join(escapedIds, ", "))

	res, err := m.db.ExecContext(ctx, query)
	if err != nil {
		su_logger.Error(ctx, err, "mysql dw batch update failed", su_logger.E().String("table", table))
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
func (m *MysqlDW) BatchGet(ctx context.Context, table string, ids []string, options ...*GetOptions) (*Iterator, error) {
	if len(ids) == 0 {
		return &Iterator{items: []*DocumentRef{}}, nil
	}

	var fields string = "*"
	if len(options) > 0 && options[0] != nil && len(options[0].Fields) > 0 {
		fields = strings.Join(options[0].Fields, ", ")
	}

	var escapedIds []string
	for _, id := range ids {
		escapedIds = append(escapedIds, escapeDWValue(id))
	}

	query := fmt.Sprintf("SELECT %s FROM %s WHERE `id` IN (%s);", fields, quoteDWTable(table), strings.Join(escapedIds, ", "))
	rows, err := m.db.QueryContext(ctx, query)
	if err != nil {
		su_logger.Error(ctx, err, "mysql dw batch get failed", su_logger.E().String("table", table))
		return nil, err
	}
	defer rows.Close()

	items, err := scanDWRowsToMaps(rows)
	if err != nil {
		su_logger.Error(ctx, err, "mysql dw batch get scan failed", su_logger.E().String("table", table))
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
// StarRocks 的 INSERT INTO 在主键冲突时会自动替换旧记录，incr 操作在数仓中不支持
func (m *MysqlDW) Upsert(ctx context.Context, table string, row UpsertRow, options ...*UpsertOptions) (*UpsertRs, error) {
	opt := dftUpsertOptions()
	opt.Apply(options...)

	if row.Id == "" {
		return nil, errors.New("empty id in Upsert")
	}

	// 构建插入数据（合并 Inserts 和 Updates）
	values := map[string]interface{}{"id": row.Id}
	for _, u := range row.Inserts {
		values[u.Field] = u.Value
	}
	for _, u := range row.Updates {
		// 数仓不支持 incr 操作，直接使用值
		values[u.Field] = u.Value
	}

	columns, vals := buildDWInsertParts(values)
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);", quoteDWTable(table), columns, vals)

	_, err := m.db.ExecContext(ctx, query)
	if err != nil {
		su_logger.Error(ctx, err, "mysql dw upsert failed", su_logger.E().String("table", table).String("id", row.Id))
		return nil, err
	}

	if opt.ChangeHook != nil {
		opt.ChangeHook(ctx, ChangeRow{Table: table, Id: row.Id, Action: ActionUpdate, Data: updatesToMapStringInterface(row.Updates)})
	}
	return &UpsertRs{Id: row.Id, MatchCount: 1}, nil
}

// UpsertSingleField 对单个字段进行更新或插入
// StarRocks 的 INSERT INTO 在主键冲突时会自动替换旧记录
func (m *MysqlDW) UpsertSingleField(ctx context.Context, table string, row UpsertSingleFields, options ...*UpsertSingleFieldRowOptions) error {
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

	columns, vals := buildDWInsertParts(insertVals)
	var query string

	if mergeAll {
		// StarRocks INSERT INTO 主键冲突时自动替换
		query = fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);", quoteDWTable(table), columns, vals)
	} else {
		// DoNothing - 使用 INSERT IGNORE
		query = fmt.Sprintf("INSERT IGNORE INTO %s (%s) VALUES (%s);", quoteDWTable(table), columns, vals)
	}

	_, err := m.db.ExecContext(ctx, query)
	if err != nil {
		su_logger.Error(ctx, err, "mysql dw upsert single field failed", su_logger.E().String("table", table).String("id", row.Id))
		return err
	}
	return nil
}

// BatchUpsert 批量更新或插入记录
// StarRocks 的 INSERT INTO 在主键冲突时会自动替换旧记录
func (m *MysqlDW) BatchUpsert(ctx context.Context, table string, rows []UpsertRow, options ...*BatchWriteOptions) (*BatchUpsertRs, error) {
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

		if err := m.batchUpsertDW(ctx, table, batch, opt); err != nil {
			if opt.ContinueOnError != 1 {
				return nil, err
			}
		}
	}

	return rs, nil
}

// batchUpsertDW 使用 INSERT INTO 实现批量 upsert
// StarRocks 的 INSERT INTO 在主键冲突时会自动替换旧记录
func (m *MysqlDW) batchUpsertDW(ctx context.Context, table string, rows []UpsertRow, opt *BatchWriteOptions) error {
	if len(rows) == 0 {
		return nil
	}

	// 1. 收集所有字段
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

	for _, row := range rows {
		rowData := make(map[string]interface{})
		rowData["id"] = row.Id
		for _, u := range row.Inserts {
			rowData[u.Field] = u.Value
		}
		for _, u := range row.Updates {
			// 数仓不支持 incr，直接使用值
			rowData[u.Field] = u.Value
		}

		var vals []string
		for _, col := range columns {
			if val, ok := rowData[col]; ok {
				vals = append(vals, escapeDWValue(val))
			} else {
				vals = append(vals, "DEFAULT")
			}
		}
		valueRows = append(valueRows, "("+strings.Join(vals, ", ")+")")
	}

	// 4. 构建完整 SQL（StarRocks INSERT INTO 主键冲突时自动替换）
	var quotedColumns []string
	for _, col := range columns {
		quotedColumns = append(quotedColumns, quoteDWField(col))
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s;",
		quoteDWTable(table),
		strings.Join(quotedColumns, ", "),
		strings.Join(valueRows, ", "))

	_, err := m.db.ExecContext(ctx, query)
	if err != nil {
		su_logger.Error(ctx, err, "mysql dw batch upsert failed", su_logger.E().String("table", table))
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
func (m *MysqlDW) ToUpdateOne(data any, options *UpdateOptions) (upData UpdateOne) {
	opt := dftMysqlDWToUpdateOptions()
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
	processStructMysqlDW(v, t, opt, &upData, &containUpdateField)
	if len(upData.Updates) > 0 {
		if !containUpdateField && !opt.IgnoreUpdateTimeField {
			upData.Updates = append(upData.Updates, Update{Field: UpdateField, Value: su_time.CurrentTimestampMilli()})
		}
	}
	return upData
}

func dftMysqlDWToUpdateOptions() *UpdateOptions {
	return &UpdateOptions{
		Tag: "json",
	}
}

func processStructMysqlDW(v reflect.Value, t reflect.Type, opt *UpdateOptions, upData *UpdateOne, containUpdateField *bool) {
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

// ==================== DW 专用辅助函数 ====================

// quoteDWTable 对表名添加反引号
func quoteDWTable(table string) string {
	if strings.Contains(table, "`") {
		return table
	}
	if idx := strings.Index(table, "."); idx != -1 {
		dbName := table[:idx]
		tableName := table[idx+1:]
		return "`" + dbName + "`.`" + tableName + "`"
	}
	return "`" + table + "`"
}

// quoteDWField 对字段名添加反引号
func quoteDWField(field string) string {
	if strings.Contains(field, "`") {
		return field
	}
	if idx := strings.LastIndex(field, "."); idx != -1 {
		prefix := field[:idx+1]
		name := field[idx+1:]
		return prefix + "`" + name + "`"
	}
	return "`" + field + "`"
}

// buildDWInsertParts 构建 INSERT 语句的列和值（直接拼装）
func buildDWInsertParts(data map[string]interface{}) (columns string, values string) {
	var cols []string
	var vals []string
	for k, v := range data {
		cols = append(cols, quoteDWField(k))
		vals = append(vals, escapeDWValue(v))
	}
	return strings.Join(cols, ", "), strings.Join(vals, ", ")
}

// buildDWSQLConds 构建 WHERE 条件（直接拼装值）
func buildDWSQLConds(conds Conds) string {
	if len(conds) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.Grow(len(conds) * 48)

	for i, c := range conds {
		if i > 0 {
			sb.WriteString(" AND ")
		}
		qf := quoteDWField(c.Field)
		op := strings.ToLower(c.Cond)
		switch op {
		case ">", ">=", "<", "<=", "=", "==", "!=":
			if op == "==" {
				op = "="
			}
			sb.WriteString(qf + " " + op + " " + escapeDWValue(c.Value))
		case "in", "array-contains":
			sb.WriteString(qf + " IN (")
			vals := toDWSlice(c.Value)
			var escapedVals []string
			for _, v := range vals {
				escapedVals = append(escapedVals, escapeDWValue(v))
			}
			sb.WriteString(strings.Join(escapedVals, ", "))
			sb.WriteString(")")
		case "not-in":
			sb.WriteString(qf + " NOT IN (")
			vals := toDWSlice(c.Value)
			var escapedVals []string
			for _, v := range vals {
				escapedVals = append(escapedVals, escapeDWValue(v))
			}
			sb.WriteString(strings.Join(escapedVals, ", "))
			sb.WriteString(")")
		case "like":
			sb.WriteString(qf + " LIKE " + escapeDWValue(c.Value))
		case "is null":
			sb.WriteString(qf + " IS NULL")
		case "is not null":
			sb.WriteString(qf + " IS NOT NULL")
		default:
			sb.WriteString(qf + " = " + escapeDWValue(c.Value))
		}
	}
	return sb.String()
}

// toDWSlice 将 interface{} 转换为 []interface{}
func toDWSlice(v interface{}) []interface{} {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return []interface{}{v}
	}
	result := make([]interface{}, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		result[i] = rv.Index(i).Interface()
	}
	return result
}

// isDWDuplicateError 判断是否是重复键错误
func isDWDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "duplicate") || strings.Contains(errStr, "1062")
}

// scanDWRowsToMaps 将多行扫描为 []map
func scanDWRowsToMaps(rows *sql.Rows) ([]map[string]interface{}, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var result []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		result = append(result, row)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
