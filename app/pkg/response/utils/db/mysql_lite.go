package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/spf13/cast"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/su_time"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
)

// MysqlLite 不依赖 gorm，基于原生 SQL 实现
type MysqlLite struct {
	db *sql.DB
}

// NewMysqlLite 创建 MysqlLite 实例，使用统一的 MysqlConfig 配置
func NewMysqlLite(config *MysqlConfig) (*MysqlLite, error) {
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

	return &MysqlLite{db: db}, nil
}

// NewMysqlLiteFromDB 从已有的 sql.DB 创建 MysqlLite
func NewMysqlLiteFromDB(db *sql.DB) *MysqlLite {
	return &MysqlLite{db: db}
}

// DB 返回底层的 sql.DB
func (m *MysqlLite) DB() *sql.DB {
	return m.db
}

// Close 关闭数据库连接
func (m *MysqlLite) Close() error {
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

// ==================== 核心 CRUD 方法 ====================

// Insert 创建一条新记录, 记录存在则进行覆盖
func (m *MysqlLite) Insert(ctx context.Context, table string, row Row, options ...*InsertOptions) (string, error) {
	opt := dftInsertOptions()
	opt.Apply(options...)

	data, err := rowToMap(row)
	if err != nil {
		su_logger.Error(ctx, err, "mysql lite insert marshal failed")
		return "", err
	}

	// 构建 INSERT ... ON DUPLICATE KEY UPDATE 语句
	columns, placeholders, values := buildInsertParts(data)
	updateParts := buildUpdateOnDuplicate(data)

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON DUPLICATE KEY UPDATE %s",
		quoteTable(table), columns, placeholders, updateParts)

	_, err = m.db.ExecContext(ctx, query, values...)
	if err != nil {
		su_logger.Error(ctx, err, "mysql lite insert failed", su_logger.E().String("table", table))
		return "", err
	}

	id := cast.ToString(data["id"])
	if opt.Hook() != nil {
		opt.Hook()(ctx, ChangeRow{Table: table, Id: id, Action: ActionAdd})
	}
	return id, nil
}

// Delete 删除一条记录
func (m *MysqlLite) Delete(ctx context.Context, table string, id string, options ...*DeleteOptions) (int, error) {
	opt := dftDeleteOptions()
	opt.Apply(options...)

	query := fmt.Sprintf("DELETE FROM %s WHERE `id` = ? LIMIT 1", quoteTable(table))
	result, err := m.db.ExecContext(ctx, query, id)
	if err != nil {
		su_logger.Error(ctx, err, "mysql lite delete failed", su_logger.E().String("table", table).String("id", id))
		return 0, err
	}

	affected, _ := result.RowsAffected()
	if opt.Hook() != nil {
		opt.Hook()(ctx, ChangeRow{Table: table, Id: id, Action: ActionDelete})
	}
	return int(affected), nil
}

// Create 创建一条新记录, 如果记录存在返回Error
func (m *MysqlLite) Create(ctx context.Context, table string, row Row, options ...*CreateOptions) (*CreateResult, error) {
	opt := dftCreateOptions()
	opt.Apply(options...)

	data, err := rowToMap(row)
	if err != nil {
		su_logger.Error(ctx, err, "mysql lite create marshal failed")
		return nil, err
	}

	columns, placeholders, values := buildInsertParts(data)
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", quoteTable(table), columns, placeholders)

	_, err = m.db.ExecContext(ctx, query, values...)
	if err != nil {
		if isDuplicateError(err) {
			return nil, ErrDocAlreadyExists
		}
		su_logger.Error(ctx, err, "mysql lite create failed", su_logger.E().String("table", table))
		return nil, err
	}

	id := cast.ToString(data["id"])
	if opt.Hook() != nil {
		opt.Hook()(ctx, ChangeRow{Table: table, Id: id, Action: ActionAdd})
	}
	return &CreateResult{ID: id}, nil
}

// Update 更新一条记录
func (m *MysqlLite) Update(ctx context.Context, table string, update UpdateOne, options ...*UpdateOptions) error {
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

	for _, u := range update.Updates {
		qf := quoteField(u.Field)
		switch strings.ToLower(u.Op) {
		case "incr":
			setParts = append(setParts, fmt.Sprintf("%s = %s + ?", qf, qf))
			values = append(values, u.Value)
		default:
			setParts = append(setParts, fmt.Sprintf("%s = ?", qf))
			values = append(values, u.Value)
		}
	}

	values = append(values, update.ID)
	query := fmt.Sprintf("UPDATE %s SET %s WHERE `id` = ?", quoteTable(table), strings.Join(setParts, ", "))

	_, err := m.db.ExecContext(ctx, query, values...)
	if err != nil {
		su_logger.Error(ctx, err, "mysql lite update failed", su_logger.E().String("table", table).String("id", update.ID))
		return err
	}

	if opt.Hook() != nil {
		opt.Hook()(ctx, ChangeRow{Table: table, Id: update.ID, Action: ActionUpdate, Data: updatesToMapStringInterface(update.Updates)})
	}
	return nil
}

// Get 读取一条记录
func (m *MysqlLite) Get(ctx context.Context, table string, id string, options ...*GetOptions) (*DocumentRef, error) {
	var fields string = "*"
	if len(options) > 0 && options[0] != nil && len(options[0].Fields) > 0 {
		fields = strings.Join(options[0].Fields, ", ")
	}

	query := fmt.Sprintf("SELECT %s FROM %s WHERE `id` = ? LIMIT 1", fields, quoteTable(table))
	rows, err := m.db.QueryContext(ctx, query, id)
	if err != nil {
		su_logger.Error(ctx, err, "mysql lite get failed", su_logger.E().String("table", table).String("id", id))
		return nil, err
	}
	defer rows.Close()

	items, err := scanRowsToMaps(rows)
	if err != nil {
		su_logger.Error(ctx, err, "mysql lite get scan failed", su_logger.E().String("table", table).String("id", id))
		return nil, err
	}

	if len(items) == 0 {
		return nil, ErrDocNotFound
	}

	return &DocumentRef{ID: id, Data: items[0]}, nil
}

// ==================== 查询方法 ====================

// Find 查询多条记录
func (m *MysqlLite) Find(ctx context.Context, table string, conds Conds, options ...*FindOptions) (*Iterator, error) {
	var fields string = "*"
	var opt *FindOptions
	if len(options) > 0 && options[0] != nil {
		opt = options[0]
		if len(opt.Fields) > 0 {
			fields = strings.Join(opt.Fields, ", ")
		}
	}

	query := fmt.Sprintf("SELECT %s FROM %s", fields, quoteTable(table))
	var args []interface{}

	// 构建 WHERE 条件
	if where, whereArgs := buildSQLCondsLite(conds); where != "" {
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

	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		su_logger.Error(ctx, err, "mysql lite find failed", su_logger.E().String("table", table))
		return nil, err
	}
	defer rows.Close()

	items, err := scanRowsToMaps(rows)
	if err != nil {
		su_logger.Error(ctx, err, "mysql lite find scan failed", su_logger.E().String("table", table))
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
func (m *MysqlLite) Count(ctx context.Context, table string, conds Conds) (int64, error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", quoteTable(table))
	var args []interface{}

	if where, whereArgs := buildSQLCondsLite(conds); where != "" {
		query += " WHERE " + where
		args = append(args, whereArgs...)
	}

	var count int64
	err := m.db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		su_logger.Error(ctx, err, "mysql lite count failed", su_logger.E().String("table", table))
		return 0, err
	}
	return count, nil
}

// ==================== 批量操作方法 ====================

// BatchCreate 批量创建记录, 如果记录存在则返回错误
func (m *MysqlLite) BatchCreate(ctx context.Context, table string, rows []Row, options ...*BatchWriteOptions) *BatchWriteResult {
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
func (m *MysqlLite) BatchInsert(ctx context.Context, table string, rows []Row, options ...*BatchWriteOptions) *BatchWriteResult {
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
func (m *MysqlLite) BatchDelete(ctx context.Context, table string, ids []string, options ...*BatchWriteOptions) *BatchWriteResult {
	opt := dftBatchWriteOptions()
	opt.Apply(options...)
	result := &BatchWriteResult{Errors: make([]error, 0)}

	if len(ids) == 0 {
		return result
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf("DELETE FROM %s WHERE `id` IN (%s)", quoteTable(table), strings.Join(placeholders, ", "))
	res, err := m.db.ExecContext(ctx, query, args...)
	if err != nil {
		su_logger.Error(ctx, err, "mysql lite batch delete failed", su_logger.E().String("table", table))
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
func (m *MysqlLite) BatchUpdate(ctx context.Context, table string, updates []UpdateOne, options ...*BatchWriteOptions) *BatchWriteResult {
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

		affected, err := m.batchUpdateWithCaseWhen(ctx, table, batch, opt)
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
func (m *MysqlLite) batchUpdateWithCaseWhen(ctx context.Context, table string, updates []UpdateOne, opt *BatchWriteOptions) (int64, error) {
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

	for field, fieldUpdates := range updateFieldMap {
		qf := quoteField(field)
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
			caseWhen.WriteString(qf + " = " + qf + " + CASE `id`")
			for _, u := range fieldUpdates {
				caseWhen.WriteString(" WHEN ? THEN ?")
				args = append(args, u.id, u.value)
			}
			caseWhen.WriteString(" ELSE 0 END")
		} else {
			// 普通更新使用 CASE WHEN
			caseWhen.WriteString(qf + " = CASE `id`")
			for _, u := range fieldUpdates {
				caseWhen.WriteString(" WHEN ? THEN ?")
				args = append(args, u.id, u.value)
			}
			caseWhen.WriteString(" ELSE " + qf + " END")
		}

		setParts = append(setParts, caseWhen.String())
	}

	// 3. 构建 WHERE IN 部分
	placeholders := make([]string, len(ids))
	for i := range ids {
		placeholders[i] = "?"
	}
	args = append(args, ids...)

	// 4. 构建完整 SQL
	query := fmt.Sprintf("UPDATE %s SET %s WHERE `id` IN (%s)",
		quoteTable(table),
		strings.Join(setParts, ", "),
		strings.Join(placeholders, ", "))

	res, err := m.db.ExecContext(ctx, query, args...)
	if err != nil {
		su_logger.Error(ctx, err, "mysql lite batch update failed", su_logger.E().String("table", table))
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
func (m *MysqlLite) BatchGet(ctx context.Context, table string, ids []string, options ...*GetOptions) (*Iterator, error) {
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
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf("SELECT %s FROM %s WHERE `id` IN (%s)", fields, quoteTable(table), strings.Join(placeholders, ", "))
	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		su_logger.Error(ctx, err, "mysql lite batch get failed", su_logger.E().String("table", table))
		return nil, err
	}
	defer rows.Close()

	items, err := scanRowsToMaps(rows)
	if err != nil {
		su_logger.Error(ctx, err, "mysql lite batch get scan failed", su_logger.E().String("table", table))
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
func (m *MysqlLite) Upsert(ctx context.Context, table string, row UpsertRow, options ...*UpsertOptions) (*UpsertRs, error) {
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
	for _, u := range row.Updates {
		qf := quoteField(u.Field)
		if strings.ToLower(u.Op) == "incr" {
			updateParts = append(updateParts, fmt.Sprintf("%s = %s + ?", qf, qf))
		} else {
			updateParts = append(updateParts, fmt.Sprintf("%s = ?", qf))
		}
		updateArgs = append(updateArgs, u.Value)
	}

	columns, placeholders, insertArgs := buildInsertParts(values)
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", quoteTable(table), columns, placeholders)

	if len(updateParts) > 0 {
		query += " ON DUPLICATE KEY UPDATE " + strings.Join(updateParts, ", ")
		insertArgs = append(insertArgs, updateArgs...)
	}

	_, err := m.db.ExecContext(ctx, query, insertArgs...)
	if err != nil {
		su_logger.Error(ctx, err, "mysql lite upsert failed", su_logger.E().String("table", table).String("id", row.Id))
		return nil, err
	}

	if opt.ChangeHook != nil {
		opt.ChangeHook(ctx, ChangeRow{Table: table, Id: row.Id, Action: ActionUpdate, Data: updatesToMapStringInterface(row.Updates)})
	}
	return &UpsertRs{Id: row.Id, MatchCount: 1}, nil
}

// UpsertSingleField 对单个字段进行更新或插入
func (m *MysqlLite) UpsertSingleField(ctx context.Context, table string, row UpsertSingleFields, options ...*UpsertSingleFieldRowOptions) error {
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

	columns, placeholders, values := buildInsertParts(insertVals)
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", quoteTable(table), columns, placeholders)

	if mergeAll {
		updateParts := buildUpdateOnDuplicate(insertVals)
		query += " ON DUPLICATE KEY UPDATE " + updateParts
	} else {
		// DoNothing - 使用 INSERT IGNORE
		query = fmt.Sprintf("INSERT IGNORE INTO %s (%s) VALUES (%s)", quoteTable(table), columns, placeholders)
	}

	_, err := m.db.ExecContext(ctx, query, values...)
	if err != nil {
		su_logger.Error(ctx, err, "mysql lite upsert single field failed", su_logger.E().String("table", table).String("id", row.Id))
		return err
	}
	return nil
}

// BatchUpsert 批量更新或插入记录，使用 CASE WHEN 优化性能
func (m *MysqlLite) BatchUpsert(ctx context.Context, table string, rows []UpsertRow, options ...*BatchWriteOptions) (*BatchUpsertRs, error) {
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

		if err := m.batchUpsertWithCaseWhen(ctx, table, batch, opt); err != nil {
			if opt.ContinueOnError != 1 {
				return nil, err
			}
		}
	}

	return rs, nil
}

// batchUpsertWithCaseWhen 使用 CASE WHEN 实现批量 upsert
func (m *MysqlLite) batchUpsertWithCaseWhen(ctx context.Context, table string, rows []UpsertRow, opt *BatchWriteOptions) error {
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
				placeholders = append(placeholders, "?")
				args = append(args, val)
			} else {
				placeholders = append(placeholders, "DEFAULT")
			}
		}
		valueRows = append(valueRows, "("+strings.Join(placeholders, ", ")+")")
	}

	// 4. 构建 ON DUPLICATE KEY UPDATE 部分（使用 CASE WHEN）
	var updateParts []string
	updateFieldMap := make(map[string][]struct {
		id    string
		value interface{}
		op    string
	})

	for _, row := range rows {
		for _, u := range row.Updates {
			updateFieldMap[u.Field] = append(updateFieldMap[u.Field], struct {
				id    string
				value interface{}
				op    string
			}{id: row.Id, value: u.Value, op: u.Op})
		}
	}

	for field, updates := range updateFieldMap {
		qf := quoteField(field)
		var caseWhen strings.Builder
		caseWhen.Grow(len(updates) * 48)

		// 检查是否所有更新都是相同的操作类型
		hasIncr := false
		for _, u := range updates {
			if strings.ToLower(u.op) == "incr" {
				hasIncr = true
				break
			}
		}

		if hasIncr {
			// 对于 incr 操作，使用 CASE WHEN 增量
			caseWhen.WriteString(qf + " = " + qf + " + CASE `id`")
			for _, u := range updates {
				caseWhen.WriteString(" WHEN ? THEN ?")
				args = append(args, u.id, u.value)
			}
			caseWhen.WriteString(" ELSE 0 END")
		} else {
			// 普通更新使用 CASE WHEN
			caseWhen.WriteString(qf + " = CASE `id`")
			for _, u := range updates {
				caseWhen.WriteString(" WHEN ? THEN ?")
				args = append(args, u.id, u.value)
			}
			caseWhen.WriteString(" ELSE " + qf + " END")
		}

		updateParts = append(updateParts, caseWhen.String())
	}

	// 5. 构建完整 SQL
	var quotedColumns []string
	for _, col := range columns {
		quotedColumns = append(quotedColumns, quoteField(col))
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s",
		quoteTable(table),
		strings.Join(quotedColumns, ", "),
		strings.Join(valueRows, ", "))

	if len(updateParts) > 0 {
		query += " ON DUPLICATE KEY UPDATE " + strings.Join(updateParts, ", ")
	}

	_, err := m.db.ExecContext(ctx, query, args...)
	if err != nil {
		su_logger.Error(ctx, err, "mysql lite batch upsert failed", su_logger.E().String("table", table))
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
func (m *MysqlLite) ToUpdateOne(data any, options *UpdateOptions) (upData UpdateOne) {
	opt := dftMysqlLiteToUpdateOptions()
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
	processStructMysqlLite(v, t, opt, &upData, &containUpdateField)
	if len(upData.Updates) > 0 {
		if !containUpdateField && !opt.IgnoreUpdateTimeField {
			upData.Updates = append(upData.Updates, Update{Field: UpdateField, Value: su_time.CurrentTimestampMilli()})
		}
	}
	return upData
}

func dftMysqlLiteToUpdateOptions() *UpdateOptions {
	return &UpdateOptions{
		Tag: "json",
	}
}

func processStructMysqlLite(v reflect.Value, t reflect.Type, opt *UpdateOptions, upData *UpdateOne, containUpdateField *bool) {
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

// quoteTable 对表名添加反引号，防止 SQL 注入
// users => `users`
// db.users => `db`.`users`
func quoteTable(table string) string {
	// 如果已经包含反引号，不重复处理
	if strings.Contains(table, "`") {
		return table
	}
	if idx := strings.Index(table, "."); idx != -1 {
		// 带数据库名的情况: db.users => `db`.`users`
		dbName := table[:idx]
		tableName := table[idx+1:]
		return "`" + dbName + "`.`" + tableName + "`"
	}
	return "`" + table + "`"
}

// quoteField 对字段名添加反引号，防止 SQL 注入
// name => `name`
// u.name => u.`name`
func quoteField(field string) string {
	// 如果已经包含反引号，不重复处理
	if strings.Contains(field, "`") {
		return field
	}
	if idx := strings.LastIndex(field, "."); idx != -1 {
		// 带表别名的情况: u.name => u.`name`
		prefix := field[:idx+1]
		name := field[idx+1:]
		return prefix + "`" + name + "`"
	}
	return "`" + field + "`"
}

// buildInsertParts 构建 INSERT 语句的列、占位符和值
func buildInsertParts(data map[string]interface{}) (columns string, placeholders string, values []interface{}) {
	var cols []string
	var phs []string
	for k, v := range data {
		cols = append(cols, quoteField(k))
		phs = append(phs, "?")
		values = append(values, v)
	}
	return strings.Join(cols, ", "), strings.Join(phs, ", "), values
}

// buildUpdateOnDuplicate 构建 ON DUPLICATE KEY UPDATE 部分
func buildUpdateOnDuplicate(data map[string]interface{}) string {
	var parts []string
	for k := range data {
		if k == "id" {
			continue
		}
		qf := quoteField(k)
		parts = append(parts, fmt.Sprintf("%s = VALUES(%s)", qf, qf))
	}
	return strings.Join(parts, ", ")
}

// buildSQLCondsLite 构建 WHERE 条件
func buildSQLCondsLite(conds Conds) (string, []interface{}) {
	if len(conds) == 0 {
		return "", nil
	}
	var sb strings.Builder
	sb.Grow(len(conds) * 32) // 预分配容量
	args := make([]interface{}, 0, len(conds))
	for i, c := range conds {
		if i > 0 {
			sb.WriteString(" AND ")
		}
		qf := quoteField(c.Field)
		op := strings.ToLower(c.Cond)
		switch op {
		case ">", ">=", "<", "<=", "=", "==", "!=":
			if op == "==" {
				op = "="
			}
			sb.WriteString(qf + " " + op + " ?")
			args = append(args, c.Value)
		case "in", "array-contains":
			sb.WriteString(qf + " IN (")
			vals := toSlice(c.Value)
			phs := make([]string, len(vals))
			for j := range vals {
				phs[j] = "?"
				args = append(args, vals[j])
			}
			sb.WriteString(strings.Join(phs, ", "))
			sb.WriteString(")")
		case "not-in":
			sb.WriteString(qf + " NOT IN (")
			vals := toSlice(c.Value)
			phs := make([]string, len(vals))
			for j := range vals {
				phs[j] = "?"
				args = append(args, vals[j])
			}
			sb.WriteString(strings.Join(phs, ", "))
			sb.WriteString(")")
		case "like":
			sb.WriteString(qf + " LIKE ?")
			args = append(args, c.Value)
		case "is null":
			sb.WriteString(qf + " IS NULL")
		case "is not null":
			sb.WriteString(qf + " IS NOT NULL")
		default:
			sb.WriteString(qf + " = ?")
			args = append(args, c.Value)
		}
	}
	return sb.String(), args
}

// toSlice 将 interface{} 转换为 []interface{}
func toSlice(v interface{}) []interface{} {
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

// isDuplicateError 判断是否是重复键错误
func isDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "duplicate") || strings.Contains(errStr, "1062")
}

// scanRowsToMaps 将多行扫描为 []map
func scanRowsToMaps(rows *sql.Rows) ([]map[string]interface{}, error) {
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
			// 处理 []byte 类型
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
